# vanda を vanda で開発する形に切り替える計画

この計画は、監督・開発者・テスト担当が、vanda のリポジトリを vanda で開発する形に切り替えるときに使う。

## 目的

今ある機能（段 1・段 2・`vanda init`・`.vanda/adapters/` のアダプタ）の約束を全部書き、テストをエビデンスとして結び、PO が GUI で全部に合意できる状態にする。そこから先の開発は、ストーリーごとに約束を足し・変え、合意してから合流する。

## ストーリー

切り替えそのものを、vanda の GitHub Issues の 1 つのストーリーにする（監督が作る）。作業は `main` から切ったブランチで行い、コミットのメッセージにはストーリーの ID を入れる。PO は、ベースブランチを `main` にしたレビューの画面で約束を読み、合意する。全部に合意し、審査が通ったら `main` に合流する。

## 手順

| # | すること | する人 |
|---|---|---|
| 1 | ストーリーの Issue を作り、ブランチを切る | 監督 |
| 2 | `vanda init` を Expect で動かして `.vanda/` を作る（答えはベースブランチ `main`、GitHub Issues の `TadahiroYamamura/vanda`、ID の書き方は既定、言語は Go・シナリオ・TLA+ でパターンは既定）。できた設定を PO に見せる | 監督 |
| 3 | `.vanda/config.toml` にレイヤーを書く（下の「レイヤー」） | 監督（PO の了承の後） |
| 4 | 情報の置き場所の rules（`.claude/rules/information-placement.md`）を見直し、約束リストと `docs/spec.md`・`docs/formats.md` の分け方を決める | 監督が案を出し、PO が決める |
| 5 | 約束リストを書く: 用語（`docs/terms.md` から）、機能（`docs/spec.md` の作ってある機能）、約束（機能ごとの振る舞いと、エビデンスの予定）。どれも `ai-proposal` のフラグを立て、推し量ったことは `guesses` に書く | 開発者 |
| 6 | 今あるテスト（Go のテスト、シナリオ、TLA+ のモデル）に、約束を指すアノテーションを付ける。エビデンスの予定に合うテストがない約束は、報告に挙げる | テスト担当 |
| 7 | `vanda check .vanda/checks/ci.toml --base origin/main` を CI に足す（下の「CI」） | 開発者 |
| 8 | PO が GUI で全部の約束を読み、合意する。直すものは開発者・テスト担当に戻す | PO |
| 9 | 審査と `make check` が通ったら、`main` に合流する | 監督（PO の指示で） |

## 約束の書き方

- 約束は、`docs/spec.md` と `docs/formats.md` に書いてある振る舞いから書く。実装を読んで書かない（実装の誤りを約束にしないため）
- 1 つの約束には、1 つの確かめられる振る舞いを書く
- まだ作っていない機能（段 3 より後）の約束は書かない
- テストのない振る舞いも約束にする。エビデンスの予定に書き、揃っていないことを審査で見えるようにする

## レイヤー（案）

| id | name | 中身 |
|---|---|---|
| `model` | モデル | 合意の判定の TLA+ のモデル |
| `cli` | CLI | `vanda` の命令（`check`・`doctor`・`init`・`serve` の起動） |
| `api` | API | `vanda serve` の HTTP の API |
| `gui` | 画面 | 画面の部品と E2E |

エージェントの状態の置き場所など、まだ作っていないもののレイヤーは、作るときに足す。

## CI

- push と pull request のたびに、今の `make check` に加えて `vanda check .vanda/checks/ci.toml --base origin/main` を動かす
- `main` への push では、ベースブランチと分かれた点より後のコミットがないので、範囲 `story` の審査はストーリーを決められない。`main` では範囲 `all` の審査だけを動かす審査の設定（`.vanda/checks/main.toml`）を別に置く

## 依存

足さない。

# vanda 段 4 の 6 の直しの計画: 読み込みの間に先に出せるものを出す

story #4。PO が画面で、ストーリーのレビューに戻るたびと、約束の詳細を開くたびに、約束リストから読めるだけの値まで待たされると言った。待つ時間そのもの（git の呼び出し）は後で別に縮める。ここでは、待つ間の見た目を直す。約束は監督が足してある（claim-detail-shown-before-history・review-detail-shown-before-list・loading-indicator）。約束は直さない。約束にない判断が要るときは、作る前に監督に聞く。

前提として読むもの: `CLAUDE.md`、`docs/formats.md`（GUI の API）、`docs/pitfalls.jsonl`、上の約束の文。

## 計った値（監督が計った。story-4、ストーリー #4、ベースブランチ main）

| API | 時間 | 遅い理由 |
|---|---|---|
| `/api/review`（ストーリー単位もブランチ単位も） | 約 5 秒 | 履歴のコミットごとに git（cat-file・diff-tree・ls-tree）を約 1600 回呼ぶ |
| `/api/claims/<ID>` | 約 6 秒 | 由来と合意の履歴のために、履歴のコミットごとに git を呼ぶ |
| `/api/terms/<ID>`、`/api/context` | 0.03 秒 | |

## 作ること

1. API: `GET /api/claims/<ID>` から由来（`origin`）と合意の履歴（`agreements`）を外し、`GET /api/claims/<ID>/history` で `{origin, agreements}` を返す。`/api/claims/<ID>` は履歴をたどらずに返す（状態・エビデンスの実際は今のまま入れる）。`docs/formats.md` を直す
2. 約束の詳細の画面: 2 つを別々に読み、`/api/claims/<ID>` が来たら中身を出す。由来と合意の履歴の欄は、`/history` が来るまで読み込み中の印を出す
3. ストーリーのレビュー: 一覧を読み込む間も、URL で選ばれている 1 件（`sel`）があれば、その今の中身を、約束は `/api/claims/<ID>`、用語は `/api/terms/<ID>`、機能は `/api/features/<ID>`、ファイルは `/api/file` で読んで先に出す。変更前の中身・差分・状態・フラグ・合意と取り消しのボタンは、一覧を読み終えてから出す。消したもの（今はない 1 件）は、一覧を待つ
4. 読み込み中の印: 回る印と「読み込んでいます…」の文字。回る印は飾り（読み上げない）にし、文字は今のまま読ませる。動きを減らす設定（prefers-reduced-motion）では回さない。今「読み込んでいます…」を出している所をすべてこの部品にする

## 担当と順

1. 開発者: 上の 1〜4。API と画面でコミットを分ける。落ちた既存のテストは名前を報告に書く（直さない）
2. テスト担当: 1 の後。上の約束のエビデンスを足し、既存のテストを直す。API が遅い時に先に出すことは、API の応答を遅らせて確かめる

## 確かめること

- `nix develop -c make check`
- `./vanda check .vanda/checks/ci.toml --base main` の promise-list-shape・annotation-links・evidence-present

## 禁止

- `pkill`・`killall`、`~/work/vanda` と `~/work/vanda-demo` での `vanda init`
- `docs/pitfalls.jsonl` の編集（候補は報告に書く）
- プッシュ、`git rebase`・`filter-branch`・`commit --amend`・`reset --hard`、`git switch` でほかのブランチに移ること
- 127.0.0.1:45569 と 127.0.0.1:45600 に触れること。サーバーを起こすときは別のポートにし、終わったら自分のプロセスだけを PID で止める
- 約束リストと合意の記録（`.vanda/agreements.toml`）の編集
- 開発者がテストのファイル（`*_test.go`、`cmd/vanda/testdata/`、`spec/`、`web/src/__tests__/`、`web/e2e/`）を書き換えること。テスト担当が実装のファイルを書き換えること

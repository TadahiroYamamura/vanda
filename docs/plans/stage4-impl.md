# vanda 段 4 の 1 の実装の計画（開発者向け）

story #4 の段 4 の 1。約束リスト（`.vanda/promises.toml`）を正とし、実装を約束に合わせる。約束は直さない。約束の文と実装のどちらに合わせるか迷ったら、作る前に監督に聞く。

前提として読むもの: `CLAUDE.md`、`docs/formats.md`、`docs/pitfalls.jsonl`（`trigger` が当たるものの `question` を確かめる）、下の各単位に挙げた約束の文。用語は約束リストの `[[term]]` を引く。

## やらないこと

- 参照（`{@id:…}`、約束 reference-*）。段 4 の 2 で作る
- 画面と CLI の文言を用語にそろえること（vanda サーバー、カスタムアダプタ、分岐点、ストーリーストア）。段 4 の 3 で行う。ただし、この計画で新しく書く文言は、はじめから用語で書く
- テストのファイル（`*_test.go`、`cmd/vanda/testdata/`、`spec/`、`web/src/__tests__/`、`web/e2e/`）を書き換えること。テスト担当が直す。実装を変えて既存のテストが落ちたら、落ちたテストの名前と、どの約束のために落ちたかを報告に書く

## 単位と順

| # | 中身 | 順 |
|---|---|---|
| U1 | ステージ test とエビデンスのタイプ monitoring を外す | はじめ。U4・U5 と並べてよい |
| U2 | 合意: ファイルを場所と指紋で見分ける、消したことへの合意、B1、外れた合意を履歴の順で決める | U1 の後 |
| U3 | 審査: コミットした中身だけを読む、合意の対象をタイプで決める、ガードレールとセンサーの揃い方、ほか | U2 の後 |
| U4 | init（main.toml と案内）、組込みの github のアダプタの鍵 | U1 と並べてよい |
| U5 | 画面: TLA+ の記号の書体と幅、Host のポート | U1 と並べてよい |

単位ごとにコミットを分け（目的の違う変更は分ける）、監督に報告する。

## U1. ステージ test と monitoring を外す

約束: promise-list-fixed-value-list、promise-list-fixed-values、claim-detail-evidence-grouping。用語 stage・evidence-type。

- `internal/check/shape.go` の `Stages` から test、`Types` から monitoring を外す
- monitoring が残る所: `internal/check/others.go`、`internal/config/config.go`（タイプの一覧と既定の types）、`internal/review/claim.go`、`web/src/labels.ts`
- test のステージが残る所: `web/src/labels.ts` の stageLabel、`web/src/components/EvidenceGroups.tsx` の STAGES
- `docs/formats.md` の両方の記述（monitoring は 355・675 行目あたり、test は 113・134・477・660 行目あたり）。`docs/plans/annotation-types.md` は過去の計画なので直さない

## U2. 合意

約束: agreed-when-fingerprint-matches、renamed-file-needs-agreement、deleted-item-needs-agreement、deletion-agreement-record、agreements-file-record-path、agreements-point-to-existing-items、pass-only-when-all-seen、story-pass-only-when-seen、detached-agreement、detached-until-story-reagrees、revoke-returns-to-proposed、review-renamed-file、review-count-verified、review-state-shown、review-deleted-shown、review-detached-shown。モデルは `spec/Agreement.tla`（記録は `[item, fp, story]`、消したことへの合意は fp = 0）。

1. ファイルの合意は、場所と指紋の両方が合うときだけ合意済にする。`internal/agreements` の `Records.Agreed`・`AgreedBy`、`internal/check/others.go`、`internal/review/review.go`、`internal/review/revoke.go`（取り消す記録の選び方と、取り消した後の判定）。今は指紋だけで当てていて、写しが合意済になる
2. 消したことへの合意の記録（`deleted = true`、fingerprint を持たない。ファイルなら key は path）を、読む・書く・取り消す。記録の形の確かめは agreements-file-record-path のとおり
3. 審査 agreement: 合意したことのある項目・合意の対象のファイルが今なく、消したことへの合意がなければ誤り。範囲 story では、比べる元にあって今ないものを、合意したことがあるかによらず見る
4. 審査 agreement-targets: 約束リストにない項目を指す記録は、消したことへの合意があれば誤りにしない
5. レビューの一覧: 名前の変更を 1 件にまとめず、古い場所の削除と新しい場所の追加の 2 件にする（API の `oldPath`、画面の「名前の変更」の件数と表示、「前の場所」もなくす）。数の確かめ（review_sensor.go）から renamed の引き算を外す。削除した件にも状態（消したことへの合意があれば合意済、なければ提案中）を付け、画面から消したことに合意できるようにする
6. B1: ストーリー s が合意 → t が変えて合意した項目は、今のストーリーが s なら、画面で s が合意し直せるようにする。今は `web/src/pages/ReviewPage.tsx` の AgreeControls が `state === "proposed"` のときだけボタンを出すため、出ない。合意できるかどうかの判定は Go の側に置き、画面は API の結果で出し分ける。取り消しの理由の文も事実に合わせる
7. 外れた合意を変えたストーリーは、合意の記録の `agreed_at` とコミットの日時を比べず、HEAD の履歴で、そのストーリーが最後に合意の記録を足したコミットより後のコミットで決める（`internal/review/review.go` の changedBy）
8. `docs/formats.md` の合意の記録・合意済の判定・一覧の項目の記述を直す

クロスチェックの Go 側（`cmd/vanda/crosscheck_test.go`）はテスト担当が新しいモデルに合わせる。開発者は、テスト担当が足す・消す・名前を変える・消したことへの合意の操作を本物の vanda で起こせるように、実装を作る。

## U3. 審査

| 約束 | 直すこと |
|---|---|
| reads-committed-files-only、review-view-branch、review-view-story | 審査とレビューの画面が、設定・約束リスト・合意の記録・アノテーションを読むファイルを、作業ツリーではなく HEAD のコミットの中身で読む。画面の「コミットしていない変更も含む」の文言をなくす |
| check-agreement-targets | 合意の対象のファイルを、アダプタの名前（txtar・tla）ではなく、types に scenario-test か model-check を含む `[[annotation]]` のパターンで決める（`internal/snapshot/snapshot.go`、`docs/formats.md`） |
| check-evidence-guardrail-needs-definition | guardrail の予定は、ガードレールの定義と、types に guardrail を含む `[[annotation]]` のエビデンスの両方があるときだけ揃う |
| check-evidence-sensor-needs-check | sensor の予定は、センサーの場所（sensor のアノテーション）と、センサーの検証（types に sensor を含む `[[annotation]]` のエビデンス）の両方があるときだけ揃う。画面のエビデンスの表（`internal/review/claim.go`）も同じ判定にする |
| promise-list-formal-claims | 約束の項目 model_check_waiver を足し、formal の機能を subjects に持つ約束に model-check の予定と model_check_waiver のどちらか一方だけを求める |
| annotation-ts-skipped-not-counted、annotation-ts-only-rejected | ts の `.skip`・`.todo` に付けたアノテーションは数えない。`.only` は誤り |
| config-annotation-timeout、project-adapter-annotation-timeout | `[[annotation]]` の timeout（秒、既定 20）。カスタムアダプタが時間を過ぎたら、どのアダプタかを書いたエラーにする |
| check-story-from-commits | トレーラーからストーリーが 1 つに決まり、--story がそれと違えば、審査の前の誤りにする |
| check-story-arg-pattern | --story の形は、どの審査があるかによらず確かめる |
| config-version-mismatch | ない「移行のコマンド」を案内しない |

`docs/formats.md` の該当する記述も同じコミットで直す。

## U4. init と github のアダプタ

| 約束 | 直すこと |
|---|---|
| init-writes-files、init-check-configs、init-guide | `.vanda/checks/main.toml`（story-origin を除く審査を範囲 all）も作り、5 つのファイルにする。案内は「ブランチでは ci.toml を --base <ベースブランチ> で、ベースブランチでは main.toml を動かす」 |
| init-terminal-streams、init-requires-terminal | 端末かどうかを、文字デバイスかどうかではなく、端末かどうかで判定する（/dev/null を端末とみなさない）。依存は足さない |
| story-github-token、story-github-token-missing、doctor-github-token | 鍵を api_url のホストで選ぶ。api.github.com なら GH_TOKEN か `gh auth token --hostname github.com`、ほかは GH_ENTERPRISE_TOKEN か `gh auth token --hostname <ホスト>`。見つからない（空を含む）なら送らずにエラー |
| story-github-http-warning | api_url が http なら、鍵を送るたびに標準エラーに警告を出す |

## U5. 画面とサーバー

| 約束 | 直すこと |
|---|---|
| tla-symbol-font | Noto Sans Math を同梱する（PO 承認済みの依存。`@fontsource/noto-sans-math` の woff2 と OFL を、ほかの書体と同じ形で）。CLAUDE.md の依存の表に足す |
| tla-symbol-keeps-width | 記号は、置き換える前の ASCII の文字数ぶんの幅（等幅の 1 文字が単位）を取り、その中央に描く |
| serve-rejects-foreign-host | Host のポートも、待ち受けているポートと合うかを確かめる |

## 確かめること（各単位の報告の前）

- `nix develop -c make check`。テスト担当の直しを待つものは、どのテストが何のために落ちたかを書く
- `./vanda check .vanda/checks/ci.toml --base main` の annotation-links と promise-list-shape

## 禁止

- `pkill`・`killall`、`~/work/vanda` と `~/work/vanda-demo` での `vanda init`
- `docs/pitfalls.jsonl` の編集（候補は報告に書く）
- プッシュ、`git rebase`・`filter-branch`・`commit --amend`・`reset --hard`
- 127.0.0.1:45569 に触れること。サーバーを起こすときは別のポートにし、終わったら止める
- 約束リストと合意の記録（`.vanda/agreements.toml`）の編集

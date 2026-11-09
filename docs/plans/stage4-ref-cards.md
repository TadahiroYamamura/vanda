# vanda 段 4 の 6 の直しの計画: 参照の枠と、用語・機能の詳細のページ

story #4。PO が画面で参照を見て、黒いツールチップではなく、指す先の中身を小さな枠（部品）で出し、用語と機能も詳細のページへ移れるようにすると決めた。約束は監督が直してある（reference-shown・reference-popup を直し、機能 show-term-feature-pages と約束 term-page-content・feature-page-content・term-feature-page-claims・term-feature-page-missing を足した）。約束は直さない。約束にない判断が要るときは、作る前に監督に聞く。

前提として読むもの: `CLAUDE.md`、`docs/formats.md`（GUI の API）、`docs/pitfalls.jsonl`（`trigger` が当たるものの `question` を確かめる）、上の約束の文、`docs/plans/stage4-references.md`（参照の API の形）。

## 決めたこと（PO と監督）

| 項目 | 中身 |
|---|---|
| 枠の中身 | 約束: ID と文。用語: ID と定義。機能: ID と定義と、コア機能・形式手法を使う機能の印。名前は参照そのものに出ているので枠には要らない（出してもよい） |
| 枠の出し方 | 参照に触れた時・フォーカスした時に出す。Esc で閉じる。枠の中の文の参照は参照の見た目で出すが、その参照に触れても次の枠は出さない |
| 参照のリンク | 指す先が今の約束リストにあれば、約束は `/claims/<ID>`、用語は `/terms/<ID>`、機能は `/features/<ID>` へのリンクにする。今の約束リストにないものはリンクにしない |
| 用語の詳細のページ `/terms/<ID>` | 名前・ID・定義、part_of の親の用語、それを親にしている用語（子）、それを subjects に持つ約束の一覧 |
| 機能の詳細のページ `/features/<ID>` | 名前・ID・定義、コア機能・形式手法を使う機能の印、それを subjects に持つ約束の一覧 |
| 約束の一覧 | 約束リストに書いた順に、ID と文。それぞれ約束の詳細へのリンク |
| ないとき | 今の約束リストにない ID なら、その ID が約束リストにないことを示す |

- API は `GET /api/terms/<ID>`・`GET /api/features/<ID>` を足し、自然言語の項目は今の片（`[{text, ref?}]`）と `refs` の形で返す。`refs` の機能の項目に `core`・`formal` を足す。`docs/formats.md` の GUI の API に書く
- 参照の見た目・片の形・差分の決まり（reference-*）は変えない

## 担当と順

1. 開発者: API（`internal/review` か近い所）、画面（`web/src`。今のツールチップの部品を枠の部品に替え、詳細のページを 2 つ足す）、`docs/formats.md`。API と画面でコミットを分ける。実装を変えて落ちた既存のテストは、名前を報告に書く（直さない）
2. テスト担当: 1 の後。上の約束のエビデンス（api と gui の unit-test）を足し、ツールチップを前提にした既存のテストを枠に合わせて直す。期待値は約束とこの計画から作る

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

# vanda 段 4 の 3 の計画: 文言を用語にそろえる

story #4 の段 4 の 3。約束リストの用語（`[[term]]`）にない言い方を、用語の名前にそろえる。振る舞いは変えない。PO の決定で、CLI の出力・画面だけでなく、コメント・`docs/`・テストの名前と期待値もそろえる。

## 置き換え

| 用語（ID） | 今の言い方（例） | そろえた言い方 |
|---|---|---|
| vanda サーバー（vanda-server） | 「手元のサーバー」「vanda のサーバー」「GUI のサーバー」 | vanda サーバー |
| カスタムアダプタ（custom-adapter） | 「.vanda/adapters/ のアダプタ」「利用者のアダプタ」 | カスタムアダプタ |
| 分岐点（branch-point） | 「分かれた点」「分かれたコミット」「ベースブランチから分かれた点」 | 分岐点 |
| ストーリーストア（story-store） | 「ストーリーの置き場所」、ストーリーの話の中の「置き場所」 | ストーリーストア |

- 「置き場所」は、ストーリーストアを指すものだけを置き換える。ファイルの場所や情報の置き場所（`.claude/rules/information-placement.md`）の意味のものは変えない
- 置き換えて文がつながらなくなる所（「ストーリーの置き場所の…」→「ストーリーストアの…」など）は、文を直す。意味は変えない
- 誤りの「どうすればよいか」で、カスタムアダプタの置き場所を言う必要がある所だけは「`.vanda/adapters/` に置いたカスタムアダプタ」のように場所を残してよい
- 約束リスト（`.vanda/promises.toml`）には、今の言い方は残っていない。直さない
- `docs/plans/` は過去の計画なので直さない

## 担当と順

1. 開発者: 実装のファイル（Go・TypeScript・`web/src` の画面）、コメント、`docs/formats.md`・`docs/spec.md`、`cmd/vanda/serve.go` などのヘルプの文。終わったら、CLI の出力と画面の文言の「前 → 後」の一覧を報告に書く
2. テスト担当: 1 の後。テストの名前・期待値・`cmd/vanda/testdata/` の台本と Expect、`spec/` の TLA+ のコメントを、上の置き換えと開発者の一覧にそろえる

## 確かめること

- `nix develop -c make check`
- 上の「今の言い方」が、ストーリーストアでない「置き場所」と `docs/plans/` を除いて残っていないこと（grep の結果を報告に書く）

## 禁止

- `pkill`・`killall`、`~/work/vanda` と `~/work/vanda-demo` での `vanda init`
- `docs/pitfalls.jsonl` の編集（候補は報告に書く）
- プッシュ、`git rebase`・`filter-branch`・`commit --amend`・`reset --hard`
- 127.0.0.1:45569 に触れること
- 約束リストと合意の記録（`.vanda/agreements.toml`）の編集
- 開発者がテストのファイル（`*_test.go`、`cmd/vanda/testdata/`、`spec/`、`web/src/__tests__/`、`web/e2e/`）を書き換えること。テスト担当が実装のファイルを書き換えること

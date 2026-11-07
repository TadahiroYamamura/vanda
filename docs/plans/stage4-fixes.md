# vanda 段 4 の 4 の後の計画: エビデンスで見つかった食い違いを直す

story #4。段 4 の 4 でエビデンスを足したときに、約束どおりに動かなかった所を、約束に合わせて直す。約束は直さない（監督が直してある）。

前提として読むもの: `CLAUDE.md`、`docs/formats.md`、`docs/pitfalls.jsonl`（`trigger` が当たるものの `question` を確かめる）、下の約束の文。

## 直すこと

| # | 約束 | 今 | 直した後 |
|---|---|---|---|
| F1 | agree-record-file、agreements-explicit-file-may-be-absent | 設定の agreements にワイルドカードのない場所をはっきり書き、そのファイルがないと、読み込みで「合意の記録のファイルがありません」の誤りになる。合意の API は 500 を返す | 誤りにせず、そのファイルの合意の記録を空とみなす。合意すると、そのファイルを作って書く（最初のパターンにワイルドカードがないとき） |
| F2 | config-layer-id | `[[layer]]` の `id = ""` が誤りにならない（`internal/config/config.go` は空の id のときに形を確かめない） | 空の id も、決まった形でないとして設定の誤りにする |
| F3 | config-pattern-syntax | 誤りにするのは、空の文字列、`/` で始まるもの、`path.Match` の書き方の誤りだけ | ほかに、`/` で区切った部分のどれかが空・`.`・`..` のもの（`./a`・`../a`・`a/../b`・`a//b`・`a/`）も、設定の誤りにする |

- F1 に合わせて、`docs/formats.md` の「設定にはっきり書いた、ワイルドカードのない場所のファイルがなければ、エラーにする」の所を、約束リストのファイルだけの話に直し、合意の記録のファイルは、はっきり書いたものもなければ空とみなすと書く
- 約束リストのファイル（promise-list-explicit-file-must-exist）は今のまま、はっきり書いたファイルがなければ誤り

## 担当と順

1. 開発者: F1〜F3 の実装と `docs/formats.md`。F ごとにコミットを分ける。実装を変えて落ちた既存のテストがあれば、その名前を報告に書く（直さない）
2. テスト担当: 1 の後。次のテストを足し、落ちた既存のテストを約束に合わせて直す
   - F1: agreements-explicit-file-may-be-absent の cli と api の unit-test。agree-record-file の「はっきり書いたまだないファイルを作る」場合
   - F2: config-layer-id のテストの入力に、空の id を足す
   - F3: config-pattern-syntax のテストの入力に、上の 5 つの形を足す。ワイルドカードを含むもの（`./**/*.go` など）も入れる

## 確かめること

- `nix develop -c make check`
- `./vanda check .vanda/checks/ci.toml --base main` の evidence-present と promise-list-shape が通ること

## 禁止

- `pkill`・`killall`、`~/work/vanda` と `~/work/vanda-demo` での `vanda init`
- `docs/pitfalls.jsonl` の編集（候補は報告に書く）
- プッシュ、`git rebase`・`filter-branch`・`commit --amend`・`reset --hard`、`git switch` でほかのブランチに移ること
- 127.0.0.1:45569 に触れること
- 約束リストと合意の記録（`.vanda/agreements.toml`）の編集
- 開発者がテストのファイル（`*_test.go`、`cmd/vanda/testdata/`、`spec/`、`web/src/__tests__/`、`web/e2e/`）を書き換えること。テスト担当が実装のファイルを書き換えること

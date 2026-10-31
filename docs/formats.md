# vanda のファイルと受け渡しの形

`docs/spec.md` で決めた中身を、どの名前・どの形で書くかをまとめる。`.vanda/` の外の取り決め（アノテーションの書き方、アダプタとの受け渡しの形）は、v1.0.0 からは変えない。

## エラーの形

- 1 件を 1 行で出す。最初の 1 件で止めず、見つけたものを全部出す
- 形は `<パス>:<行>: <何がいけないか>。<どう直すか>`。パスはリポジトリの直下からの `/` 区切りの場所。行は 1 から数える
- 行が見つからないとき（インラインの表 `{ … }` の中の項目など）は、行を書かず、ID と項目で場所を示す: `<パス>: <場所>: <何がいけないか>。<どう直すか>`（例: `.vanda/promises.toml: claim "task-unique" の evidence の 2 件目 の layers: …`）。推測の行は出さない
- 項目がないときは、その項目を持つべき表の見出しの行を示す

## `.vanda/` の場所

`.vanda/` は、git のリポジトリの根（`git rev-parse --show-toplevel`）で探す。git のリポジトリの外で動かしたら誤りにする。

## 版

`.vanda/config.toml` の `version` が動いている vanda の版と合わないときは、`vanda check` も `vanda doctor` も止まる。`vanda version` だけは止まらない。

## ファイルのパターン

設定に書くファイルのパターンは、リポジトリの直下からの `/` 区切りの場所で書く。

| 書き方 | 当たるもの |
|---|---|
| `*` | `/` を含まない 0 文字以上 |
| `?` | `/` でない 1 文字 |
| `[…]` | 括弧の中の 1 文字（Go の `path.Match` と同じ） |
| `**` | 0 段以上のディレクトリ。`/` で区切った 1 段に `**` だけを書く（例: `e2e/**/*.txtar`） |

- ワイルドカードのない場所を書き、そのファイルがなければエラーにする。ワイルドカードのあるパターンが何にも当たらないのは、エラーにしない
- `.git` の中は見ない

## 設定（`.vanda/config.toml`）

```toml
version = "0.1.0"
promises = [".vanda/promises.toml"]
agreements = [".vanda/agreements.toml"]

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["**/*_test.go", "cmd/**/*.go"]
adapter = "go"

[[annotation]]
paths = ["docs/**/*.md"]
command = ["tools/vanda-md-adapter", "--strict"]

[[guardrail]]
adapter = "file"
paths = ["db/schema.sql"]

[story]
id_pattern = "#[0-9]+"
```

| 項目 | 型 | 必ずか | 中身 |
|---|---|---|---|
| `version` | 文字列 | 必ず | 使う vanda の版（`major.minor.patch`）。数として比べ、動いている vanda が古ければ上げるよう、新しければ移行のコマンドで `.vanda/` を書き換えるよう知らせて、エラーにする |
| `promises` | 文字列の配列 | 省いてよい | 約束リストのファイルのパターン。省けば `[".vanda/promises.toml"]` |
| `agreements` | 文字列の配列 | 省いてよい | 合意の記録のファイルのパターン。省けば `[".vanda/agreements.toml"]` |
| `[[layer]]` | 表の配列 | 省いてよい | レイヤーの一覧。`id`・`name`・`description`（どれも文字列で、必ず）。`id` は ID の形（英小文字で始め、英小文字・数字をハイフンでつないだ形）で、重ならない |
| `[[annotation]]` | 表の配列 | 省いてよい | アノテーションを読むファイルと、使うアダプタ。`paths`（文字列の配列、必ず）と、`adapter`（中に持つアダプタ: `go`・`txtar`・`tla`・`sql`）か `command`（外部のコマンドと引数の配列）のどちらか 1 つ。どちらも書かなければ、アダプタが決まっていないとみなし、当たったファイルを審査で止める |
| `[[guardrail]]` | 表の配列 | 省いてよい | ガードレールの定義の取り出し。`adapter`（今は `file` だけ。ファイルを読むだけ）と `paths`（文字列の配列）。ガードレールの定義と約束の結びつけは、アノテーション（SQL なら `--` のコメントの中の `@vanda:evidence <ID>`）で行う |
| `[story]` の `id_pattern` | 文字列 | 省いてよい | コミットのメッセージの中のストーリーの ID の書き方。ID 全体に当たる正規表現（Go の RE2）。省けば `#[0-9]+`（GitHub Issues）。空の文字列に当たるものは書けない |

決まっていない項目は、エラーにする。

## 約束リスト（既定は `.vanda/promises.toml`）

```toml
[[term]]
id = "task-id"
name = "タスクの ID"
definition = "タスクを見分ける名前"
part_of = "task"

[[feature]]
id = "add-task"
name = "タスクの登録"
definition = "タスクを足す"
core = true
formal = false
formal_waiver = "…"

[[claim]]
id = "task-id-unique"
subjects = ["task-id"]
statement = "タスクの ID は重ならない"
rationale = "…"
condition = "…"
layers = ["model", "db"]
flags = ["ai-proposal"]
guesses = ["…"]

[[claim.evidence]]
stage = "test"
type = "unit-test"
layers = ["model"]
detail = "…"
```

| 表 | 項目 | 型 | 必ずか |
|---|---|---|---|
| `[[term]]` | `id`・`name`・`definition` | 文字列 | 必ず |
| | `part_of` | 文字列（用語の ID を 1 つ） | 省いてよい |
| `[[feature]]` | `id`・`name`・`definition` | 文字列 | 必ず |
| | `core` | 真偽 | 省いてよい。省けば偽 |
| | `formal` | 真偽 | 省いてよい。省けば `core` と同じ値とみなす |
| | `formal_waiver` | 文字列 | `core = true` かつ `formal = false` のときは必ず。それ以外で書いたら誤り |
| `[[claim]]` | `id`・`statement` | 文字列 | 必ず |
| | `subjects` | 文字列の配列（用語か機能の ID。約束の ID は書けない） | 必ず。1 つ以上 |
| | `layers` | 文字列の配列（設定のレイヤーの ID） | 必ず。1 つ以上 |
| | `rationale`・`condition` | 文字列 | 省いてよい |
| | `flags` | 文字列の配列（`ai-proposal` だけ） | 省いてよい |
| | `guesses` | 文字列の配列 | 省いてよい |
| | `[[claim.evidence]]` | 表の配列 | 必ず。1 つ以上 |
| `[[claim.evidence]]` | `stage` | 文字列（`plan`・`implementation`・`test`・`release`・`operation`） | 必ず |
| | `type` | 文字列（`docs/spec.md` の「エビデンスのタイプ」の値） | 必ず |
| | `layers` | 文字列の配列（約束の `layers` の中から） | 必ず。1 つ以上 |
| | `detail` | 文字列 | 省いてよい |

- ID は、英小文字で始め、英小文字・数字をハイフンでつないだ形（例: `task-id`、`a2`）。用語・機能・約束の ID は、すべてのファイルを通して重ならない
- 重なった ID の誤りは、後に出てきたほうの行を指し、前に出てきた場所をメッセージに書く
- 知らない表・項目の誤りは、その表の見出し・項目の行を指す
- 表は `term`・`feature`・`claim` だけ。決まっていない表と項目は、エラーにする
- `[[claim.evidence]]` は、インラインの表の配列（`evidence = [{ … }]`）で書いてもよい。ただし、エラーでは行を示せない
- 設定の `promises` のパターンに当たるファイルを、すべて読む

## 審査の設定（`.vanda/checks/<名前>.toml`）

```toml
[[check]]
name = "promise-list-shape"
scope = "all"

[[check]]
name = "agreement"
scope = "story"
```

| 項目 | 中身 |
|---|---|
| `name` | 審査の名前。`promise-list-shape`（約束リストの形）・`annotation-links`（アノテーションのつながり）・`story-origin`（由来）・`agreement`（合意）・`agreement-targets`（合意の記録の指す先）・`evidence-present`（エビデンスが揃っている）・`unread-files`（読まれていないファイル） |
| `scope` | `all`（全部）か `story`（1 つのストーリーで変わったもの） |

- 書いた順に行う。どれかが失敗したら、終了コードを失敗にする
- `scope = "story"` の審査を含むなら、`--base <ブランチ>` が要る。どのストーリーかは、ベースブランチから分かれた点より後のコミットのメッセージにある ID（設定の `id_pattern`）から決める。ID が 1 つならそれ。0 個か 2 つ以上なら、`--story <ID>` を求めるエラーにする。`--story` を渡せば、それを使う
- 読まれていないファイルの審査（`unread-files`）は、`.vanda/config.toml` と `.vanda/checks/*.toml` を除く

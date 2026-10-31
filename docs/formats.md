# vanda のファイルと受け渡しの形

`docs/spec.md` で決めた中身を、どの名前・どの形で書くかをまとめる。`.vanda/` の外の取り決め（アノテーションの書き方、アダプタとの受け渡しの形）は、v1.0.0 からは変えない。

## エラーの形

- 1 件を 1 行で出す。最初の 1 件で止めず、見つけたものを全部出す
- 出す順は毎回同じにする。ファイルの場所の順、同じファイルなら行の順（行のないものは、そのファイルの先）。審査をまたぐときは、審査の設定に並べた順を先にする
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

- 設定にはっきり書いた、ワイルドカードのない場所のファイルがなければ、エラーにする。設定に `promises`・`agreements` を書かずに使う既定のファイル（`.vanda/promises.toml`、`.vanda/agreements.toml`）は、なければ空とみなす。ワイルドカードのあるパターンが何にも当たらないのは、エラーにしない
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

## 合意の記録（既定は `.vanda/agreements.toml`）

```toml
[[agreement]]
kind = "claim"
key = "task-id-unique"
fingerprint = "sha256:…"
agreed_at = 2026-10-31T12:00:00+09:00
story = "#24"

[agreement.user]
name = "Tadahiro Yamamura"
email = "…"

[[agreement]]
kind = "file"
key = "sha256:…"
fingerprint = "sha256:…"
path = "e2e/add-task.txtar"
user = { name = "…", email = "…" }
agreed_at = 2026-10-31T12:00:00Z
story = "#24"
```

| 項目 | 型 | 必ずか | 中身 |
|---|---|---|---|
| `kind` | 文字列 | 必ず | 対象の種類。`claim`・`term`・`feature`・`file` |
| `key` | 文字列 | 必ず | 約束・用語・機能なら ID。ファイルなら、中身の指紋（`fingerprint` と同じ値） |
| `fingerprint` | 文字列 | 必ず | 合意したときの中身の指紋 |
| `path` | 文字列 | ファイルのときは必ず。それ以外は書けない | 合意したときのファイルの場所 |
| `user` の `name`・`email` | 文字列 | 必ず | 合意したときの git の `user.name`・`user.email` |
| `agreed_at` | 時差の付いた日時 | 必ず | 合意した日時 |
| `story` | 文字列 | 必ず | ストーリーの ID。コミットのメッセージと同じ書き方（例: `#24`） |

- 今の中身の指紋に合う合意（`kind`・`key`・`fingerprint` がすべて合う）が 1 つでもあれば合意済
- 決まっていない表と項目は、エラーにする

## 指紋

形は `sha256:` に続けて、SHA-256 の値を 16 進の小文字 64 文字で書いたもの。

| 対象 | 元 |
|---|---|
| 約束・用語・機能 | 約束リストで読み込んだ項目の表から `flags` と `guesses` を除き、下の決まった形の JSON にしたもの（UTF-8 のバイト列） |
| ファイル（シナリオ、ガードレールの定義、オラクルの定義） | ファイルの中身のバイト列そのもの |

項目の表は、書いたとおりに使う（`id` を含む。省いた項目に既定の値を埋めない。`formal` を省いたものと `formal = true` と書いたものは、指紋が違う）。

決まった形の JSON:

- 表は `{…}`。キーを名前の順（UTF-8 のバイトの順）に並べる
- 配列は `[…]`。順はそのまま（表の配列 `[[claim.evidence]]` も、書いた順）
- 文字列は `"…"`。エスケープするのは、`"` → `\"`、`\` → `\\`、改行 → `\n`、復帰 → `\r`、タブ → `\t`、そのほかの U+0000〜U+001F → `\u00xx`（16 進の小文字）だけ。日本語などの文字や、`<`・`>`・`&` は、UTF-8 のまま書く
- 真偽は `true`・`false`、整数は 10 進
- 区切りの `,` と `:` の前後に空白を入れない。最後に改行を付けない
- 浮動小数点数と日時は、指紋を作れない（約束リストの項目に書けない値なので、エラーにする）

例: `[[term]]` に `id = "task"`、`name = "タスク"`、`definition = "やること"` と書けば、JSON は `{"definition":"やること","id":"task","name":"タスク"}`。

## アノテーション

読むファイルは、設定の `[[annotation]]` の `paths` に当たるもの。1 つのファイルが 2 つ以上の `[[annotation]]` に当たったら、誤りにする。

### 書き方（どの言語でも同じ）

- 1 行に 1 つ、`@vanda:<種類> <ID>` か `@vanda:internal <理由>` と書く。種類は `term`・`feature`・`claim`・`internal`・`evidence`・`sensor`
- `internal` のほかは、ID を 1 つだけ書く。2 つ以上の約束に関わるなら、行を分ける
- 同じ行で閉じたブロックのコメントの閉じ（`*)`・`*/`・`-->`）は、ID や理由に含めない
- 次のものは誤り: 決まっていない種類（`@vanda:foo`、`@vanda:claimx`）、ID のないもの、ID が 2 つ以上あるもの、理由のない `internal`、1 行に 2 つ以上の `@vanda:`
- アノテーションとして数えるのは、そのアダプタが知っている言語のコメントの中の `@vanda:` だけ。文字列の中などコメントの外の `@vanda:` は、数えず、誤りにもしない。外部のコマンドのアダプタでは、言語が分からないので、すべての行の `@vanda:` を数える

### 中に持つアダプタ

| `adapter` | 書ける種類 | 書ける所 | 付いているものの名前 |
|---|---|---|---|
| `go` | `evidence`・`sensor` | 数えるのは `//`・`/* */` のコメントの中だけ。`func` の宣言の直前のコメント（空行をはさまない doc comment）に書く。ほかの所のコメントは誤り | `func` の名前（テストなら `TestXxx`）。メソッドは `型.名前`（例: `Store.Add`） |
| `txtar` | `evidence` | 数えるのは、最初のファイルの区切り（`-- 名前 --` の行）より前の注釈だけ。ファイル全体に付く。区切りより後は中のファイルの中身なので、数えない | ファイルの名前から `.txtar` を除いたもの（testscript のサブテストの名前） |
| `tla` | `term`・`feature`・`claim`・`internal` | 数えるのは、コメント（`\*`、入れ子にできる `(* *)`）の中だけ。文字列の中は数えない | アノテーションの後の、空行でもコメントでもない最初の行の定義の名前（`Name ==`、`Name(…) ==`、関数の定義 `Name[…] ==`。前に `LOCAL` があってもよい）。その行が定義でなければ誤り |
| `sql` | `evidence` | 数えるのは、文字列（`'…'`）の外の `--` から行の終わりまでのコメントの中だけ | アノテーションの後の、空行でもコメント（`--` で始まる行）でもない最初の行の中身を、前後の空白を除いてそのまま |

- 書ける種類のほかのアノテーションは、誤りにする。外部のコマンドのアダプタでは、どの種類も書ける
- Go の型には、`term` を求めない（Go は実装で、仕様ではないため）

### 外部のコマンドのアダプタ

設定の `[[annotation]]` に `command = ["コマンド", "引数", …]` と書く。vanda は、`@vanda:` の行のあるファイルごとに 1 回、リポジトリの根を作業のディレクトリにしてコマンドを動かす。

標準入力に渡すもの（UTF-8 の JSON）:

```json
{"path": "docs/add-task.md", "annotations": [{"line": 2}, {"line": 7}]}
```

| 項目 | 中身 |
|---|---|
| `path` | ファイルのリポジトリの根からの `/` 区切りの場所 |
| `annotations[].line` | `@vanda:` のある行（1 から数える）。小さい順 |

標準出力に返すもの:

```json
{"names": [{"line": 2, "name": "add-task の手順"}, {"line": 7, "name": "TestAddTask"}]}
```

| 項目 | 中身 |
|---|---|
| `names[].line` | 渡された行 |
| `names[].name` | その行のアノテーションが付いているものの名前（空にしない） |

- 渡された行のすべてに、名前を 1 つずつ返す。渡していない行、空の名前、同じ行の 2 つ以上の名前、決まっていない項目は、誤りにする
- アノテーションの置き場所が誤っているなど、名前を返せないときは、0 でない終了コードで終わり、標準エラーに理由を書く。vanda は、それをエラーのメッセージに含める

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
- 審査の設定のファイルは、引数に書いた場所を、カレントディレクトリから読む
- `scope = "story"` の審査を含むなら、`--base <ブランチ>` が要る
- `agreement` と `evidence-present` を `scope = "story"` で行うときは、どのストーリーかを決める。ベースブランチから分かれた点より後のコミットのメッセージにある ID（設定の `id_pattern`）から決める。ID が 1 つならそれ。0 個か 2 つ以上なら、`--story <ID>` を求めるエラーにする。`--story` を渡せば、それを使う
- `story-origin` は、`scope = "story"` でもストーリーを決めない（ベースブランチから分かれた点より後のコミットを見るだけ）
- 読まれていないファイルの審査（`unread-files`）は、`.vanda/config.toml` と `.vanda/checks/*.toml` を除く

### エビデンスが揃っている審査（`evidence-present`）

予定したエビデンスの 1 件ごとに、その約束を指す、タイプに合うアダプタのアノテーションが 1 つ以上あれば揃っているとする。1 つのアノテーションが、同じ約束の複数の予定を満たしてよい。

| タイプ | 揃っているとするアノテーション |
|---|---|
| `scenario-test` | `txtar` の `@vanda:evidence` |
| `model-check` | `tla` の `@vanda:claim` |
| `guardrail` | `sql` の `@vanda:evidence` |
| `unit-test`・`cross-check`・`random-check`・`interface-test`・`concurrency-test` | `go` の `@vanda:evidence` |
| `sensor` | `go` の `@vanda:sensor` |
| `proof` | 段 1 では扱えない（予定に書いたら誤り） |
| `human-review`・`monitoring` | 確かめない |

## CLI

| コマンド | 中身 |
|---|---|
| `vanda version` | 動いている vanda の版を、1 行で出す（例: `0.1.0`）。`.vanda/` がなくても、版が合わなくても動く |
| `vanda doctor` | 設定（`.vanda/config.toml`）を読み、版が合うかを確かめる。誤りがあれば、標準エラーに出して終了コード 1 で終わる |
| `vanda check <審査の設定のファイル> [--base <ブランチ>] [--story <ID>]` | 審査を行う。どれかが失敗したら終了コード 1 |

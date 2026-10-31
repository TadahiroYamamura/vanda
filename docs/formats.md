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
- 空の ID（`id = ""`）は、形の誤り
- 重なった ID の誤りは、表の種類を問わず、出てきた順（ファイルの場所の順、同じファイルなら行の順）で後に出てきたほうの行を指し、前に出てきた場所をメッセージに書く
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

審査ごとに書ける範囲は次のとおり。ほかの組み合わせは、審査の設定の誤りにする。審査が 1 つもない設定も誤り。

| 審査 | 書ける範囲 |
|---|---|
| `promise-list-shape`・`annotation-links`・`agreement-targets`・`unread-files` | `all` |
| `story-origin` | `story` |
| `agreement`・`evidence-present` | `all`・`story` |

- 書いた順に行う。どれかが失敗したら、終了コードを失敗にする
- 審査の設定のファイルは、引数に書いた場所を、カレントディレクトリから読む
- `scope = "story"` の審査を含むなら、`--base <ブランチ>` が要る
- `agreement` と `evidence-present` を `scope = "story"` で行うときは、どのストーリーかを決める。ベースブランチから分かれた点より後のコミットのメッセージにある ID（設定の `id_pattern`）から決める。ID が 1 つならそれ。0 個か 2 つ以上なら、`--story <ID>` を求めるエラーにする。`--story` を渡せば、それを使う
- `story-origin` は、`scope = "story"` でもストーリーを決めない（ベースブランチから分かれた点より後のコミットを見るだけ）
- `--story` の値は、`id_pattern` に全体が当たらなければ誤り。範囲がすべて `all` の設定に `--base` を渡したときは、使わずに通す
- 「1 つのストーリーで変わったもの」は、分かれた点（`git merge-base <ベースブランチ> HEAD`）の中身と、作業ツリーの今の中身を比べて決める。分かれた点の中身は `git show` で読み、パターンは今の設定のものを当てる。約束・用語・機能は、指紋が変わったものと増えたもの。合意の対象のファイルは、中身が変わったものと増えたもの

### 出力

- 審査の設定の順に、審査ごとに `審査 <名前>（<範囲>）: 通った` か `審査 <名前>（<範囲>）: 失敗（<件数> 件）` の行を、標準出力に出す（通っても失敗しても）。誤りの中身は、標準エラーに 1 件 1 行で出す（審査の設定の順、同じ審査の中ではファイルの場所と行の順）。ストーリーを決めた審査では、範囲の後にストーリーの ID を書く（例: `審査 agreement（story #24）: 通った`）
- どれかが失敗したら、終了コード 1
- 設定の誤り、版の違い、審査の設定の誤り、`--base`・`--story` の誤りは、審査を始める前に標準エラーに出して、終了コード 1 で終わる
- 約束リストの読み込みの誤りは `promise-list-shape` が、アノテーションの読み込みの誤りは `annotation-links` が、合意の記録の読み込みの誤りは `agreement-targets` が出す。それを前提にするほかの審査は、「<何>が読めないため行えない」の 1 行で失敗にする

### 審査ごとの中身

| 審査 | 確かめること |
|---|---|
| `promise-list-shape` | 読み込みの誤り（TOML、型、必須の項目、決まっていない表と項目）。ID の形と重なり。`part_of` の先が用語にあり、循環しない。`subjects` の先が用語か機能にある。`layers` が設定のレイヤーにある。エビデンスの `layers` が約束の `layers` の中にある。約束のどのレイヤーにもエビデンスがある。`stage`・`type`・`flags` が決まった値。`formal_waiver` の決まり |
| `annotation-links` | アノテーションの読み込みの誤り。`term` は用語を、`feature` は機能を、`claim`・`evidence`・`sensor` は約束を指す。TLA+ の仕様の型（下）に `term` か `internal` があり、`feature` がない。`term` も `internal` もない誤りは型の定義の行を、`feature` を付けた誤りはそのアノテーションの行を指す |
| `story-origin` | 分かれた点より後の、合流でないコミットのうち、約束リストのファイル（今のパターンで決める）を変えたものの、メッセージに `id_pattern` に当たるものがある |
| `agreement` | 合意の対象（約束・用語・機能と、`txtar`・`tla` のアノテーションのパターンに当たるファイルと、`[[guardrail]]` の `paths` に当たるファイル。Go のファイルは対象にしない）の今の中身が合意済。約束に `flags`・`guesses` が残っていない。範囲 `story` では、そのストーリーで変わったものだけを見て、さらに、そのストーリーの外れた合意（下）も誤りにする |
| `agreement-targets` | 合意の記録の読み込みの誤り。`kind` が `claim`・`term`・`feature` の合意が、約束リストにある項目を指す |
| `evidence-present` | 下の「エビデンスが揃っている審査」。範囲 `story` では、そのストーリーで変わった約束だけを見る |
| `unread-files` | `.vanda/` のファイル（`.vanda/config.toml` と `.vanda/checks/*.toml` を除く）が、`promises` か `agreements` のパターンに当たる |

### 外れた合意

ストーリー s の外れた合意は、s が合意したことがあり、今の中身に s の合意がないもの。誰が変えたかを問わない（s 自身が変えたものも、s が合意し直すまで外れたまま。ほかのストーリーが今の中身に合意しても外れたまま）。

- 約束・用語・機能: s の合意の記録の `kind`・`key` で今の項目を探し、今の指紋での s の合意がなければ外れている
- ファイル: s の合意の記録の `path` で今のファイルを探し、今の中身の指紋での s の合意がなければ外れている（名前を変えたファイルは追わない）

### TLA+ の仕様の型

`tla` のアダプタのファイルで、`TypeOK ==` から次の最上位の定義（行の先頭から書いた定義）か `====` までのうち、`\in`・`\subseteq` の右で、次の `/\`・`\/` か行の終わりまでに出てくる識別子で、同じモジュールの最上位の定義の名前のものを、仕様の型とする。`CONSTANT` の集合は対象にしない。`TypeOK` のないファイルには、何も求めない。

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
| `vanda doctor` | 設定（`.vanda/config.toml`）を読み、版が合うかを確かめる。ストーリーの置き場所が設定にあれば、繋がるかも確かめる（GitHub Issues なら、認証の鍵で `GET <api_url>/repos/<repository>` を呼び、Issues が使えるか。外部のコマンドなら、コマンドがあるか）。通れば標準出力に 1 行を出す（置き場所を確かめたら、同じ行の終わりに、何を確かめたかを書き足す）。誤りがあれば、標準エラーに出して終了コード 1 で終わる |
| `vanda check <審査の設定のファイル> [--base <ブランチ>] [--story <ID>]` | 審査を行う。審査ごとの結果の行は標準出力に、誤りの中身は標準エラーに出す。どれかが失敗したら終了コード 1 |

## GUI の API（`vanda serve`）

v1.0.0 からは変えない取り決めではないが、画面とテストが頼るので、形をここに書く。

### 共通

- `vanda serve [--port <ポート>] [--no-open]` で起動する。`127.0.0.1` でだけ待ち受ける。`--port` を省けば（`0` なら）空いているポートを選ぶ。開いた URL を、`--no-open` のときも必ず標準出力に 1 行で出す。`--no-open` でなければブラウザで開く
- `Host` が `127.0.0.1:<ポート>`・`localhost:<ポート>` でない要求は `403` で断る。`GET` 以外で、`Origin` があって `http://<Host>` でない要求も `403` で断る
- 中身は UTF-8 の JSON。ストーリーの ID は URL の中で `%23` のようにエンコードする（`#24` → `%2324`）
- 誤りは、HTTP の状態コードと、次の形で返す。`message` は何がいけないか、`fix` はどう直すか（なければ省く）

```json
{"error": {"message": "…", "fix": "…"}}
```

| 状態コード | いつ |
|---|---|
| `400` | 引数の誤り（ないベースブランチ、`id_pattern` に合わないストーリーの ID など） |
| `404` | ないもの（ない約束、ないファイル、ないストーリー） |
| `409` | 合意を断ったとき（表示した後に中身が変わった、合意で変えるファイルに書きかけの変更がある） |
| `500` | vanda の中の誤り（一覧の数と差分の数が合わない、git が失敗したなど）と、今の約束リストか合意の記録に読み込みの誤りがあって一覧を作れないとき（`message` に誤りの数と最初の 1 件を入れる） |
| `502` | ストーリーの置き場所の誤り（認証、ネットワーク）。`message` に置き場所が返した理由をそのまま入れる |

- 対象の種類（`kind`）は、合意の記録と同じ `claim`・`term`・`feature`・`file`
- ファイルの種類（`fileType`）は、`scenario`（`txtar` のアノテーションのパターンに当たる）・`oracle`（`tla`）・`guardrail`（`[[guardrail]]` の `paths`）

### `GET /api/context`

今の設定と、選べるもの。画面を開いたときに読む。

```json
{
  "version": "0.1.0",
  "branch": "story-24",
  "head": "0123abcd…",
  "defaultBase": "origin/develop",
  "bases": ["develop", "main", "origin/develop", "origin/main"],
  "user": {"name": "Tadahiro Yamamura", "email": "…"},
  "stories": [{"id": "#24", "inBranch": true}, {"id": "#12", "inBranch": false}]
}
```

| 項目 | 中身 |
|---|---|
| `branch` | 今のブランチ。ブランチにいなければ空 |
| `defaultBase` | 設定の `[gui]` の `base`。書いていなければ空 |
| `bases` | 手元のブランチと、リモートを追うブランチ（`refs/heads`・`refs/remotes`。`origin/HEAD` は除く）。名前の順 |
| `user` | git の `user.name`・`user.email`。どちらかがなければ `null`（合意できない） |
| `stories` | `HEAD` の履歴の、合流でないコミットのメッセージに出てくるストーリーの ID（`id_pattern`）。新しいコミットに出てくる順。`inBranch` は、`defaultBase` から分かれた点より後のコミットに出てくるか |

### `GET /api/story?id=<ストーリーの ID>`

ストーリーの置き場所から読んだもの（CM-08）。

```json
{
  "id": "#24",
  "title": "タスクの一覧・完了・中止を追加する",
  "body": "…（Markdown のまま）",
  "url": "https://github.com/<owner>/<repo>/issues/24",
  "attachments": [{"name": "画面.png", "url": "https://github.com/user-attachments/assets/…"}]
}
```

- 置き場所が設定にないときと、ないストーリーは `404`。認証・ネットワーク・アダプタの誤りは `502`（`message` に、置き場所かアダプタが返した理由をそのまま入れる）
- 番号は、ストーリーの ID の中の、最後の数字の並び

### `GET /api/review`

ストーリーで変わったものの一覧（RV-01・02・04・05・10・13・14）。

| 引数 | 中身 |
|---|---|
| `view` | `branch`（ブランチごと）か `story`（ストーリー全体） |
| `base` | `view=branch` のとき必ず。ベースブランチ |
| `story` | ストーリーの ID。`view=story` のとき必ず。`view=branch` で省けば、`vanda check` と同じ決め方（分かれた点より後のコミットのメッセージにある ID が 1 つならそれ）で決め、決まらなければ `story` を `null` にする（合意と外れた合意は出せない） |

```json
{
  "view": "branch",
  "base": "origin/develop",
  "mergeBase": "0123abcd…",
  "story": "#24",
  "total": 3,
  "entries": [
    {
      "no": 1,
      "kind": "claim",
      "id": "task-id-unique",
      "path": null,
      "oldPath": null,
      "fileType": null,
      "change": "modified",
      "state": "proposed",
      "fingerprint": "sha256:…",
      "flags": ["ai-proposal", "changed-since-agreement"],
      "group": "layer:model",
      "before": {"statement": "…", "rationale": "…", "condition": null, "subjects": ["task-id"], "layers": ["model"], "evidence": [{"stage": "test", "type": "unit-test", "layers": ["model"], "detail": null}], "flags": [], "guesses": []},
      "after": {"statement": "…", "rationale": "…", "condition": null, "subjects": ["task-id"], "layers": ["model"], "evidence": [], "flags": ["ai-proposal"], "guesses": []},
      "detachedAgreement": null,
      "otherCommits": []
    },
    {
      "no": 2,
      "kind": "file",
      "id": null,
      "path": "e2e/add-task.txtar",
      "oldPath": "e2e/add.txtar",
      "fileType": "scenario",
      "change": "renamed",
      "state": "agreed",
      "fingerprint": "sha256:…",
      "flags": [],
      "group": "files",
      "before": {"text": "…"},
      "after": {"text": "…"},
      "detachedAgreement": null,
      "otherCommits": []
    }
  ],
  "detached": []
}
```

| 項目 | 中身 |
|---|---|
| `mergeBase` | `view=branch` のときの分かれた点のコミット。`view=story` では `null` |
| `total` | `entries` の数（RV-02 の総数） |
| `entries[].no` | 通し番号（1 から）。並びは `group` の順（下）、同じまとまりの中は ID か場所の順 |
| `entries[].id`・`path` | 約束・用語・機能は `id`、ファイルは `path`（削除したものは前の場所） |
| `entries[].oldPath` | `change` が `renamed` のときの前の場所。ほかは `null` |
| `entries[].change` | `added`・`modified`・`deleted`・`renamed` |
| `entries[].state` | 今の中身が合意済なら `agreed`、まだなら `proposed`。`deleted` は `null`（合意の対象がないので、合意できない） |
| `entries[].fingerprint` | 今の中身の指紋。`deleted` は `null` |
| `entries[].flags` | `ai-proposal`（AI 案。約束の `flags`）・`guess`（推測。約束の `guesses` が空でない）・`changed-since-agreement`（差分あり。同じ鍵（ファイルなら同じ場所）に合意の記録があり、今の中身の指紋に合う合意がない）。この順 |
| `entries[].group` | たたむまとまり（RV-10）。約束は `layer:<約束の layers の最初のレイヤー>`、用語と機能は `terms-features`、ファイルは `files`。並びは、設定のレイヤーの順の約束、`terms-features`、`files` |
| `entries[].before`・`after` | 変更前（`added` は `null`）と変更後（`deleted` は `null`）の中身。下の表 |
| `entries[].detachedAgreement` | このストーリーの合意が外れたもの（RV-14）なら `{"by": ["#30"]}`（`by` は、そのストーリーの合意の後に中身を変えたコミットのストーリーの ID。ID のないコミットは数えない）。ほかは `null`。`by` は見せるための手がかりで、外れたかどうかの判定には使わない。「合意の後」は、合意の記録の `agreed_at` とコミットの日時を比べて決めるので、コミットの日時を書き換えた履歴では外れることがある |
| `entries[].otherCommits` | `view=story` で、ストーリーで最初に変えた後に、このストーリーの ID のないコミットが同じものを変えていたら、`[{"commit": "…", "subject": "…", "stories": ["#30"]}]`。`view=branch` では空 |
| `detached` | このストーリーの外れた合意のうち、`entries` に出ないもの（比べる元と今のあいだで変わっていないもの）。形は `entries` と同じで、`no` は `null`、`change` は `unchanged`、`before` は `null`。`total` に数えない |

`before`・`after` の中身（項目は、約束リストの書いたとおり。省いた項目は `null`、配列は `[]`）:

| `kind` | 中身 |
|---|---|
| `claim` | `statement`・`rationale`・`condition`・`subjects`・`layers`・`evidence`（`stage`・`type`・`layers`・`detail` の配列）・`flags`・`guesses` |
| `term` | `name`・`definition`・`partOf` |
| `feature` | `name`・`definition`・`core`・`formal`・`formalWaiver` |
| `file` | `text`（ファイルの中身） |

- 一覧は、比べる元の中身と今の中身で指紋が違う鍵（約束・用語・機能は種類と ID、ファイルは場所）を、1 度ずつ出す。中身が同じで場所だけが変わったファイル（比べる元にあって今ない場所と、今あって比べる元にない場所で、中身の指紋が同じもの）は、`renamed` の 1 件にまとめる。中身も場所も変わったファイルは、`deleted` と `added` の 2 件にする
- サーバーは、一覧を返す前に、`entries` の数が、比べる元と今で指紋の違う鍵の数から `renamed` の数を引いたものに合うかを確かめ、合わなければ `500` にする
- `view=branch` の比べる元は、`vanda check` と同じく `git merge-base <base> HEAD` の中身。今の中身は作業ツリー
- `view=story` の比べる元は、鍵ごとに、そのストーリーの ID を持つコミット（合流でないもの）のうち、その鍵の中身を変えた最初のコミットの、1 つ目の親の中身。今の中身は作業ツリー。比べる元と今の中身が同じもの（途中で足して後で消したものなど）は出ない

### `POST /api/agree`

1 件に合意する（RV-06）。

```json
{"kind": "claim", "id": "task-id-unique", "path": null, "fingerprint": "sha256:…", "story": "#24"}
```

| 項目 | 中身 |
|---|---|
| `id`・`path` | 約束・用語・機能は `id`、ファイルは今の `path` |
| `fingerprint` | 画面に見せていた中身の指紋（一覧の `fingerprint`）。今の中身の指紋と違えば `409` で断る |
| `story` | 合意するストーリーの ID（`id_pattern` に全体が当たる） |

返すもの（`200`）:

```json
{"commit": "0123abcd…", "agreement": {"kind": "claim", "key": "task-id-unique", "fingerprint": "sha256:…", "path": null, "user": {"name": "…", "email": "…"}, "agreedAt": "2026-10-31T12:00:00+09:00", "story": "#24"}}
```

- 合意の記録は、設定の `agreements` のパターンの順で、最初に当たったファイルの終わりに書き足す。どれにも当たらず、最初のパターンにワイルドカードがなければ、そのファイルを作る。ワイルドカードがあれば `409` で断る
- 約束なら、約束リストのその約束の表から、`flags` と `guesses` の行（複数行の配列なら、その全部の行）を消す。ほかの行は変えない。インラインの表（`{ … }`）の中に書いた `flags`・`guesses` と、複数行の文字列（`"""`・`'''`）を含む `flags`・`guesses` は消せないので、`409` で断る
- 合意の記録のファイルと約束リストのファイルだけを、1 つのコミットにする（`git commit -- <ファイル>`）。ほかのファイルの書きかけの変更（索引に足したものも）は入れない。メッセージは `(chore) <ストーリーの ID> で <種類> <ID か場所> に合意する`（種類は `約束`・`用語`・`機能`・`ファイル`）
- 合意の対象が書いてあるファイル（約束・用語・機能なら、その項目のある約束リストのファイル。ファイルなら、そのファイル）に、コミットしていない変更があれば `409` で断る。理由には、先にコミットするよう書く（合意のコミットの時点のリポジトリが、合意した中身と合うようにするため）
- 合意で変えるファイルに、コミットしていない変更があれば `409` で断る
- 同じストーリーで、今の中身にもう合意していれば `409` で断る
- 合意の記録は、`user = { name = "…", email = "…" }` の形で書く。`agreed_at` は、合意した時の手元の時差の付いた日時（秒まで）
- 書いた後に約束リストと合意の記録を読み直し、合意が今の中身に合わない、フラグのほかの項目が変わったなどがあれば、書いたファイルを元に戻して `500` にする
- git の `user.name` か `user.email` がなければ `400` で断る

### `GET /api/claims/<約束の ID>`

約束の詳細（CL-01・02・04・05）。

```json
{
  "id": "task-id-unique",
  "statement": "…", "rationale": "…", "condition": null,
  "layers": [{"id": "model", "name": "モデル"}],
  "subjects": [{"kind": "term", "id": "task-id", "name": "タスクの ID"}],
  "state": "agreed",
  "fingerprint": "sha256:…",
  "flags": [], "guesses": [],
  "evidence": [
    {
      "stage": "test", "type": "model-check", "layers": ["model"], "detail": null,
      "found": [{"adapter": "tla", "file": "spec/Task.tla", "line": 12, "name": "IdsUnique", "text": "IdsUnique ==\n  …"}]
    }
  ],
  "origin": [{"commit": "0123abcd…", "date": "2026-10-31T12:00:00+09:00", "subject": "(feat) …", "stories": ["#24"], "change": "added"}],
  "agreements": [{"user": {"name": "…", "email": "…"}, "agreedAt": "…", "story": "#24", "fingerprint": "sha256:…", "current": true}]
}
```

| 項目 | 中身 |
|---|---|
| `evidence[].found` | その約束を指す、タイプに合うアノテーション（「エビデンスが揃っている審査」の表と同じ組み合わせ）。`text` は、`txtar` と `sql` ならファイルの中身全体、`tla` なら付いている定義（定義の行から、次の最上位の定義か `====` の前まで）、`go` なら `null` |
| `subjects` | 約束の `subjects` の順。約束リストにない ID は、`kind`・`name` を `null` にする |
| `origin` | HEAD の履歴の、合流でないコミットのうち、約束の指紋が変わったもの（作ったものを含む）の、古い順。`change` は `added`・`modified`。`stories` はメッセージにあるストーリーの ID。消した後に同じ ID で作り直した約束は新しいものとみなし、消すまでのコミットを入れない |
| `agreements` | 合意の記録のうち、`kind = "claim"` でこの ID を指すもの。合意の記録のファイルに書いた順。`current` は、今の中身の指紋に合うか |

- アノテーションに読み込みの誤りがあれば、`500` を返し、誤りの数と最初の 1 件を出す

### `GET /api/file?path=<場所>`

合意の対象のファイルとアノテーションを読むファイルの中身（TLA+ のファイル全体へのリンクに使う）。`{"path": "…", "text": "…"}`。ほかのファイルは `404`。

## TLA+ の記号（CL-07）

画面では、オラクルの定義（TLA+）の次の書き方を、右の記号に置き換えて見せる。文字列（`"…"`）とコメント（`\*`、`(* *)`）の中と、4 つ以上の `-`・`=` だけの行の区切りは置き換えない。長いものから先に当てる。`\` で始まる書き方は、後に英字・数字・`_` が続かないときだけ当てる（`\in` を `\intersect` に当てない）。コピーしたときは、リポジトリの文字のまま（ASCII）にする。置き換えは見せ方だけで、判定には使わない（画面の側で行う）。

| 書き方 | 記号 | 書き方 | 記号 |
|---|---|---|---|
| `==` | `≜` | `\in` | `∈` |
| `/\`・`\land` | `∧` | `\notin` | `∉` |
| `\/`・`\lor` | `∨` | `\subseteq` | `⊆` |
| `~`・`\lnot`・`\neg` | `¬` | `\cup`・`\union` | `∪` |
| `=>` | `⇒` | `\cap`・`\intersect` | `∩` |
| `<=>`・`\equiv` | `⇔` | `\X`・`\times` | `×` |
| `\A` | `∀` | `|->` | `↦` |
| `\E` | `∃` | `->` | `→` |
| `#`・`/=` | `≠` | `<<`・`>>` | `⟨`・`⟩` |
| `<=`・`=<`・`\leq` | `≤` | `[]` | `□` |
| `>=`・`\geq` | `≥` | `<>` | `◇` |
| `'` | `′` | `~>` | `↝` |
| `\o`・`\circ` | `∘` | `\div` | `÷` |

## 設定の `[story]` と `[gui]`（段 2 で足したもの）

```toml
[story]
id_pattern = "#[0-9]+"
adapter = "github"
repository = "TadahiroYamamura/vanda"
api_url = "https://api.github.com"

[gui]
base = "origin/develop"
```

| 項目 | 型 | 必ずか | 中身 |
|---|---|---|---|
| `[story]` の `adapter` | 文字列 | `command` とどちらか 1 つ。どちらもなければ、画面にストーリーの題・本文を出さない | 中に持つストーリーの置き場所のアダプタ。今は `github`（GitHub Issues）だけ |
| `[story]` の `repository` | 文字列 | `adapter = "github"` のときは必ず | `<owner>/<repo>` |
| `[story]` の `api_url` | 文字列 | 省いてよい。`adapter = "github"` のときだけ書ける | GitHub の REST API の URL。省けば `https://api.github.com`（GitHub Enterprise なら、その API の URL） |
| `[story]` の `command` | 文字列の配列 | 同上 | 外部のコマンドのアダプタと引数 |
| `[gui]` の `base` | 文字列 | 省いてよい | 画面を開いたときのベースブランチ（CM-06） |

### GitHub Issues のアダプタ

- 認証の鍵は設定に書かない。使うたびに、環境の変数 `GH_TOKEN` があればそれを、なければ `gh auth token` の出力を使う
- `GET <api_url>/repos/<repository>/issues/<番号>` を呼ぶ。番号は、ストーリーの ID の数字の部分
- 添付ファイルは、本文の中のリンクと画像（Markdown の `[名前](URL)`・`![名前](URL)` と、HTML の `<img src>`）のうち、URL が `https://github.com/user-attachments/`・`https://github.com/<repository>/assets/`・`https://user-images.githubusercontent.com/` で始まるもの。名前は、リンクの文字か画像の代わりの文字（なければ URL の最後の部分）

### 外部のコマンドのアダプタ（ストーリーの置き場所）

アノテーションの外部のコマンドのアダプタと同じく、リポジトリの根を作業のディレクトリにして動かし、標準入力に JSON を渡し、標準出力から JSON を受け取る。`action` は、段 2 では `read` だけ（コメントを書く `write` は段 4 で足す）。

```json
{"action": "read", "story": "#24"}
```

```json
{"title": "…", "body": "…", "url": "…", "attachments": [{"name": "…", "url": "…"}]}
```

- ストーリーがない・認証が通らないなどで返せないときは、0 でない終了コードで終わり、標準エラーに理由を書く。vanda は、それを画面にそのまま出す
- 決まっていない項目は誤りにする。`title`・`body`・`url` は必ず（`url` が分からなければ空の文字列）。`attachments` は省いてよい

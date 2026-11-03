# vanda のファイルと受け渡しの形

約束リスト（`.vanda/promises.toml`）と `docs/spec.md` で決めた中身を、外の人（アダプタを作る人など）が読めるよう、どの名前・どの形で書くかをまとめる。`.vanda/` の外の取り決め（アノテーションの書き方、アダプタとの受け渡しの形）は、v1.0.0 からは変えない。

## エラーの形

- 1 件を 1 行で出す。最初の 1 件で止めず、見つけたものを全部出す
- 出す順は毎回同じにする。ファイルの場所の順、同じファイルなら行の順（行のないものは、そのファイルの先）。審査をまたぐときは、審査の設定に並べた順を先にする
- 形は `<パス>:<行>: <何がどうなっているか>。<どうすればよいか>。`（です・ます調。例: `.vanda/promises.toml:21: 約束 task-add-saves-todo の今の中身に、まだ合意していません（提案中。指紋 sha256:…）。中身を確かめてから合意し、合意の記録に書いてください。`）。直し方がないときは `<パス>:<行>: <何がどうなっているか>。`。どの誤りも「。」で終わる。パスはリポジトリの直下からの `/` 区切りの場所。行は 1 から数える
- 行が見つからないとき（インラインの表 `{ … }` の中の項目など）は、行を書かず、ID と項目で場所を示す: `<パス>: <場所>: <何がどうなっているか>。<どうすればよいか>。`（例: `.vanda/promises.toml: claim "task-unique" の evidence の 2 件目 の layers: …`）。推測の行は出さない
- 項目がないときは、その項目を持つべき表の見出しの行を示す

## `.vanda/` の場所

`.vanda/` は、git のリポジトリの根（`git rev-parse --show-toplevel`）で探す。git のリポジトリの外で動かしたら誤りにする。

組込みのほかのアダプタ（アノテーション、ストーリーの置き場所）は、`.vanda/adapters/` の直下に置く。直下のファイルの名前、またはディレクトリの名前が、アダプタの名前になる。設定には `adapter = "<名前>"` と名前だけを書く。

- ファイルなら、そのファイルを実行する。ディレクトリなら、その直下の `run` を実行する。どちらも、実行の権限と、動かし方を決める 1 行目（例: `#!/usr/bin/env python3`）が要る
- 設定の `args`（文字列の配列、省いてよい）を、そのまま引数として渡す
- 組込みのアダプタと名前が重なったら、`.vanda/adapters/` のアダプタを使う
- `.vanda/adapters/` の下のファイルは、vanda の版を上げるときの移行のコマンドが書き換えない

## 読む中身

`vanda check` と `vanda serve`（画面と API）は、設定（`.vanda/config.toml`）・約束リスト・合意の記録と、アノテーションを読むファイルを、HEAD のコミットの中身で読む。コミットしていないファイルと変更（`git add` したものを含む）は読まない。コミットが 1 つもなければ、どのファイルもないものとして読む（設定がない誤りになる）。

- `vanda doctor` は、作業ツリーの設定を読む
- `.vanda/adapters/` のアダプタ（実行するファイル）と、`vanda check` に渡す審査の設定のファイルは、作業ツリーのものを使う
- 合意と取り消し（`POST /api/agree`・`POST /api/revoke`）は、作業ツリーのファイルを書き換えてコミットする。書き換えるファイルと合意の対象が書いてあるファイルにコミットしていない変更があれば断るので、合意する中身は HEAD の中身と同じになる

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
types = ["unit-test", "sensor"]

[[annotation]]
paths = ["docs/**/*.md"]
adapter = "md-annotation"
args = ["--strict"]
types = ["scenario-test"]

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
| `[[annotation]]` | 表の配列 | 省いてよい | アノテーションを読むファイルと、使うアダプタ。`paths`（文字列の配列、必ず）、`adapter`（アダプタの名前。組込みの `go`・`txtar`・`tla`・`sql`・`ts` か、`.vanda/adapters/` のアダプタ）、`args`（`.vanda/adapters/` のアダプタに渡す引数。省いてよい）、`types`（文字列の配列、必ず。そのパターンで読んだアノテーションが満たすエビデンスのタイプ。下の「エビデンスが揃っている審査」の表のタイプだけを書ける。空の配列なら、どのタイプも満たさない）。`adapter` を書かなければ、アダプタが決まっていないとみなし、当たったファイルを審査で止める。名前のアダプタが組込みにも `.vanda/adapters/` にもなければ誤り |
| `[[guardrail]]` | 表の配列 | 省いてよい | ガードレールの定義の取り出し。`adapter`（今は `file` だけ。ファイルを読むだけ）と `paths`（文字列の配列）。ガードレールの定義と約束の結びつけは、アノテーション（SQL なら `--` のコメントの中の `@vanda:evidence <ID>`）で行う |
| `[story]` の `id_pattern` | 文字列 | 省いてよい | ストーリーの ID の書き方（コミットの `Story:` のトレーラーの値と、`--story` の値に当てる）。ID 全体に当たる正規表現（Go の RE2）。省けば `#[0-9]+`（GitHub Issues）。空の文字列に当たるものは書けない |

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
stage = "implementation"
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
| `[[claim.evidence]]` | `stage` | 文字列（`plan`・`implementation`・`release`・`operation`） | 必ず |
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

[[agreement]]
kind = "file"
key = "e2e/old-task.txtar"
deleted = true
path = "e2e/old-task.txtar"
user = { name = "…", email = "…" }
agreed_at = 2026-10-31T12:00:00Z
story = "#24"
```

| 項目 | 型 | 必ずか | 中身 |
|---|---|---|---|
| `kind` | 文字列 | 必ず | 対象の種類。`claim`・`term`・`feature`・`file` |
| `key` | 文字列 | 必ず | 約束・用語・機能なら ID。ファイルなら、中身の指紋（`fingerprint` と同じ値）。消したファイルへの合意なら、場所（`path` と同じ値） |
| `fingerprint` | 文字列 | 消したことへの合意のほかは必ず。消したことへの合意には書けない | 合意したときの中身の指紋 |
| `deleted` | 真偽 | 消したことへの合意だけ。`true` だけを書ける | 消したことへの合意（`fingerprint` の代わりに持つ）。消したものには中身も指紋もないので、ファイルは場所で指す |
| `path` | 文字列 | ファイルのときは必ず。それ以外は書けない | 合意したときのファイルの場所 |
| `user` の `name`・`email` | 文字列 | 必ず | 合意したときの git の `user.name`・`user.email` |
| `agreed_at` | 時差の付いた日時 | 必ず | 合意した日時 |
| `story` | 文字列 | 必ず | ストーリーの ID。コミットの `Story:` のトレーラーと同じ書き方（例: `#24`） |

- 今の中身の指紋に合う合意（`kind`・`key`・`fingerprint` がすべて合う。ファイルなら、`path` も今の場所に合う）が 1 つでもあれば合意済。ファイルは場所で見分けるので、合意済のファイルを写した先や、名前を変えた先は、その場所の今の中身への合意がなければ提案中
- 消したことへの合意（`kind`・`key`（ファイルなら `path`）が合い、`deleted = true`）があれば、今ないそのものは合意済。中身への合意と消したことへの合意は、互いに数えない
- 合意の記録は消さずに履歴として残す。指紋の合わなくなった記録も、今ないものを指す記録も残る
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
- コメントの中で、逆引用符で囲んだもの（例: `` `@vanda:term` ``）はアノテーションとして数えない。説明の文で書き方に触れるときに使う。同じ行に本物のアノテーションがあっても、1 行に 2 つ以上の `@vanda:` の誤りにしない（例: `@vanda:internal` の理由の文で、書き方に触れるとき）
- アノテーションとして数えるのは、そのアダプタが知っている言語のコメントの中の `@vanda:` だけ。文字列の中などコメントの外の `@vanda:` は、数えず、誤りにもしない。`.vanda/adapters/` のアダプタでは、vanda は言語が分からないので、`@vanda:` のある行をすべてアダプタに渡し、コメントの外の行はアダプタに `"annotation": false` で返してもらう

### 組込みのアダプタ

| `adapter` | 書ける種類 | 書ける所 | 付いているものの名前 |
|---|---|---|---|
| `go` | `evidence`・`sensor` | 数えるのは `//`・`/* */` のコメントの中だけ。`func` の宣言の直前のコメント（空行をはさまない doc comment）に書く。ほかの所のコメントは誤り | `func` の名前（テストなら `TestXxx`）。メソッドは `型.名前`（例: `Store.Add`） |
| `txtar` | `evidence` | 数えるのは、最初のファイルの区切り（`-- 名前 --` の行）より前の、`#` で始まるコメントの行だけ（testscript の命令の行は数えない）。ファイル全体に付く。区切りより後は中のファイルの中身なので、数えない | ファイルの名前から `.txtar` を除いたもの（testscript のサブテストの名前） |
| `tla` | `term`・`feature`・`claim`・`internal` | 数えるのは、コメント（`\*`、入れ子にできる `(* *)`）の中だけ。文字列の中は数えない | アノテーションの後の、空行でもコメントでもない最初の行の定義の名前（`Name ==`、`Name(…) ==`、関数の定義 `Name[…] ==`。前に `LOCAL` があってもよい）。その行が定義でなければ誤り |
| `sql` | `evidence` | 数えるのは、文字列（`'…'`）の外の `--` から行の終わりまでのコメントの中だけ | アノテーションの後の、空行でもコメント（`--` で始まる行）でもない最初の行の中身を、前後の空白を除いてそのまま |
| `ts` | `evidence` | 数えるのは `//`・`/* */` のコメントの中だけ。文字列・テンプレート文字列・正規表現の中は数えない。`test`・`it`・`describe` から始まる呼び出し（後ろに付く修飾は名前を決めずにすべて受け付ける。`.skip`・`.only`・`.todo`・`.each(…)(…)`、Playwright の `test.describe`・`test.describe.serial` など）の直前のコメント（空行をはさまない）に書く。`beforeEach`・`afterEach`・`beforeAll`・`afterAll`・`step` が付いた呼び出しの直前と、ほかの所のコメントは誤り | 最後の呼び出しの最初の引数（`.each(表)("名前", …)` なら `"名前"`）。文字列か、テンプレート文字列なら書いてあるとおりの文字（`${op} を…` はそのまま）。`describe`（`test.describe` を含む）の中なら、外側から順に ` > ` でつなぐ（例: `ReviewPage > 一覧を出す`）。名前が変数などで決まった文字でなければ誤り |

- 書ける種類のほかのアノテーションは、誤りにする。`.vanda/adapters/` のアダプタでは、どの種類も書ける
- Go の型には、`term` を求めない（Go は実装で、仕様ではないため）

### `.vanda/adapters/` のアダプタ

`.vanda/adapters/` に置いたアダプタ（上の「`.vanda/` の場所」）。vanda は、`@vanda:` の行のあるファイルごとに 1 回、リポジトリの根を作業のディレクトリにしてアダプタを動かす。

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
{"names": [{"line": 2, "name": "add-task の手順"}, {"line": 7, "name": "TestAddTask"}, {"line": 12, "annotation": false}]}
```

| 項目 | 中身 |
|---|---|
| `names[].line` | 渡された行 |
| `names[].name` | その行のアノテーションが付いているものの名前（空にしない） |
| `names[].annotation` | `false` だけを書ける。その行の `@vanda:` がコメントの外（文字列の中など）にあり、アノテーションではないことを示す。`name` とは一緒に書かない |

- 渡された行のすべてに、名前か `"annotation": false` のどちらか 1 つを返す。`"annotation": false` を返した行は、アノテーションとして数えない（組込みのアダプタと同じく、コメントの中のものだけをアノテーションとするため）。渡していない行、空の名前、同じ行の 2 つ以上の答え、名前と `annotation` の両方、決まっていない項目は、誤りにする
- アノテーションの置き場所が誤っているなど、名前を返せないときは、0 でない終了コードで終わり、標準エラーに理由を書く。vanda は、それをエラーのメッセージに含める

## コミットのストーリーの ID

コミットのストーリーの ID は、メッセージの末尾のトレーラー（`Co-Authored-By:` と同じ Git のトレーラー）の `Story: <ID>` から読む。subject と本文の中の ID は数えない。

```
(feat) タスクの一覧を追加する

一覧を見たいという要望があったため。

Story: #24
Co-Authored-By: …
```

- トレーラーは、メッセージの最後の段落（どの行も `キー: 値` か、空白で始まる続きの行）。1 つ目の段落（subject）はトレーラーとみなさない。キーの大文字と小文字は区別しない
- 1 つのコミットに書ける `Story:` は 1 つまで（1 つのブランチは 1 つのストーリーに当たり、コミットは 1 つのブランチで作るため）。2 つ以上は誤り
- 値は、設定の `id_pattern` に全体が当たるもの。当たらなければ誤り
- 誤りを出す所: 審査の設定にストーリーを決める審査（範囲 `story` の `agreement`・`evidence-present`）があれば、`vanda check` は、分かれた点より後のコミットにトレーラーの誤りがあるとき、審査を始める前の誤りにする（終了コード 1）。`--story` を渡したときも同じ（`--story` は、ストーリーが 1 つに決まらないときにどれを審査するかを選ぶためのもので、トレーラーの確かめを飛ばすためのものではない。手元と CI で結果をそろえるため）。範囲がすべて `all` の設定では、`--base` を渡しても分かれた点を使わないので、トレーラーも見ない。`story-origin` は、約束リストを変えたコミットのトレーラーの誤りを、その審査の失敗にする。画面（API）は、誤りで止めずに `200` で返し、トレーラーの誤ったコミットのストーリーを空にしたうえで、どのコミットの何が誤りかを `GET /api/context` の `trailerErrors` で返す。画面はそれを出す（黙って空にしない）

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
- `agreement` と `evidence-present` を `scope = "story"` で行うときは、どのストーリーかを決める。ベースブランチから分かれた点より後のコミットの `Story:` のトレーラー（下の「コミットのストーリーの ID」）から決める。ID が 1 つならそれ。0 個か 2 つ以上なら、`--story <ID>` を求めるエラーにする。`--story` を渡せば、それを使う
- `story-origin` は、`scope = "story"` でもストーリーを決めない（ベースブランチから分かれた点より後のコミットを見るだけ）
- `--story` の値は、`id_pattern` に全体が当たらなければ誤り。範囲がすべて `all` の設定に `--base` を渡したときは、使わずに通す
- 「1 つのストーリーで変わったもの」は、分かれた点（`git merge-base <ベースブランチ> HEAD`）の中身と、HEAD の中身を比べて決める。分かれた点の中身は `git show` で読み、パターンは今の設定のものを当てる。約束・用語・機能は、指紋が変わったもの、増えたもの、分かれた点にあって今ないもの。合意の対象のファイルは、中身が変わったもの、増えたもの、分かれた点にあって今ないもの

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
| `story-origin` | 分かれた点より後の、合流でないコミットのうち、約束リストのファイル（今のパターンで決める）を変えたものに、`Story:` のトレーラーがある。トレーラーの誤り（2 つ以上、`id_pattern` に合わない）も、そのコミットの誤りにする |
| `agreement` | 合意の対象（約束・用語・機能と、`types` に `scenario-test` か `model-check` を含む `[[annotation]]` のパターンに当たるファイルと、`[[guardrail]]` の `paths` に当たるファイル。どのアダプタで読むかによらない。`types` がどちらも含まないパターン（Go のテストなど）だけに当たるファイルは対象にしない）の今の中身が合意済。約束に `flags`・`guesses` が残っていない。中身に合意したことのある約束・用語・機能・合意の対象のファイルが今なければ、消したことへの合意がある。範囲 `story` では、そのストーリーで変わったものだけを見て（分かれた点にあって今ないものは、合意したことがあるかによらず、消したことへの合意を求める）、さらに、そのストーリーの外れた合意（下）も誤りにする |
| `agreement-targets` | 合意の記録の読み込みの誤り（`kind` が `file` の記録に `path` がない、`key` が `fingerprint`（消したことへの合意なら `path`）と違う、`kind` が `file` でない記録に `path` がある、など）。`kind` が `claim`・`term`・`feature` の合意が、約束リストにある項目か、消したことへの合意のある項目を指す |
| `evidence-present` | 下の「エビデンスが揃っている審査」。範囲 `story` では、そのストーリーで変わった約束だけを見る |
| `unread-files` | `.vanda/` のファイル（`.vanda/config.toml`、`.vanda/checks/*.toml`、`.vanda/adapters/` の下を除く）が、`promises` か `agreements` のパターンに当たる |

### 外れた合意

ストーリー s の外れた合意は、s が合意したことがあり、今の中身に s の合意がないもの。誰が変えたかを問わない（s 自身が変えたものも、s が合意し直すまで外れたまま。ほかのストーリーが今の中身に合意しても外れたまま）。

- 約束・用語・機能: s の合意の記録の `kind`・`key` で今の項目を探し、今の指紋での s の合意がなければ外れている
- ファイル: s の合意の記録の `path` で今のファイルを探し、今の中身の指紋での s の合意がなければ外れている（名前を変えたファイルは追わない）

### TLA+ の仕様の型

`tla` のアダプタのファイルで、`TypeOK ==` から次の最上位の定義（行の先頭から書いた定義）か `====` までのうち、`\in`・`\subseteq` の右で、次の `/\`・`\/` か行の終わりまでに出てくる識別子で、同じモジュールの最上位の定義の名前のものを、仕様の型とする。`CONSTANT` の集合は対象にしない。`TypeOK` のないファイルには、何も求めない。

- 読まれていないファイルの審査（`unread-files`）は、`.vanda/config.toml`、`.vanda/checks/*.toml`、`.vanda/adapters/` の下を除く

### エビデンスが揃っている審査（`evidence-present`）

予定したエビデンスの 1 件ごとに、その約束を指す、タイプに合うアノテーションが 1 つ以上あれば揃っているとする。タイプに合うアノテーションは、`types` にそのタイプを含む `[[annotation]]` で読んだ、下の表の種類のもの。1 つのアノテーションが、同じ約束の複数の予定を満たしてよい。

| タイプ | 揃っているとするアノテーションの種類 |
|---|---|
| `model-check` | `@vanda:claim` |
| `sensor` | `@vanda:sensor` |
| `cross-check`・`random-check`・`scenario-test`・`unit-test`・`interface-test`・`concurrency-test`・`guardrail` | `@vanda:evidence` |
| `proof` | 今は扱えない（予定に書いたら誤り。`types` に書けない） |
| `human-review` | 確かめない（`types` に書けない） |

組込みのアダプタは、既定のタイプを持つ（`vanda init` が `types` にそのまま書く）。

| `adapter` | 既定のタイプ |
|---|---|
| `go` | `unit-test`・`cross-check`・`random-check`・`interface-test`・`concurrency-test`・`sensor` |
| `txtar` | `scenario-test` |
| `tla` | `model-check` |
| `sql` | `guardrail` |
| `ts` | `unit-test` |

## CLI

| コマンド | 中身 |
|---|---|
| `vanda version` | 動いている vanda の版を、1 行で出す（例: `0.1.0`）。`.vanda/` がなくても、版が合わなくても動く |
| `vanda doctor` | 設定（`.vanda/config.toml`）を読み、版が合うかを確かめる。ストーリーの置き場所が設定にあれば、繋がるかも確かめる（GitHub Issues なら、`api_url` のホストに合う認証キーで `GET <api_url>/repos/<repository>` を呼び、Issues が使えるか。`.vanda/adapters/` のアダプタなら、そこにあって実行できるか）。通れば標準出力に 1 行を出す（置き場所を確かめたら、同じ行の終わりに、何を確かめたかを書き足す）。誤りがあれば、標準エラーに出して終了コード 1 で終わる |
| `vanda check <審査の設定のファイル> [--base <ブランチ>] [--story <ID>]` | 審査を行う。審査ごとの結果の行は標準出力に、誤りの中身は標準エラーに出す。どれかが失敗したら終了コード 1 |

## GUI の API（`vanda serve`）

v1.0.0 からは変えない取り決めではないが、画面とテストが頼るので、形をここに書く。

### 共通

- `vanda serve [--port <ポート>] [--no-open]` で起動する。`127.0.0.1` でだけ待ち受ける。`--port` を省けば（`0` なら）空いているポートを選ぶ。開いた URL を、`--no-open` のときも必ず標準出力に 1 行で出す（例: `vanda を起動しました: http://127.0.0.1:44927/（Ctrl+C で停止します）`）。`--no-open` でなければブラウザで開く
- `Host` が `127.0.0.1:<ポート>`・`localhost:<ポート>` でない要求は `403` で断る。`GET` 以外で、`Origin` があって `http://<Host>` でない要求も `403` で断る
- 中身は UTF-8 の JSON。ストーリーの ID は URL の中で `%23` のようにエンコードする（`#24` → `%2324`）
- 誤りは、HTTP の状態コードと、次の形で返す。`message` は何がどうなっているか（例: `約束 a の中身が、画面に表示した後に変わりました（…）。`）、`fix` はどうすればよいか（例: `一覧を読み直し、今の中身を確かめてから合意してください。`）。どちらも、です・ます調の文で、`。` で終える

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
- ファイルの種類（`fileType`）は、`scenario`（`types` に `scenario-test` を含む `[[annotation]]` のパターンに当たる）・`oracle`（`types` に `model-check` を含み、`scenario-test` を含まない）・`guardrail`（`[[guardrail]]` の `paths` にだけ当たる）

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
  "stories": [{"id": "#24", "inBranch": true}, {"id": "#12", "inBranch": false}],
  "layers": [{"id": "model", "name": "モデル"}],
  "storyStore": "github"
}
```

| 項目 | 中身 |
|---|---|
| `branch` | 今のブランチ。ブランチにいなければ空 |
| `defaultBase` | 設定の `[gui]` の `base`。書いていなければ空 |
| `bases` | 手元のブランチと、リモートを追うブランチ（`refs/heads`・`refs/remotes`。`origin/HEAD` は除く）。名前の順 |
| `storyStore` | ストーリーの置き場所のアダプタ。設定の `[story]` の `adapter` の名前（`github` か、`.vanda/adapters/` のアダプタの名前）。設定になければ空。空なら、画面はストーリーを読みに行かず、置き場所が設定にないことを注記で出す |
| `layers` | 設定のレイヤーの一覧（設定の順）。画面で、まとまりの名前に使う |
| `user` | git の `user.name`・`user.email`。どちらかがなければ `null`（合意できない） |
| `trailerErrors` | `HEAD` の履歴の、合流でないコミットのうち、`Story:` のトレーラーが誤ったもの（新しい順）。`[{"commit": "…", "subject": "…", "message": "Story: のトレーラーが 2 つあります（#4・#5）。1 つのコミットに書けるのは 1 つだけです。"}]`。そのコミットのストーリーは、ほかの項目（`stories` など）では空にする。なければ `[]` |
| `stories` | `HEAD` の履歴の、合流でないコミットの `Story:` のトレーラーにあるストーリーの ID。新しいコミットに出てくる順。`inBranch` は、`defaultBase` から分かれた点より後のコミットに出てくるか |

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
| `view` | レビュー単位（RV-13）。`branch`（ブランチ）か `story`（ストーリー） |
| `base` | `view=branch` のとき必ず。ベースブランチ。`view=story` では省いてよく、合意を取り消せるか（`entries[].revoke`）の「今のブランチ」を決めるためだけに使う（省けば設定の `[gui]` の `base`） |
| `story` | ストーリーの ID。`view=story` のとき必ず。`view=branch` で省けば、`vanda check` と同じ決め方（分かれた点より後のコミットの `Story:` のトレーラーにある ID が 1 つならそれ）で決め、決まらなければ `story` を `null` にする（合意と外れた合意は出せない） |

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
      "fileType": null,
      "change": "modified",
      "state": "proposed",
      "fingerprint": "sha256:…",
      "flags": ["ai-proposal", "changed-since-agreement"],
      "group": "layer:model",
      "before": {"statement": "…", "rationale": "…", "condition": null, "subjects": ["task-id"], "layers": ["model"], "evidence": [{"stage": "implementation", "type": "unit-test", "layers": ["model"], "detail": null}], "flags": [], "guesses": []},
      "after": {"statement": "…", "rationale": "…", "condition": null, "subjects": ["task-id"], "layers": ["model"], "evidence": [], "flags": ["ai-proposal"], "guesses": []},
      "detachedAgreement": null,
      "otherCommits": []
    },
    {
      "no": 2,
      "kind": "file",
      "id": null,
      "path": "e2e/add-task.txtar",
      "fileType": "scenario",
      "change": "modified",
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
| `entries[].change` | `added`・`modified`・`deleted` |
| `entries[].state` | 今の中身が合意済なら `agreed`、まだなら `proposed`。`deleted` は、消したことへの合意があれば `agreed`、なければ `proposed` |
| `entries[].fingerprint` | 今の中身の指紋。`deleted` は `null` |
| `entries[].flags` | `ai-proposal`（AI 案。約束の `flags`）・`guess`（推測。約束の `guesses` が空でない）・`changed-since-agreement`（差分あり。同じ鍵（ファイルなら同じ場所）に合意の記録があり、今の中身の指紋に合う合意がない）。この順 |
| `entries[].group` | たたむまとまり（RV-10）。約束は `layer:<約束の layers の最初のレイヤー>`、用語と機能は `terms-features`、ファイルは `files`。並びは、設定のレイヤーの順の約束、`terms-features`、`files` |
| `entries[].before`・`after` | 変更前（`added` は `null`）と変更後（`deleted` は `null`）の中身。下の表 |
| `entries[].detachedAgreement` | このストーリーの合意が外れたもの（RV-14）なら `{"by": ["#30"]}`（`by` は、そのストーリーの合意の後に中身を変えたコミットのストーリーの ID。ID のないコミットは数えない）。ほかは `null`。`by` は見せるための手がかりで、外れたかどうかの判定には使わない。「合意の後」は、日時でなく HEAD の履歴の順で決める。HEAD の履歴を古い順にたどり、そのストーリーのその鍵への合意の記録（中身への合意）が初めて現れたコミットを、記録ごとに探し、そのうち最も後のコミットより後のコミットを見る（rebase や amend でコミットの日時が変わっても変わらない。手で書いた合意の記録も、コミットした後は同じに扱う）。どの記録もまだコミットしていなければ、`by` は空 |
| `entries[].otherCommits` | `view=story` で、ストーリーで最初に変えた後に、このストーリーの ID のないコミット（ほかのストーリーのコミットと、ID のないコミット）が同じものを変えていたら、`[{"commit": "…", "subject": "…", "stories": ["#30"]}]`（履歴の古い順）。`view=branch` では空 |
| `entries[].revoke` | 合意済（`state` が `agreed`）の件だけ。今の人が、このストーリーの、今の中身への合意（`deleted` なら、消したことへの合意）を取り消せるか（RV-17）。`{"possible": true, "reason": null, "others": []}`。取り消せるのは、今の git の `user.name`・`user.email` の人が、今のブランチ（ベースブランチと分かれた点より後）で書いた合意の記録。取り消せないときは `possible` が `false` で、`reason` に短い理由を出す（ストーリーが決まっていない、git の人が分からない、このストーリーの合意がない、ほかの人が書いた、ベースブランチにすでにある、ベースブランチが決まっていない）。`others` は、取り消しても残る、同じストーリーの今の中身へのほかの人の合意（残れば合意済のまま）。ほかの件は `null` |
| `entries[].agree` | 今の人が、このストーリーで合意できるか（`deleted` なら、消したことに合意できるか）。`{"possible": true, "reason": null}`。合意済（`state` が `agreed`）でも、このストーリーの今の中身への合意がなければ合意できる（ストーリーの合意が外れた後に、ほかのストーリーが今の中身に合意していても、そのストーリーが合意し直すまで外れたままのため）。合意できないときは `possible` が `false` で、`reason` に短い理由を出す（ストーリーが決まっていない、git の人が分からない、このストーリーで今の中身（消したこと）にもう合意している、指紋を作れない）。`POST /api/agree` が断る条件と合わせる。`detached` の件も同じ |
| `branchStories` | レビュー単位がブランチ（`view=branch`）のとき、分かれた点より後のコミットの `Story:` のトレーラーにあるストーリーの ID。名前の順。`view=story` では `null` |
| `storyFrom` | `story` をどう決めたか。`query`（`story` で名指しした）・`branch`（ブランチのコミットから 1 つに決まった）。決まっていなければ空の文字列 |
| `storyChoice` | 画面でストーリーをどう示すか（RV-13）。`fixed`: レビュー単位がブランチで、ブランチのコミットからストーリーが 1 つに決まる（`story` で別のものを名指ししたときを除く）。選ばせずに示すだけにする。`branch`: レビュー単位がブランチで、ブランチのストーリーが 2 つ以上ある。`branchStories` から選ばせる。`all`: レビュー単位がストーリーのときと、ブランチのストーリーが見つからない（か、名指ししたものが違う）とき。すべてのストーリー（`/api/context` の `stories`）から選ばせる |
| `detached` | このストーリーの外れた合意のうち、`entries` に出ないもの（比べる元と今のあいだで変わっていないもの）。形は `entries` と同じで、`no` は `null`、`change` は `unchanged`、`before` は `null`。`total` に数えない |

`before`・`after` の中身（項目は、約束リストの書いたとおり。省いた項目は `null`、配列は `[]`）:

| `kind` | 中身 |
|---|---|
| `claim` | `statement`・`rationale`・`condition`・`subjects`・`layers`・`evidence`（`stage`・`type`・`layers`・`detail` の配列）・`flags`・`guesses` |
| `term` | `name`・`definition`・`partOf` |
| `feature` | `name`・`definition`・`core`・`formal`・`formalWaiver` |
| `file` | `text`（ファイルの中身） |

- 一覧は、比べる元の中身と今の中身で指紋が違う鍵（約束・用語・機能は種類と ID、ファイルは場所）を、1 度ずつ出す。ファイルは場所で見分けるので、名前を変えたファイルは、中身が同じでも、古い場所の `deleted` と新しい場所の `added` の 2 件にする（まとめない）。一度消して同じ場所に作り直したものは、同じ場所の 1 件として比べる（中身が同じなら出ない）
- サーバーは、一覧を返す前に、`entries` の数が、比べる元と今で指紋の違う鍵の数に合うか、同じ鍵が 2 度出ていないかを確かめ、合わなければ `500` にする
- `view=branch` の比べる元は、`vanda check` と同じく `git merge-base <base> HEAD` の中身。今の中身は HEAD の中身（コミットしていない変更は比べない）
- `view=story` の比べる元は、鍵ごとに、`Story:` のトレーラーがそのストーリーの ID のコミット（合流でないもの）のうち、その鍵の中身を変えた最初のコミットの、1 つ目の親の中身。今の中身は HEAD の中身。HEAD の履歴にないコミットは見ない。比べる元と今の中身が同じもの（途中で足して後で消したものなど）は出ない

#### 変更前と変更後の差分（`entries[].diff`）

`entries[]` に `diff` を足す。`before` と `after` の両方があるとき（`modified`）だけ作り、ほかは `null`。差分は `github.com/sergi/go-diff` で作り、画面は見せるだけにする。

ファイル（`kind` が `file`）:

```json
"diff": {
  "lines": [
    {"op": "equal",  "before": 1,    "after": 1,    "text": "exec vanda add 買い物", "parts": null},
    {"op": "delete", "before": 2,    "after": null, "text": "stdout '登録した'",     "parts": [{"op": "equal", "text": "stdout '"}, {"op": "delete", "text": "登録"}, {"op": "equal", "text": "した'"}]},
    {"op": "insert", "before": null, "after": 2,    "text": "stdout '足した'",       "parts": [{"op": "equal", "text": "stdout '"}, {"op": "insert", "text": "足"}, {"op": "equal", "text": "した'"}]},
    {"op": "insert", "before": null, "after": 3,    "text": "",                     "parts": null, "noNewline": true}
  ]
}
```

| 項目 | 中身 |
|---|---|
| `lines` | 行の単位の差分（`DiffLinesToChars` で行を 1 文字にして比べ、戻したもの）。変更前と変更後のすべての行を、ファイルの順に 1 度ずつ並べる。同じ所で消した行と足した行は、消した行を先に並べる |
| `lines[].op` | `equal`（両方にある）・`delete`（変更前にだけある）・`insert`（変更後にだけある） |
| `lines[].before`・`after` | 変更前と変更後の行の番号（1 から）。その側にない行は `null` |
| `lines[].text` | 行の中身（行の終わりの改行は含めない） |
| `lines[].parts` | 消した行が続く所のすぐ後に足した行が続くとき（書き換えた所）だけ、その消した行のまとまりと足した行のまとまりを文字の単位で比べ（`DiffMain` の後に `DiffCleanupSemantic` をかけ、細切れの一致をまとめる）、行ごとに分けたもの。消した行には `equal`・`delete` だけ、足した行には `equal`・`insert` だけが出る。つなげると `text` になる。書き換えでない行は `null` |
| `lines[].noNewline` | ファイルの最後の行で、終わりに改行がないときだけ `true`（ほかは省く）。改行の有無だけが違う最後の行は、`delete` と `insert` の 2 行になる |

約束・用語・機能（`kind` が `claim`・`term`・`feature`）:

```json
"diff": {
  "fields": {
    "statement": [{"op": "equal", "text": "ID は"}, {"op": "insert", "text": "絶対に"}, {"op": "equal", "text": "重ならない"}],
    "rationale": [{"op": "equal", "text": "理由"}]
  }
}
```

| 項目 | 中身 |
|---|---|
| `fields` | 文字の項目ごとの、文字の単位の差分（`DiffMain` の後に `DiffCleanupSemantic`）。`op` は `equal`・`delete`・`insert`。`equal` と `delete` をつなげると変更前、`equal` と `insert` をつなげると変更後になる |
| 対象の項目 | 約束は `statement`・`rationale`・`condition`、用語は `name`・`definition`、機能は `name`・`definition`・`formalWaiver`。変更前と変更後の両方に書いてある項目だけを入れ、片方にしかない項目は入れない（画面は、`before`・`after` の値をそのまま見せる）。変わっていない項目も、`equal` の 1 つとして入れる |

- 配列の項目（`subjects`・`layers`・`guesses`）と真偽の項目は、差分を作らない。画面は `before`・`after` を並べて見せる
- 約束のエビデンスの予定（`evidence`）は、エビデンスの単位の差分を `diff.evidence` に入れる（約束でなければ省く）

```json
"evidence": {
  "before": [{"op": "modified", "layers": [{"op": "equal", "text": "model"}], "detail": [{"op": "equal", "text": "追加すると一覧"}, {"op": "delete", "text": "に出る"}, {"op": "insert", "text": "の先頭に出る"}]}, {"op": "removed", "layers": null, "detail": null}],
  "after":  [{"op": "modified", "layers": [{"op": "equal", "text": "model"}], "detail": [{"op": "equal", "text": "追加すると一覧"}, {"op": "delete", "text": "に出る"}, {"op": "insert", "text": "の先頭に出る"}]}, {"op": "added", "layers": null, "detail": null}]
}
```

| 項目 | 中身 |
|---|---|
| `before`・`after` | `before.evidence`・`after.evidence` と同じ数・同じ順の、エビデンス 1 つずつの印 |
| 対応づけ | ステージ（`stage`）とタイプ（`type`）が同じものを、同じエビデンスとみなす。同じステージとタイプが 2 つ以上あれば、それぞれ現れた順に、1 つ目どうし、2 つ目どうし、と対応させる。対応の相手がないものは、変更前なら `removed`（消した）、変更後なら `added`（足した） |
| `op` | 対応したものは、レイヤーと詳細が同じなら `equal`、どちらかが違えば `modified`。ほかは上のとおり |
| `pair` | 対応したもう片方の側のエビデンスの添字（`before` の印なら `after.evidence` の添字）。`removed`・`added` は `null` |
| `layers`・`detail` | `modified` のときだけ。レイヤー（`layers` を「・」でつないだ文字）と詳細（ないときは空の文字）の、文字の単位の差分（`fields` と同じ形）。両側に同じものを入れ、画面は変更前の側で `equal`・`delete`、変更後の側で `equal`・`insert` を見せる。ほかは `null` |

### `POST /api/agree`

1 件に合意する（RV-06）。

```json
{"kind": "claim", "id": "task-id-unique", "path": null, "fingerprint": "sha256:…", "story": "#24", "deleted": false}
```

| 項目 | 中身 |
|---|---|
| `id`・`path` | 約束・用語・機能は `id`、ファイルは今の `path`（消したことへの合意なら、前の `path`） |
| `fingerprint` | 画面に見せていた中身の指紋（一覧の `fingerprint`）。今の中身の指紋と違えば `409` で断る。`deleted` が `true` なら見ない |
| `deleted` | 消したことに合意するなら `true`（省けば `false`）。そのものが今ある（約束リストにある、合意の対象のファイルとしてある）なら `409` で断る。ファイルの場所が、合意の対象のファイルのパターンに当たらなければ `404` |
| `story` | 合意するストーリーの ID（`id_pattern` に全体が当たる） |

返すもの（`200`）:

```json
{"commit": "0123abcd…", "agreement": {"kind": "claim", "key": "task-id-unique", "fingerprint": "sha256:…", "deleted": false, "path": null, "user": {"name": "…", "email": "…"}, "agreedAt": "2026-10-31T12:00:00+09:00", "story": "#24"}}
```

- 消したことへの合意では、返す `agreement` の `fingerprint` は `null`、`deleted` は `true`（ほかの合意では `false`）。合意の記録には `fingerprint` の代わりに `deleted = true` を書く。消した約束・用語・機能では、約束リストのどのファイル（消したファイルを含む）にもコミットしていない変更がないことを求め、あれば `409` で断る。コミットのメッセージは `(chore) <ストーリーの ID> で <種類> <ID か場所> を消したことに合意する`
- 合意の記録は、設定の `agreements` のパターンの順で、最初に当たったファイルの終わりに書き足す。どれにも当たらず、最初のパターンにワイルドカードがなければ、そのファイルを作る。ワイルドカードがあれば `409` で断る
- 約束なら、約束リストのその約束の表から、`flags` と `guesses` の行（複数行の配列なら、その全部の行）を消す。ほかの行は変えない。インラインの表（`{ … }`）の中に書いた `flags`・`guesses` と、複数行の文字列（`"""`・`'''`）を含む `flags`・`guesses` は消せないので、`409` で断る
- 合意の記録のファイルと約束リストのファイルだけを、1 つのコミットにする（`git commit -- <ファイル>`）。ほかのファイルの書きかけの変更（索引に足したものも）は入れない。メッセージは `(chore) <ストーリーの ID> で <種類> <ID か場所> に合意する`（種類は `約束`・`用語`・`機能`・`ファイル`）で、末尾に `Story: <ストーリーの ID>` のトレーラーを書く
- 合意の対象が書いてあるファイル（約束・用語・機能なら、その項目のある約束リストのファイル。ファイルなら、そのファイル）に、コミットしていない変更があれば `409` で断る。理由には、先にコミットするよう書く（合意のコミットの時点のリポジトリが、合意した中身と合うようにするため）
- 合意で変えるファイルに、コミットしていない変更があれば `409` で断る
- 同じストーリーで、今の中身にもう合意していれば `409` で断る
- 合意の記録は、`user = { name = "…", email = "…" }` の形で書く。`agreed_at` は、合意した時の手元の時差の付いた日時（秒まで）
- 書いた後に約束リストと合意の記録を読み直し、合意が今の中身に合わない、フラグのほかの項目が変わったなどがあれば、書いたファイルを元に戻して `500` にする
- git の `user.name` か `user.email` がなければ `400` で断る

### `POST /api/revoke`

1 件への合意を取り消す（RV-17）。

```json
{"kind": "claim", "id": "task-id-unique", "path": null, "fingerprint": "sha256:…", "story": "#24", "base": "origin/develop", "deleted": false}
```

返すもの（`200`）: `{"commit": "0123abcd…", "stillAgreed": false, "removed": 1}`

- `deleted` が `true` なら、消したことへの合意を取り消す（`fingerprint` は見ない）。そのものが今あれば `409` で断る。コミットのメッセージは `(chore) <ストーリーの ID> で <種類> <ID か場所> を消したことへの合意を取り消す`
- `base` は、今のブランチを決めるベースブランチ（省けば設定の `[gui]` の `base`）。`entries[].revoke` と同じ判定で、取り消せなければ `409` で断り、理由を出す
- 消すのは、今の人が今のブランチで書いた、このストーリーの今の中身への合意の記録（同じものが 2 つ以上あれば、すべて）。ほかの人や、ほかのストーリーの記録は残す。`stillAgreed` は、取り消した後も合意済か（ほかの記録が残っているとき）
- 記録の `[[agreement]]` の表（見出しから、終わりの空行とコメントの行を除いた所まで）と、その直前の空行 1 つを消す。ほかの行は変えない。フラグは戻さない
- 消したファイルだけを 1 つのコミットにする。メッセージは `(chore) <ストーリーの ID> で <種類> <ID か場所> の合意を取り消す` で、末尾に `Story: <ストーリーの ID>` のトレーラーを書く。消すファイルにコミットしていない変更があれば `409` で断る
- 書いた後に読み直し、消したもののほかの記録が変わっていれば、ファイルを元に戻して `500` にする

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
      "stage": "plan", "type": "model-check", "layers": ["model"], "detail": null,
      "found": [{"adapter": "tla", "file": "spec/Task.tla", "line": 12, "name": "IdsUnique", "text": "IdsUnique ==\n  …"}],
      "present": true
    }
  ],
  "origin": [{"commit": "0123abcd…", "date": "2026-10-31T12:00:00+09:00", "subject": "(feat) …", "stories": ["#24"], "change": "added"}],
  "agreements": [{"user": {"name": "…", "email": "…"}, "agreedAt": "…", "story": "#24", "fingerprint": "sha256:…", "current": true}]
}
```

| 項目 | 中身 |
|---|---|
| `evidence[].found[].state` | ファイルが合意の対象（シナリオ・オラクルの定義・ガードレールの定義）のときだけ、今の場所と中身の指紋に合う合意の記録があれば `agreed`、なければ `proposed`（ストーリーによらない）。合意の対象でないもの（Go のテストなど）は `null` |
| `evidence[].found` | その約束を指す、タイプに合うアノテーション（「エビデンスが揃っている審査」の表と同じ組み合わせ）。`text` は、`txtar` と `sql` ならファイルの中身全体、`tla` なら付いている定義（定義の行から、次の最上位の定義か `====` の前まで。終わりの空行とコメントだけの行は含めない）、`go` と `ts` なら `null`。`line` は、アノテーションが付いているものの行（`go` は `func` の宣言、`ts` は `test`・`it`・`describe` の呼び出し、`tla` は定義、`sql` は文の行。`txtar` はファイル全体に付くので 1） |
| `subjects` | 約束の `subjects` の順。約束リストにない ID は、`kind`・`name` を `null` にする |
| `evidence[].present` | 揃っているか（RV-08。エビデンスが揃っている審査と同じ判定）。`found` があれば `true`。`proof` は `false`。`human-review` は `null`（アノテーションで確かめない） |
| `origin` | HEAD の履歴の、合流でないコミットのうち、約束の指紋が変わったもの（作ったものを含む）。コミットの日時の新しい順（同じ日時なら、履歴で後のコミットを先に）。`change` は `added`・`modified`。`stories` は `Story:` のトレーラーにあるストーリーの ID。消した後に同じ ID で作り直した約束は新しいものとみなし、消すまでのコミットを入れない |
| `agreements` | 合意の記録のうち、`kind = "claim"` でこの ID を指すもの（消したことへの合意は除く）。合意した日時（`agreed_at`）の新しい順（同じ日時なら、合意の記録のファイルの順（設定の `agreements` のファイルの順、同じファイルなら行の順）で後のものを先に）。`current` は、今の中身の指紋に合うか。`commit` は、その合意の記録を書き足したコミット（HEAD の履歴を古い順にたどり、合意の記録のファイルを変えたコミットの時点の記録に、同じ記録（`kind`・`key`・`fingerprint`・`story`・`agreed_at`・`user`）が初めて現れたコミット）。まだコミットしていなければ `null` |

- アノテーションに読み込みの誤りがあれば、`500` を返し、誤りの数と最初の 1 件を出す

### `GET /api/claims/<約束の ID>/at/<コミット>`

あるコミットの時点の約束の中身（CL-08）。由来の `commit` か、合意の履歴の `commit` を渡す。

```json
{"id": "task-id-unique", "commit": "0123abcd…", "date": "2026-10-31T12:00:00+09:00", "subject": "(feat) …", "stories": ["#24"], "content": {"statement": "…", "rationale": null, "condition": null, "subjects": ["task-id"], "layers": ["model"], "evidence": [], "flags": [], "guesses": []}}
```

- `content` は、その時点の約束リストの項目（形は `/api/review` の `before`・`after` の `claim` と同じ）
- `layers` は、`content.layers` の順に、今の設定のレイヤーの名前を付けた `[{"id": "model", "name": "モデル"}]`。今の設定にないレイヤーは、`name` を空の文字列にする（画面は ID のまま出す）
- コミットはハッシュ（16 進の 4〜64 文字）で渡す。ほかの形は `400`、ないコミットは `404`
- その時点に約束がなければ `404`、その時点の約束リストが読めなければ `500`。どちらも `message` に理由を出す

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
| `[story]` の `adapter` | 文字列 | 省いてよい。なければ、画面にストーリーの題・本文を出さない | ストーリーの置き場所のアダプタの名前。組込みの `github`（GitHub Issues）か、`.vanda/adapters/` のアダプタ |
| `[story]` の `args` | 文字列の配列 | 省いてよい。`.vanda/adapters/` のアダプタのときだけ書ける | アダプタに渡す引数 |
| `[story]` の `repository` | 文字列 | 組込みの `github` を使うときは必ず。ほかのときは書けない | `<owner>/<repo>` |
| `[story]` の `api_url` | 文字列 | 省いてよい。組込みの `github` を使うときだけ書ける | GitHub の REST API の URL。省けば `https://api.github.com`（GitHub Enterprise なら、その API の URL） |
| `[gui]` の `base` | 文字列 | 省いてよい | 画面を開いたときのベースブランチ（CM-06） |

### GitHub Issues のアダプタ

- 認証キーは設定に書かない。使うたびに、`api_url` のホストに合うものを使う。ホストが `api.github.com` なら、環境変数 `GH_TOKEN` があればそれを、なければ `gh auth token --hostname github.com` の出力を使う。ほかのホスト（GitHub Enterprise）なら、`GH_ENTERPRISE_TOKEN` があればそれを、なければ `gh auth token --hostname <そのホスト>` の出力を使う。ホストはポートを含む（例: `ghe.example.com:8443`）
- 合う認証キーが見つからない（環境変数がなく、`gh` がない・ログインしていない・出力が空）ときは、GitHub への要求を送らずに `502` にし、`gh` が返した理由を出す。`vanda doctor` も同じ決まりで認証キーを選ぶ
- `api_url` が `http://` で始まるときは、認証キーを送るたびに、暗号化せずに送っていることを標準エラーに出す
- `GET <api_url>/repos/<repository>/issues/<番号>` を呼ぶ。番号は、ストーリーの ID の数字の部分
- 添付ファイルは、本文の中のリンクと画像（Markdown の `[名前](URL)`・`![名前](URL)` と、HTML の `<img src>`）のうち、URL が `https://github.com/user-attachments/`・`https://github.com/<repository>/assets/`・`https://user-images.githubusercontent.com/` で始まるもの。名前は、リンクの文字か画像の代わりの文字（なければ URL の最後の部分）。並びは本文に出てくる順で、同じ URL は 1 度だけ
- 添付ファイルの見分け方は、`github.com` の URL だけ。GitHub Enterprise の添付は、まだ見分けない

### `.vanda/adapters/` のアダプタ（ストーリーの置き場所）

アノテーションの `.vanda/adapters/` のアダプタと同じく、リポジトリの根を作業のディレクトリにして動かし、標準入力に JSON を渡し、標準出力から JSON を受け取る。`action` は、段 2 では `read` だけ（コメントを書く `write` は段 4 で足す）。

```json
{"action": "read", "story": "#24"}
```

```json
{"title": "…", "body": "…", "url": "…", "attachments": [{"name": "…", "url": "…"}]}
```

- ストーリーがない・認証が通らないなどで返せないときは、0 でない終了コードで終わり、標準エラーに理由を書く。vanda は、それを画面にそのまま出す
- 決まっていない項目は誤りにする。`title`・`body`・`url` は必ず（`url` が分からなければ空の文字列）。`attachments` は省いてよい

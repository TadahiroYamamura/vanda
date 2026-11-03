# vanda 段 4 の 2 の計画: 参照（`{@id:…}`）

story #4 の段 4 の 2。約束 reference-form・reference-literal・reference-broken・reference-fingerprint-raw・reference-shown・reference-popup・reference-diff・reference-filter（用語 reference、機能 show-references）を作る。約束は直さない。約束の文と食い違う・約束にない判断が要るときは、作る前に監督に聞く。

前提として読むもの: `CLAUDE.md`、`docs/formats.md`（約束リスト・GUI の API）、`docs/pitfalls.jsonl`（`trigger` が当たるものの `question` を確かめる）、上の約束の文。

## 書き方（約束のまとめ）

- 約束リストの、名前を除く自然言語で書く項目の中の `{@id:<ID>}` が参照。`{@@` は文字の `{@`（参照ではない）
- 自然言語で書く項目: 用語の `definition`、機能の `definition`・`formal_waiver`、約束の `statement`・`rationale`・`condition`・`guesses`（の各要素）・`model_check_waiver`、エビデンスの `detail`。`id`・`name`・`subjects` などは対象にしない
- 左から読み、`{@@` を文字の `{@` として読み飛ばす。それ以外の `{@` は参照の始まりで、次の `}` までを `<種類>:<値>` として読む
- 次のどれかなら promise-list-shape の誤り: `}` がない・`:` がない（形でない）、種類が `id` でない、値の ID の用語・機能・約束が約束リストにない。誤りは、その項目の行を指す
- 指紋は今のまま、書いたままの表から作る（変えない）

## API の形（監督が決めた。`docs/formats.md` の「GUI の API」に書く）

自然言語で書く項目は、文字列ではなく、片（かけら）の配列で返す。

```json
"statement": [{"text": "タスクを足すと、"}, {"text": "タスクの ID", "ref": "task-id"}, {"text": " を付ける"}]
```

- 片は `{"text": …}` か、参照なら `{"text": <見せる文字>, "ref": <指す ID>}`。見せる文字は、用語と機能なら名前、約束なら ID
- 壊れた参照は、書いたままの文字を参照でない片にする。`{@@` は `{@` にした文字の片にする
- 対象: `entries[].before`・`after`（約束・用語・機能）、`GET /api/claims/<ID>`、`GET /api/claims/<ID>/at/<コミット>` の、上の「自然言語で書く項目」。`guesses` は片の配列の配列。書いていない項目は今までどおり `null`
- それぞれの応答の最上位に `refs` を足す。応答の中の片が指すすべての ID について、`{"kind": "term" | "feature" | "claim", "name": <用語と機能は名前、約束は null>, "text": <用語と機能は definition、約束は statement。中の参照は見せる文字に置き換えた、ただの文字列>, "current": <今の約束リストにあれば true>}`
- 名前は今の約束リストのものを使う。今の約束リストにない ID だけ、比べる元（`GET /api/review` ならその分かれた点・範囲の始まり、`.../at/<コミット>` ならそのコミット）の約束リストのものを使う（reference-diff と同じ決まり）
- `entries[].diff.fields` と、エビデンスの `detail` の差分は、両側を片の見せる文字に置き換えた文どうしで比べる。差分の 1 つ（`{"op", "text"}`）が参照にかかる所は、参照の境で分け、参照にかかる分に `"ref": <ID>` を付ける（`delete` は変更前の側、`equal`・`insert` は変更後の側の参照の位置で決める）

## 画面

- 片を見せる部品を 1 つ作り、自然言語で書く項目を出すすべての所（一覧の詳細、変更前と変更後、差分、約束の詳細、約束の履歴）で使う
- 参照は、参照だと分かる見た目（下線か色）で出す。約束を指し `current` が true の参照は、約束の詳細へのリンクにする
- 参照に触れたとき・フォーカスしたときに、`refs` の `text` を出す（用語と機能は定義、約束は文）。キーボードでフォーカスできるようにする
- 一覧の絞り込みは、片の `text` をつないだ文に当てる（ID・名前・パスに当てるのは今のまま）

## 単位と担当

| # | 中身 | 担当 |
|---|---|---|
| R1 | 参照を読む部品（`internal/promises`）、promise-list-shape の誤り、`docs/formats.md` の約束リストの書き方 | 開発者 |
| R2 | API の片・`refs`・差分（`internal/review`）、`docs/formats.md` の GUI の API | 開発者 |
| R3 | 画面（`web/src`） | 開発者 |
| T | 約束ごとのエビデンス（下の表） | テスト担当 |

R1 → R2 → R3 の順にコミットを分ける。テスト担当は、この計画と約束から期待値を作り、開発者と並べて進める（実装を読んで期待値を作らない）。

| 約束 | エビデンスの予定 |
|---|---|
| reference-form | unit-test（cli）: 自然言語で書く項目それぞれで参照として読み、`name` の中の `{@id:…}` は読まない |
| reference-literal | unit-test（cli）: `{@@`・`{@@@`・`{@@id:x}` などが参照にならず、誤りにもならない |
| reference-broken | unit-test（cli）: 3 つの壊れ方それぞれが promise-list-shape の誤りになり、項目の行を指す |
| reference-fingerprint-raw | unit-test（cli・api）: 参照先の名前・定義を変えても、参照元の指紋が変わらず、合意が外れない |
| reference-shown | unit-test（api）: 片と `refs`。unit-test（gui）: 見た目とリンク |
| reference-popup | unit-test（gui）: 触れた時・フォーカスした時に定義・文が出る。human-review は PO |
| reference-diff | unit-test（api）: 参照先の名前を変えただけの所が差にならない。今はない用語・機能は比べる元の名前 |
| reference-filter | unit-test（gui）: 名前で絞り込める・書いたままの `{@id:…}` では当たらない |

テストのファイルにアノテーションを付ける（`@vanda:evidence` の書き方は `docs/formats.md`）。

## やらないこと

- 約束リストを参照に書き換えること（段 4 の 5）
- 文字のまま書いた用語の名前を見つける審査（作らないと決まっている）
- MCP のサーバーの出力を変えること

## 確かめること（報告の前）

- `nix develop -c make check`
- `./vanda check .vanda/checks/ci.toml --base main` の promise-list-shape（今の約束リストの `{@@` が誤りにならない）と annotation-links

## 禁止

- `pkill`・`killall`、`~/work/vanda` と `~/work/vanda-demo` での `vanda init`
- `docs/pitfalls.jsonl` の編集（候補は報告に書く）
- プッシュ、`git rebase`・`filter-branch`・`commit --amend`・`reset --hard`
- 127.0.0.1:45569 に触れること。サーバーを起こすときは別のポートにし、終わったら止める
- 約束リストと合意の記録（`.vanda/agreements.toml`）の編集
- 開発者がテストのファイル（`*_test.go`、`cmd/vanda/testdata/`、`spec/`、`web/src/__tests__/`、`web/e2e/`）を書き換えること。テスト担当が実装のファイルを書き換えること

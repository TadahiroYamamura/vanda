# エビデンスのタイプの対応・組込みの TypeScript のアダプタ・Playwright のアダプタの計画

ストーリー #4（vanda を vanda で開発する形に切り替える）の中で行う。Issue #2 と、Issue #1 のうち TypeScript の分を含む。順に作る。

## 1. `[[annotation]]` の `types`（Issue #2）

- `[[annotation]]` に `types`（文字列の配列、必ず書く）を足す。そのアノテーションが満たすエビデンスのタイプを書く
- エビデンスが揃っている審査（`evidence-present`）は、決め打ちの表をやめ、予定のタイプを `types` に含む `[[annotation]]` から読んだアノテーションで揃っているかを見る
- アノテーションの種類とタイプの組み合わせ:

| タイプ | 揃っているとするアノテーションの種類 |
|---|---|
| `model-check` | `@vanda:claim` |
| `sensor` | `@vanda:sensor` |
| `human-review`・`monitoring` | 確かめない（`types` に書けない） |
| `proof` | 今は扱えない（`types` に書けない） |
| そのほか（`scenario-test`・`unit-test`・`guardrail` など） | `@vanda:evidence` |

- 組込みのアダプタは、既定のタイプを持つ（`go`: `unit-test`・`cross-check`・`random-check`・`interface-test`・`concurrency-test`・`sensor`、`txtar`: `scenario-test`、`tla`: `model-check`、`sql`: `guardrail`、`ts`: `unit-test`）。`vanda init` は、選んだ言語のアダプタの既定のタイプをそのまま書く（問いかけない）
- 組込みのアダプタに、書けない種類でしか満たせないタイプを `types` に書いたとき（例: `tla` に `unit-test`）は、設定の誤りにする。`.vanda/adapters/` のアダプタは、どの種類も書けるので誤りにしない。`types = []` は、どのタイプも満たさないとする
- vanda のリポジトリの `.vanda/config.toml` に `types` を書き足す

## 2. 組込みの TypeScript のアダプタ（`ts`）

| 項目 | 中身 |
|---|---|
| 書ける種類 | `evidence` |
| 数える所 | `//`・`/* */` のコメントの中だけ。文字列・テンプレート文字列の中は数えない |
| 書ける所 | `test`・`it`・`describe` から始まる呼び出し（後ろに付く修飾は名前を決めずにすべて受け付ける。`.skip`・`.only`・`.todo`・`.each(…)(…)`、Playwright の `test.describe`・`test.describe.serial` など）の直前のコメント（空行をはさまない）。`beforeEach`・`afterEach`・`beforeAll`・`afterAll`・`test.step` と、ほかの所のコメントは誤り |
| 付いているものの名前 | 最後の呼び出しの最初の引数（`.each(表)("名前", …)` なら `"名前"`）。文字列か、テンプレート文字列なら書いてあるとおりの文字（`${op} を…` はそのまま）。`describe`（`test.describe` を含む）の中なら、外側から順に ` > ` でつなぐ（例: `ReviewPage > 一覧を出す`）。名前が変数などで決まった文字でなければ誤り |
| 見つけ方と既定のパターン | `.test.ts`・`.test.tsx` で終わるファイル。既定のパターンは `**/*.test.ts` と `**/*.test.tsx`。`vanda init` のパターンの問いは、どの言語でも空白で区切って複数書ける形にする |

## 3. `.vanda/adapters/playwright`（vanda のリポジトリだけで使う）

- vanda のリポジトリの `.vanda/adapters/playwright` に置く、`.vanda/adapters/` のアダプタ。E2E（`web/e2e/**/*.spec.ts`）を読み、`types = ["scenario-test"]` で設定に書く
- 読み方は組込みの `ts` と同じ（`test(`・`describe(` の直前のコメント、名前は ` > ` でつないだもの）。`.vanda/adapters/` のアダプタの受け渡しの見本として、組込みのものとは別に作る
- 書く言語: Node（`#!/usr/bin/env node`。flake に Node がある）。依存は足さない

## 依存

足さない。

## 分担

| 部分 | 開発者 | テスト担当 |
|---|---|---|
| 1 | 設定・審査・`init`・`docs/formats.md` の表 | 種類×タイプの全部の組み合わせ、`types` がないときの誤り、`init` が既定のタイプを書くこと |
| 2 | アダプタ | 書ける所・書けない所、名前の取り方（入れ子の `describe`、`.each`、文字列でない名前）、文字列の中の `@vanda:` |
| 3 | アダプタと設定 | E2E にアノテーションを付けたときに、シナリオテストとして揃うこと |

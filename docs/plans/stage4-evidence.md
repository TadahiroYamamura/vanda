# vanda 段 4 の 4 の計画: 足りないエビデンスを足す

story #4 の段 4 の 4。審査 evidence-present が失敗にしている 72 の約束の 84 件のエビデンスを、テスト担当が足す。約束は直さない。

前提として読むもの: `CLAUDE.md`、`docs/formats.md`（アノテーションの書き方）、`docs/pitfalls.jsonl`（`trigger` が当たるものの `question` を確かめる）、`.vanda/config.toml` の `[[annotation]]`、下の表の約束の `statement`・`condition`・`[[claim.evidence]]`（`type`・`layers`・`detail`）。

## 進め方

- 期待値は約束の文と `docs/formats.md` から作る。実装を読んで期待値を作らない（どこを呼べばよいかを知るために読むのはよい）
- 同じことを確かめている既存のテストがあれば、まずその拡張で済まないか見る。既存のテストにアノテーションを付けるだけで済ませるときは、そのテストが約束の文のすべての部分を確かめていることを確かめ、足りない所を足す
- エビデンスの `layers` に合う所に書く。api は `cmd/vanda` の API のテスト、gui は `web/src/__tests__/`、cli は CLI のテストか台本
- タイプとファイル: unit-test・interface-test・concurrency-test は `*_test.go`（gui の unit-test は `web/src/**/*.test.tsx`）、scenario-test は `cmd/vanda/testdata/` の `.txtar`・`web/e2e/` の `.spec.ts`・`cmd/vanda/testdata/expect/` の `.exp`
- concurrency-test は、同時に要求を出し、`go test -race` で動かして確かめる
- 判定のテストは、実装の判定をわざと壊して落ちることを確かめる（壊した実装はコミットしない。確かめた後に戻す）
- 約束どおりに動かず落ちるテストは、コミットしない。その約束の名前、テストの中身、どう食い違ったかを報告に書き、ほかの約束に進む
- 約束の文が曖昧で期待値が決められないときは、その約束を飛ばして報告に書く
- 約束ごとにコミットを分けなくてよい。近い約束をまとめてコミットする

## 分け方

2 人のテスト担当が並べて進める。ワークツリーとブランチは別の名前にする（A は `evidence-a`、B は `evidence-b`）。同じ名前のブランチを使わない。

### A: CLI・設定・約束リスト・合意の記録（43 の約束）

| 約束 | 足りないタイプ |
|---|---|
| agree-commit-message | unit-test |
| agree-inline-flags-refused | unit-test |
| agree-record-file | unit-test |
| agree-record-form | unit-test |
| agree-record-user-from-git | unit-test |
| agreements-default-file-may-be-absent | unit-test |
| agreements-fingerprint-shape | unit-test |
| agreements-required-fields | unit-test |
| branch-point-merge-base | unit-test・unit-test |
| check-base-unused-without-story | unit-test・scenario-test |
| check-config-unknown-entries-rejected | unit-test |
| check-errors-before-run | unit-test・scenario-test |
| check-guardrail-file-missing | unit-test |
| check-load-errors-owner | unit-test |
| check-name-known | unit-test |
| check-output-story-id | unit-test・scenario-test |
| check-scope-table | unit-test |
| config-annotation-types-unique | unit-test |
| config-api-url-default | unit-test |
| config-api-url-scheme | unit-test |
| config-github-repository | unit-test |
| config-guardrail-adapter-file | unit-test |
| config-id-pattern-default | unit-test |
| config-id-pattern-regex | unit-test |
| config-layer-id | unit-test |
| config-pattern-syntax | unit-test |
| config-patterns-not-empty | unit-test |
| config-required-fields | unit-test |
| fingerprint-form | unit-test |
| fingerprint-item-json | unit-test |
| fingerprint-json-strings | unit-test |
| init-check-failure-keeps | unit-test |
| init-no-store-id-pattern | unit-test・interface-test |
| init-write-failure-cleans | unit-test |
| origin-commit-stories | unit-test |
| project-adapter-annotation-true-rejected | unit-test |
| project-adapter-missing-answer | unit-test・interface-test |
| promise-list-field-types | unit-test |
| promise-list-fixed-value-list | unit-test |
| promise-list-non-empty | unit-test |
| promise-list-required-field-names | unit-test |
| revoke-commit-message | unit-test |
| revoke-removes-all-duplicates | unit-test |

### B: vanda サーバーの API と画面（29 の約束）

| 約束 | 足りないタイプ |
|---|---|
| agree-button-label | unit-test |
| agree-cannot-reason-shown | unit-test |
| api-error-fix-shown | unit-test |
| claim-detail-found-location | unit-test |
| claim-detail-guesses | unit-test |
| claim-detail-origin | scenario-test |
| claim-detail-state | unit-test・unit-test |
| claim-detail-subjects | unit-test・unit-test |
| context-error-shown | unit-test |
| file-page-error-shown | unit-test |
| header-context | unit-test |
| review-added-content | unit-test・unit-test |
| review-change-counts | unit-test |
| review-detached-on-top | unit-test |
| review-detail-links | unit-test |
| review-empty-note | unit-test |
| review-group-first-layer | unit-test |
| review-groups-fold-over-ten | unit-test |
| review-order | unit-test・unit-test |
| revoke-others-shown | unit-test |
| revoke-reason-shown | unit-test |
| serve-agree-one-at-a-time | unit-test・concurrency-test |
| serve-browser-open-failure | unit-test |
| serve-config-error-stops | unit-test |
| serve-port-unavailable | unit-test・interface-test |
| serve-reloads-config | unit-test |
| story-github-issue-number | unit-test |
| story-open-link | unit-test |
| story-undecided-note | unit-test |

同じ約束にタイプが 2 つ（例: unit-test・unit-test）あるのは、`layers` の違うエビデンスが 2 つあるもの。

## 確かめること（報告の前）

- `nix develop -c make check`
- `./vanda check .vanda/checks/ci.toml --base main` の evidence-present に、自分の担当の約束が残っていないこと（残したものは、理由を報告に書く）と、annotation-links が通ること

## 禁止

- `pkill`・`killall`、`~/work/vanda` と `~/work/vanda-demo` での `vanda init`
- `docs/pitfalls.jsonl` の編集（候補は報告に書く）
- プッシュ、`git rebase`・`filter-branch`・`commit --amend`・`reset --hard`
- 127.0.0.1:45569 に触れること。サーバーを起こすときは別のポートにし、終わったら止める
- 約束リストと合意の記録（`.vanda/agreements.toml`）の編集
- 実装のファイル（テストのファイル以外）を書き換えること

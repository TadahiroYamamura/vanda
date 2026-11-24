# 約束リストの作り直し: 今の仕様から新しい仕様への対応の表

今の仕様 298 件のそれぞれが、新しい仕様 24 件（`promise-reorg-properties.md` の「仕様を、機能から書き直した結果」）のどれの細部に当たるかを並べた。アノテーションの付け直しは、この表に従う（`promise-reorg-shape-impl.md` の 2.8）。

新しい仕様の番号は、この表の中だけのもの。ID は、約束リストに書く時に決める。

## 1. 新しい仕様ごとの数

| # | 性質 | 機能 | 仕様（短く） | 当たる今の仕様 | 今のアノテーション |
|---|---|---|---|---|---|
| 1 | A-1 | 見た中身に、承認を結びつける | 承認は、見た時点の中身に結びつく | 9 | 20 |
| 2 | A-1 | 見た中身に、承認を結びつける | 画面の中身と判定は、vanda check と一致する | 2 | 4 |
| 3 | A-1 | 見た中身に、承認を結びつける | 承認と取り消しは、ファイルを壊さず、ほかの変更を巻き込まない | 6 | 19 |
| 4 | A-1 | 中身が変わった承認を、失効させる | 中身が変われば、承認済みでなくなる | 16 | 39 |
| 5 | A-1 | 承認を取り消す | 自分の承認だけを取り消せる | 8 | 23 |
| 6 | A-2 | 承認されていない変更を、誤りにする | 承認のチェックが通れば、すべて承認されている | 8 | 25 |
| 7 | A-3 | アノテーションで結びつける | アノテーションで、支持する仕様を示せる | 15 | 45 |
| 8 | A-3 | アノテーションで結びつける | カスタムアダプタで、ほかの言語を読ませられる | 13 | 35 |
| 9 | A-3 | エビデンスのない仕様を、誤りにする | エビデンスのチェックが通れば、予定は揃っている | 8 | 16 |
| 10 | A-4 | 変更を、ストーリーに結びつける | 約束リストの変更は、ストーリーに結びつく | 8 | 13 |
| 11 | A-4 | 変更と承認の履歴を示す | 誰が・いつ・どのストーリーで・どの中身に、を辿れる | 16 | 33 |
| 12 | B-2 | チェックの誤りを知らせる | 誤りを見逃さず、場所と直し方を添えて知らせる | 56 | 91 |
| 13 | B-2 | チェックの誤りを知らせる | CI から動かし、終了コードと出力で結果を知る | 13 | 24 |
| 14 | C | この PC からだけ使える | 画面からしか操作できない | 6 | 7 |
| 15 | C | 秘密の情報を書かない | トークンは、使うホストにだけ送る | 3 | 5 |
| 16 | D-1 | 変わったものを、漏れなく並べる | 漏れも重複もなく閲覧できる | 29 | 57 |
| 17 | D-1 | 変更前と変更後を比べる | 何がどう変わったかを理解できる | 13 | 36 |
| 18 | D-1 | 項目の中身を、1 か所で示す | 中身とエビデンスを 1 か所で確かめられる | 20 | 49 |
| 19 | D-1 | ストーリーを見せる | ストーリーを同じ画面で読める | 10 | 12 |
| 20 | D-1 | ストーリーを見せる | カスタムアダプタで、ほかのストーリーストアをつなげられる | 1 | 1 |
| 21 | D-2 | コア機能に印を付け、強いエビデンスを求める | コア機能の仕様に、モデル検査か理由を求める | 2 | 4 |
| 22 | D-4 | フラグを立てる | 提案と推測を、開く前に見分けられる | 4 | 6 |
| 23 | D-4 | フラグを立てる | フラグが残っていれば、チェックは通らない | 2 | 9 |
| 24 | E-1 | .vanda/ を作る | 質問に答えるだけで .vanda/ を作れる | 21 | 39 |
| X | - | 消す | doctor・version と一緒に消す | 5 | 14 |
| ? | - | 当てない | アノテーションを外し、ただのテストとして残す | 4 | 5 |

- 「今のアノテーション」は、今の仕様を指す `@vanda:evidence`・`@vanda:sensor`・`@vanda:claim` の数（合わせて 631）。テストのデータの中の、テスト用の ID を指すもの（296）は数えていない
- X は、vanda doctor と vanda version の仕様。コマンドと一緒に、テストも消す
- ? は、どの仕様にも当てないもの。サーバーの起動の使い勝手（serve-prints-url、serve-port-unavailable、serve-browser-open-failure）と、ヘッダーの表示（header-context）。約束リストに残す価値がないので、アノテーション（5 か所）を外し、ただのテストとして残す
- 当て先は、仕様の文の先頭 100 字ほどを読んで決めた。2 つ以上の仕様にまたがるものは、主な 1 つに当てた
- 12 番（誤りを知らせる）に 56 件が集まる。約束リスト・設定・承認の記録・チェックの設定の、形の決まりのすべてが入るため

## 2. 今の仕様ごとの当て先

| 今の仕様 | # |
|---|---|
| promise-list-reads-all-patterns | 12 |
| reads-committed-files-only | 2 |
| promise-list-default-file-may-be-absent | 12 |
| promise-list-explicit-file-must-exist | 12 |
| promise-list-unknown-entries-rejected | 12 |
| promise-list-required-fields | 12 |
| promise-list-required-field-names | 12 |
| promise-list-non-empty | 12 |
| promise-list-field-types | 12 |
| promise-list-id-shape | 12 |
| reference-form | 12 |
| reference-literal | 12 |
| reference-broken | 12 |
| reference-fingerprint-raw | 4 |
| reference-shown | 18 |
| reference-popup | 18 |
| reference-diff | 17 |
| reference-filter | 16 |
| promise-list-id-unique | 12 |
| promise-list-part-of | 12 |
| promise-list-subjects-exist | 12 |
| promise-list-layers-known | 12 |
| promise-list-evidence-layers-within-claim | 12 |
| promise-list-every-layer-has-evidence | 9 |
| promise-list-fixed-values | 12 |
| promise-list-fixed-value-list | 12 |
| promise-list-formal-waiver | 21 |
| promise-list-formal-claims | 21 |
| fingerprint-excludes-flags | 4 |
| fingerprint-ignores-formatting | 4 |
| fingerprint-as-written | 4 |
| fingerprint-file-content | 4 |
| fingerprint-form | 4 |
| fingerprint-item-json | 4 |
| fingerprint-json-strings | 4 |
| agreed-when-fingerprint-matches | 4 |
| pass-only-when-all-seen | 6 |
| story-pass-only-when-seen | 6 |
| renamed-file-needs-agreement | 4 |
| old-agreements-kept-as-history | 11 |
| detached-agreement | 4 |
| detached-until-story-reagrees | 4 |
| revoke-returns-to-proposed | 5 |
| changed-id-is-new | 4 |
| agreements-unknown-entries-rejected | 12 |
| agreements-default-file-may-be-absent | 12 |
| agreements-explicit-file-may-be-absent | 12 |
| agreements-required-fields | 12 |
| agreements-fingerprint-shape | 12 |
| agreements-file-record-path | 12 |
| agreements-point-to-existing-items | 12 |
| deleted-item-needs-agreement | 6 |
| deletion-agreement-record | 6 |
| review-deleted-shown | 16 |
| annotation-syntax-errors | 7 |
| annotation-outside-comment-ignored | 7 |
| annotation-backquoted-ignored | 7 |
| annotation-one-rule-per-file | 7 |
| annotation-adapter-required | 7 |
| annotation-kinds-per-builtin-adapter | 7 |
| annotation-go-func-doc | 7 |
| annotation-txtar-comment-lines | 7 |
| annotation-tla-definition | 7 |
| annotation-sql-statement | 7 |
| annotation-ts-test-call | 7 |
| annotation-ts-fixed-name | 7 |
| annotation-ts-hooks-rejected | 7 |
| annotation-ts-skipped-not-counted | 9 |
| annotation-ts-only-rejected | 9 |
| annotation-links-kinds | 7 |
| annotation-links-tla-types | 7 |
| annotation-types-required | 12 |
| annotation-types-unsatisfiable-allowed | 12 |
| annotation-types-empty | 9 |
| project-adapter-name | 8 |
| project-adapter-overrides-builtin | 8 |
| project-adapter-args-and-cwd | 8 |
| project-adapter-reads-committed-content | 8 |
| project-adapter-args-only-for-project | 8 |
| project-adapter-not-runnable | 8 |
| adapter-name-unknown | 12 |
| project-adapter-not-annotation | 8 |
| project-adapter-response-errors | 8 |
| project-adapter-missing-answer | 8 |
| project-adapter-annotation-true-rejected | 8 |
| project-adapter-failure-reason | 8 |
| project-adapter-annotation-timeout | 8 |
| config-annotation-timeout | 8 |
| unread-files-excludes-adapters | 12 |
| check-config-required | 13 |
| check-base-required-for-story | 13 |
| check-base-unused-without-story | 13 |
| check-runs-in-order-and-fails | 13 |
| check-errors-before-run | 13 |
| check-scope-per-check | 13 |
| check-scope-table | 13 |
| check-name-known | 13 |
| check-config-unknown-entries-rejected | 13 |
| check-output-lines | 13 |
| check-output-story-id | 13 |
| check-error-order | 13 |
| check-error-form | 12 |
| check-story-from-commits | 10 |
| check-story-arg-pattern | 13 |
| check-blocked-by-unreadable-input | 12 |
| check-load-errors-owner | 12 |
| check-story-origin | 10 |
| check-agreement-targets | 6 |
| check-guardrail-file-missing | 6 |
| check-agreement-flags-left | 23 |
| check-agreement-story-scope | 6 |
| check-evidence-present | 9 |
| check-evidence-guardrail-needs-definition | 9 |
| check-evidence-sensor-needs-check | 9 |
| check-evidence-proof-and-unchecked | 9 |
| check-unread-files | 12 |
| config-version-mismatch | 12 |
| config-unknown-entries-rejected | 12 |
| config-required-fields | 12 |
| config-layer-id | 12 |
| config-patterns-not-empty | 12 |
| config-pattern-syntax | 12 |
| config-annotation-types-unique | 12 |
| config-guardrail-adapter-file | 12 |
| config-id-pattern-not-empty-match | 12 |
| config-id-pattern-default | 10 |
| config-id-pattern-regex | 12 |
| config-github-only-fields | 12 |
| config-github-repository | 12 |
| config-api-url-scheme | 12 |
| config-api-url-default | 19 |
| outside-git-repository | 12 |
| doctor-ok-line | X |
| doctor-github-issues | X |
| doctor-github-token | X |
| doctor-project-adapter | X |
| version-always-runs | X |
| serve-listens-localhost | 14 |
| serve-prints-url | ? |
| serve-config-error-stops | 12 |
| serve-port-unavailable | ? |
| serve-browser-open-failure | ? |
| serve-rejects-foreign-host | 14 |
| serve-rejects-foreign-origin | 14 |
| api-error-shape | 12 |
| serve-reloads-config | 12 |
| serve-agree-one-at-a-time | 3 |
| context-default-base | 16 |
| context-bases | 16 |
| context-base-always-shown | 16 |
| header-context | ? |
| context-no-user-cannot-agree | 11 |
| context-story-store | 19 |
| story-github-read | 19 |
| story-github-issue-number | 19 |
| story-github-attachments | 19 |
| story-github-token | 15 |
| story-github-token-missing | 15 |
| story-github-http-warning | 15 |
| story-errors | 19 |
| story-project-adapter | 20 |
| review-no-omission-no-duplicate | 16 |
| review-renamed-file | 16 |
| review-count-verified | 16 |
| review-numbering | 16 |
| review-order | 16 |
| review-group-first-layer | 16 |
| review-change-counts | 16 |
| review-empty-note | 16 |
| review-view-branch | 16 |
| review-view-story | 16 |
| review-other-commits | 16 |
| review-story-choice | 16 |
| review-detached-shown | 4 |
| review-detached-unchanged-shown | 4 |
| review-detached-on-top | 4 |
| review-flags | 22 |
| review-flag-labels | 22 |
| review-before-after | 17 |
| review-added-content | 17 |
| review-file-diff-lines | 17 |
| review-evidence-diff-pairing | 17 |
| review-groups-fold | 16 |
| review-groups-fold-over-ten | 16 |
| review-filter-text | 16 |
| review-filter-not-agreed | 16 |
| review-filter-count | 16 |
| review-filter-kept | 16 |
| agree-writes-and-commits | 3 |
| agree-record-file | 3 |
| agree-record-form | 11 |
| agree-record-user-from-git | 11 |
| agree-commit-message | 11 |
| agree-rejects-stale | 1 |
| agree-removes-flags | 23 |
| agree-inline-flags-refused | 3 |
| agree-rejects-uncommitted | 3 |
| agree-rejects-duplicate | 3 |
| evidence-presence-shown | 2 |
| revoke-own-branch-only | 5 |
| revoke-commits-and-keeps-flags | 5 |
| revoke-removes-all-duplicates | 5 |
| revoke-commit-message | 11 |
| detail-origin-and-history | 11 |
| agree-from-detail-pages | 1 |
| agree-from-detail-pages-notes | 1 |
| agree-from-detail-pages-stay | 1 |
| claim-detail-fields | 18 |
| claim-detail-state | 18 |
| claim-detail-subjects | 18 |
| claim-detail-guesses | 18 |
| claim-detail-evidence-grouping | 18 |
| claim-detail-evidence-content | 18 |
| claim-detail-found-location | 18 |
| claim-detail-origin | 11 |
| origin-commit-stories | 10 |
| claim-detail-agreements | 11 |
| tla-math-symbols | 17 |
| tla-symbol-font | 17 |
| tla-symbol-keeps-width | 17 |
| tla-copy-ascii | 17 |
| tla-no-replace-in-strings-comments | 17 |
| claim-at-commit | 11 |
| claim-at-commit-errors | 11 |
| init-requires-terminal | 24 |
| init-terminal-streams | 24 |
| init-keeps-existing | 24 |
| init-defaults-from-repository | 24 |
| init-store-choices | 24 |
| init-no-store-id-pattern | 24 |
| init-language-choices | 24 |
| init-patterns | 24 |
| init-default-types | 24 |
| init-validates-answers | 24 |
| init-values-only | 24 |
| init-writes-files | 24 |
| init-write-failure-cleans | 24 |
| init-check-failure-keeps | 24 |
| init-check-configs | 24 |
| init-decline-creates-nothing | 24 |
| init-guide | 24 |
| init-missing-project-adapter | 24 |
| init-accepts-builtin-name | 24 |
| init-reads-answers-sent-after-prompt | 24 |
| init-restores-terminal | 24 |
| story-shown-on-screen | 19 |
| story-open-link | 19 |
| story-undecided-note | 19 |
| story-error-shown-on-screen | 19 |
| agree-on-screen | 1 |
| review-detail-links | 18 |
| agree-button-label | 1 |
| agree-cannot-reason-shown | 1 |
| revoke-on-screen | 5 |
| revoke-others-shown | 5 |
| revoke-reason-shown | 5 |
| revoke-refused | 5 |
| file-api | 14 |
| claim-detail-error-shown | 12 |
| api-error-fix-shown | 12 |
| context-error-shown | 12 |
| file-page-error-shown | 12 |
| agree-refusal-shown | 1 |
| review-error-shown | 16 |
| review-state-shown | 16 |
| review-list-icons | 22 |
| review-list-first-line | 16 |
| review-icon-names | 22 |
| agree-on-screen-reloads | 1 |
| story-body-markdown | 14 |
| story-links-new-tab | 14 |
| highlight-fallback | 17 |
| config-file-required | 12 |
| back-restores-state | 16 |
| keep-user-whitespace | 17 |
| diff-whitespace-marked | 17 |
| entry-select-anywhere | 16 |
| evidence-presence-three-states | 18 |
| evidence-detail-no-truncate | 18 |
| claim-at-commit-unknown-layer | 11 |
| claim-at-commit-multiple-open | 11 |
| claim-at-commit-toggle | 11 |
| claim-at-commit-error-shown | 11 |
| found-file-state-shown | 18 |
| story-id-from-trailer | 10 |
| branch-point-merge-base | 6 |
| story-trailer-at-most-one | 10 |
| story-trailer-matches-pattern | 10 |
| story-trailer-errors-shown | 10 |
| agree-commit-has-story-trailer | 11 |
| term-page-content | 18 |
| feature-page-content | 18 |
| term-feature-page-claims | 18 |
| review-term-feature-claims | 18 |
| term-feature-page-missing | 18 |
| claim-detail-shown-before-history | 18 |
| review-detail-shown-before-list | 18 |
| loading-indicator | 16 |

# 約束リストの作り直し: 今の仕様から新しい仕様への対応の表

今の仕様 298 件のそれぞれが、新しい仕様 24 件（`promise-reorg-properties.md` の「仕様を、機能から書き直した結果」）のどれの細部に当たるかを並べた。アノテーションの付け直しは、この表に従う（`promise-reorg-shape-impl.md` の 2.8）。

1 章の番号は、この表の中だけのもの。2 章は、約束リストに書いた新しい仕様の ID で並べた。

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

「新しい仕様」が X のものは、テストも消す。? のものは、アノテーションを外し、テストとして残す。

| 今の仕様 | 新しい仕様 |
|---|---|
| promise-list-reads-all-patterns | mistakes-reported-with-fix |
| reads-committed-files-only | screen-matches-check |
| promise-list-default-file-may-be-absent | mistakes-reported-with-fix |
| promise-list-explicit-file-must-exist | mistakes-reported-with-fix |
| promise-list-unknown-entries-rejected | mistakes-reported-with-fix |
| promise-list-required-fields | mistakes-reported-with-fix |
| promise-list-required-field-names | mistakes-reported-with-fix |
| promise-list-non-empty | mistakes-reported-with-fix |
| promise-list-field-types | mistakes-reported-with-fix |
| promise-list-id-shape | mistakes-reported-with-fix |
| reference-form | mistakes-reported-with-fix |
| reference-literal | mistakes-reported-with-fix |
| reference-broken | mistakes-reported-with-fix |
| reference-fingerprint-raw | agreement-detached-when-content-changes |
| reference-shown | item-detail-in-one-place |
| reference-popup | item-detail-in-one-place |
| reference-diff | owner-understands-each-change |
| reference-filter | owner-reviews-every-change |
| promise-list-id-unique | mistakes-reported-with-fix |
| promise-list-part-of | mistakes-reported-with-fix |
| promise-list-subjects-exist | mistakes-reported-with-fix |
| promise-list-layers-known | mistakes-reported-with-fix |
| promise-list-evidence-layers-within-claim | mistakes-reported-with-fix |
| promise-list-every-layer-has-evidence | check-passes-only-with-evidence |
| promise-list-fixed-values | mistakes-reported-with-fix |
| promise-list-fixed-value-list | mistakes-reported-with-fix |
| promise-list-formal-waiver | core-specs-need-model-check |
| promise-list-formal-claims | core-specs-need-model-check |
| fingerprint-excludes-flags | agreement-detached-when-content-changes |
| fingerprint-ignores-formatting | agreement-detached-when-content-changes |
| fingerprint-as-written | agreement-detached-when-content-changes |
| fingerprint-file-content | agreement-detached-when-content-changes |
| fingerprint-form | agreement-detached-when-content-changes |
| fingerprint-item-json | agreement-detached-when-content-changes |
| fingerprint-json-strings | agreement-detached-when-content-changes |
| agreed-when-fingerprint-matches | agreement-detached-when-content-changes |
| pass-only-when-all-seen | check-passes-only-when-agreed |
| story-pass-only-when-seen | check-passes-only-when-agreed |
| renamed-file-needs-agreement | agreement-detached-when-content-changes |
| old-agreements-kept-as-history | agreement-history-kept |
| detached-agreement | agreement-detached-when-content-changes |
| detached-until-story-reagrees | agreement-detached-when-content-changes |
| revoke-returns-to-proposed | revoke-own-agreement-only |
| changed-id-is-new | agreement-detached-when-content-changes |
| agreements-unknown-entries-rejected | mistakes-reported-with-fix |
| agreements-default-file-may-be-absent | mistakes-reported-with-fix |
| agreements-explicit-file-may-be-absent | mistakes-reported-with-fix |
| agreements-required-fields | mistakes-reported-with-fix |
| agreements-fingerprint-shape | mistakes-reported-with-fix |
| agreements-file-record-path | mistakes-reported-with-fix |
| agreements-point-to-existing-items | mistakes-reported-with-fix |
| deleted-item-needs-agreement | check-passes-only-when-agreed |
| deletion-agreement-record | check-passes-only-when-agreed |
| review-deleted-shown | owner-reviews-every-change |
| annotation-syntax-errors | annotations-link-evidence |
| annotation-outside-comment-ignored | annotations-link-evidence |
| annotation-backquoted-ignored | annotations-link-evidence |
| annotation-one-rule-per-file | annotations-link-evidence |
| annotation-adapter-required | annotations-link-evidence |
| annotation-kinds-per-builtin-adapter | annotations-link-evidence |
| annotation-go-func-doc | annotations-link-evidence |
| annotation-txtar-comment-lines | annotations-link-evidence |
| annotation-tla-definition | annotations-link-evidence |
| annotation-sql-statement | annotations-link-evidence |
| annotation-ts-test-call | annotations-link-evidence |
| annotation-ts-fixed-name | annotations-link-evidence |
| annotation-ts-hooks-rejected | annotations-link-evidence |
| annotation-ts-skipped-not-counted | check-passes-only-with-evidence |
| annotation-ts-only-rejected | check-passes-only-with-evidence |
| annotation-links-kinds | annotations-link-evidence |
| annotation-links-tla-types | annotations-link-evidence |
| annotation-types-required | mistakes-reported-with-fix |
| annotation-types-unsatisfiable-allowed | mistakes-reported-with-fix |
| annotation-types-empty | check-passes-only-with-evidence |
| project-adapter-name | annotation-adapter-contract |
| project-adapter-overrides-builtin | annotation-adapter-contract |
| project-adapter-args-and-cwd | annotation-adapter-contract |
| project-adapter-reads-committed-content | annotation-adapter-contract |
| project-adapter-args-only-for-project | annotation-adapter-contract |
| project-adapter-not-runnable | annotation-adapter-contract |
| adapter-name-unknown | mistakes-reported-with-fix |
| project-adapter-not-annotation | annotation-adapter-contract |
| project-adapter-response-errors | annotation-adapter-contract |
| project-adapter-missing-answer | annotation-adapter-contract |
| project-adapter-annotation-true-rejected | annotation-adapter-contract |
| project-adapter-failure-reason | annotation-adapter-contract |
| project-adapter-annotation-timeout | annotation-adapter-contract |
| config-annotation-timeout | annotation-adapter-contract |
| unread-files-excludes-adapters | mistakes-reported-with-fix |
| check-config-required | check-runs-in-ci |
| check-base-required-for-story | check-runs-in-ci |
| check-base-unused-without-story | check-runs-in-ci |
| check-runs-in-order-and-fails | check-runs-in-ci |
| check-errors-before-run | check-runs-in-ci |
| check-scope-per-check | check-runs-in-ci |
| check-scope-table | check-runs-in-ci |
| check-name-known | check-runs-in-ci |
| check-config-unknown-entries-rejected | check-runs-in-ci |
| check-output-lines | check-runs-in-ci |
| check-output-story-id | check-runs-in-ci |
| check-error-order | check-runs-in-ci |
| check-error-form | mistakes-reported-with-fix |
| check-story-from-commits | changes-traceable-to-story |
| check-story-arg-pattern | check-runs-in-ci |
| check-blocked-by-unreadable-input | mistakes-reported-with-fix |
| check-load-errors-owner | mistakes-reported-with-fix |
| check-story-origin | changes-traceable-to-story |
| check-agreement-targets | check-passes-only-when-agreed |
| check-guardrail-file-missing | check-passes-only-when-agreed |
| check-agreement-flags-left | flags-block-check |
| check-agreement-story-scope | check-passes-only-when-agreed |
| check-evidence-present | check-passes-only-with-evidence |
| check-evidence-guardrail-needs-definition | check-passes-only-with-evidence |
| check-evidence-sensor-needs-check | check-passes-only-with-evidence |
| check-evidence-proof-and-unchecked | check-passes-only-with-evidence |
| check-unread-files | mistakes-reported-with-fix |
| config-version-mismatch | mistakes-reported-with-fix |
| config-unknown-entries-rejected | mistakes-reported-with-fix |
| config-required-fields | mistakes-reported-with-fix |
| config-layer-id | mistakes-reported-with-fix |
| config-patterns-not-empty | mistakes-reported-with-fix |
| config-pattern-syntax | mistakes-reported-with-fix |
| config-annotation-types-unique | mistakes-reported-with-fix |
| config-guardrail-adapter-file | mistakes-reported-with-fix |
| config-id-pattern-not-empty-match | mistakes-reported-with-fix |
| config-id-pattern-default | changes-traceable-to-story |
| config-id-pattern-regex | mistakes-reported-with-fix |
| config-github-only-fields | mistakes-reported-with-fix |
| config-github-repository | mistakes-reported-with-fix |
| config-api-url-scheme | mistakes-reported-with-fix |
| config-api-url-default | owner-reads-story-with-review |
| outside-git-repository | mistakes-reported-with-fix |
| doctor-ok-line | X |
| doctor-github-issues | X |
| doctor-github-token | X |
| doctor-project-adapter | X |
| version-always-runs | X |
| serve-listens-localhost | server-operated-only-from-own-screen |
| serve-prints-url | ? |
| serve-config-error-stops | mistakes-reported-with-fix |
| serve-port-unavailable | ? |
| serve-browser-open-failure | ? |
| serve-rejects-foreign-host | server-operated-only-from-own-screen |
| serve-rejects-foreign-origin | server-operated-only-from-own-screen |
| api-error-shape | mistakes-reported-with-fix |
| serve-reloads-config | mistakes-reported-with-fix |
| serve-agree-one-at-a-time | agreement-keeps-files-safe |
| context-default-base | owner-reviews-every-change |
| context-bases | owner-reviews-every-change |
| context-base-always-shown | owner-reviews-every-change |
| header-context | ? |
| context-no-user-cannot-agree | agreement-history-kept |
| context-story-store | owner-reads-story-with-review |
| story-github-read | owner-reads-story-with-review |
| story-github-issue-number | owner-reads-story-with-review |
| story-github-attachments | owner-reads-story-with-review |
| story-github-token | token-sent-only-to-its-host |
| story-github-token-missing | token-sent-only-to-its-host |
| story-github-http-warning | token-sent-only-to-its-host |
| story-errors | owner-reads-story-with-review |
| story-project-adapter | story-adapter-contract |
| review-no-omission-no-duplicate | owner-reviews-every-change |
| review-renamed-file | owner-reviews-every-change |
| review-count-verified | owner-reviews-every-change |
| review-numbering | owner-reviews-every-change |
| review-order | owner-reviews-every-change |
| review-group-first-layer | owner-reviews-every-change |
| review-change-counts | owner-reviews-every-change |
| review-empty-note | owner-reviews-every-change |
| review-view-branch | owner-reviews-every-change |
| review-view-story | owner-reviews-every-change |
| review-other-commits | owner-reviews-every-change |
| review-story-choice | owner-reviews-every-change |
| review-detached-shown | agreement-detached-when-content-changes |
| review-detached-unchanged-shown | agreement-detached-when-content-changes |
| review-detached-on-top | agreement-detached-when-content-changes |
| review-flags | flags-visible-before-open |
| review-flag-labels | flags-visible-before-open |
| review-before-after | owner-understands-each-change |
| review-added-content | owner-understands-each-change |
| review-file-diff-lines | owner-understands-each-change |
| review-evidence-diff-pairing | owner-understands-each-change |
| review-groups-fold | owner-reviews-every-change |
| review-groups-fold-over-ten | owner-reviews-every-change |
| review-filter-text | owner-reviews-every-change |
| review-filter-not-agreed | owner-reviews-every-change |
| review-filter-count | owner-reviews-every-change |
| review-filter-kept | owner-reviews-every-change |
| agree-writes-and-commits | agreement-keeps-files-safe |
| agree-record-file | agreement-keeps-files-safe |
| agree-record-form | agreement-history-kept |
| agree-record-user-from-git | agreement-history-kept |
| agree-commit-message | agreement-history-kept |
| agree-rejects-stale | agreement-bound-to-seen-content |
| agree-removes-flags | flags-block-check |
| agree-inline-flags-refused | agreement-keeps-files-safe |
| agree-rejects-uncommitted | agreement-keeps-files-safe |
| agree-rejects-duplicate | agreement-keeps-files-safe |
| evidence-presence-shown | screen-matches-check |
| revoke-own-branch-only | revoke-own-agreement-only |
| revoke-commits-and-keeps-flags | revoke-own-agreement-only |
| revoke-removes-all-duplicates | revoke-own-agreement-only |
| revoke-commit-message | agreement-history-kept |
| detail-origin-and-history | agreement-history-kept |
| agree-from-detail-pages | agreement-bound-to-seen-content |
| agree-from-detail-pages-notes | agreement-bound-to-seen-content |
| agree-from-detail-pages-stay | agreement-bound-to-seen-content |
| claim-detail-fields | item-detail-in-one-place |
| claim-detail-state | item-detail-in-one-place |
| claim-detail-subjects | item-detail-in-one-place |
| claim-detail-guesses | item-detail-in-one-place |
| claim-detail-evidence-grouping | item-detail-in-one-place |
| claim-detail-evidence-content | item-detail-in-one-place |
| claim-detail-found-location | item-detail-in-one-place |
| claim-detail-origin | agreement-history-kept |
| origin-commit-stories | changes-traceable-to-story |
| claim-detail-agreements | agreement-history-kept |
| tla-math-symbols | owner-understands-each-change |
| tla-symbol-font | owner-understands-each-change |
| tla-symbol-keeps-width | owner-understands-each-change |
| tla-copy-ascii | owner-understands-each-change |
| tla-no-replace-in-strings-comments | owner-understands-each-change |
| claim-at-commit | agreement-history-kept |
| claim-at-commit-errors | agreement-history-kept |
| init-requires-terminal | start-with-init |
| init-terminal-streams | start-with-init |
| init-keeps-existing | start-with-init |
| init-defaults-from-repository | start-with-init |
| init-store-choices | start-with-init |
| init-no-store-id-pattern | start-with-init |
| init-language-choices | start-with-init |
| init-patterns | start-with-init |
| init-default-types | start-with-init |
| init-validates-answers | start-with-init |
| init-values-only | start-with-init |
| init-writes-files | start-with-init |
| init-write-failure-cleans | start-with-init |
| init-check-failure-keeps | start-with-init |
| init-check-configs | start-with-init |
| init-decline-creates-nothing | start-with-init |
| init-guide | start-with-init |
| init-missing-project-adapter | start-with-init |
| init-accepts-builtin-name | start-with-init |
| init-reads-answers-sent-after-prompt | start-with-init |
| init-restores-terminal | start-with-init |
| story-shown-on-screen | owner-reads-story-with-review |
| story-open-link | owner-reads-story-with-review |
| story-undecided-note | owner-reads-story-with-review |
| story-error-shown-on-screen | owner-reads-story-with-review |
| agree-on-screen | agreement-bound-to-seen-content |
| review-detail-links | item-detail-in-one-place |
| agree-button-label | agreement-bound-to-seen-content |
| agree-cannot-reason-shown | agreement-bound-to-seen-content |
| revoke-on-screen | revoke-own-agreement-only |
| revoke-others-shown | revoke-own-agreement-only |
| revoke-reason-shown | revoke-own-agreement-only |
| revoke-refused | revoke-own-agreement-only |
| file-api | server-operated-only-from-own-screen |
| claim-detail-error-shown | mistakes-reported-with-fix |
| api-error-fix-shown | mistakes-reported-with-fix |
| context-error-shown | mistakes-reported-with-fix |
| file-page-error-shown | mistakes-reported-with-fix |
| agree-refusal-shown | agreement-bound-to-seen-content |
| review-error-shown | owner-reviews-every-change |
| review-state-shown | owner-reviews-every-change |
| review-list-icons | flags-visible-before-open |
| review-list-first-line | owner-reviews-every-change |
| review-icon-names | flags-visible-before-open |
| agree-on-screen-reloads | agreement-bound-to-seen-content |
| story-body-markdown | server-operated-only-from-own-screen |
| story-links-new-tab | server-operated-only-from-own-screen |
| highlight-fallback | owner-understands-each-change |
| config-file-required | mistakes-reported-with-fix |
| back-restores-state | owner-reviews-every-change |
| keep-user-whitespace | owner-understands-each-change |
| diff-whitespace-marked | owner-understands-each-change |
| entry-select-anywhere | owner-reviews-every-change |
| evidence-presence-three-states | item-detail-in-one-place |
| evidence-detail-no-truncate | item-detail-in-one-place |
| claim-at-commit-unknown-layer | agreement-history-kept |
| claim-at-commit-multiple-open | agreement-history-kept |
| claim-at-commit-toggle | agreement-history-kept |
| claim-at-commit-error-shown | agreement-history-kept |
| found-file-state-shown | item-detail-in-one-place |
| story-id-from-trailer | changes-traceable-to-story |
| branch-point-merge-base | check-passes-only-when-agreed |
| story-trailer-at-most-one | changes-traceable-to-story |
| story-trailer-matches-pattern | changes-traceable-to-story |
| story-trailer-errors-shown | changes-traceable-to-story |
| agree-commit-has-story-trailer | agreement-history-kept |
| term-page-content | item-detail-in-one-place |
| feature-page-content | item-detail-in-one-place |
| term-feature-page-claims | item-detail-in-one-place |
| review-term-feature-claims | item-detail-in-one-place |
| term-feature-page-missing | item-detail-in-one-place |
| claim-detail-shown-before-history | item-detail-in-one-place |
| review-detail-shown-before-list | item-detail-in-one-place |
| loading-indicator | owner-reviews-every-change |

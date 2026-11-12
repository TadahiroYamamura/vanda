# 約束リストの作り直し: 上の側の約束の草案

PO が直すためのたたき台。今の約束リスト（約束 298・用語 75・機能 34）を元に、`.claude/rules/promises.md` の基準で、機能が実現する「上の側の約束」を書き出した。約束リスト自体はまだ変えない。

- 関係: 上の側の約束 ←具体化― 機能 ←具体化― 受け入れ基準の約束 ←supports― エビデンス
- 「受け入れ基準の候補」は今の約束の ID。代表だけを挙げ、全部ではない
- 正しさの略: 規＝規範的、機＝機能的、目＝目的的
- 機能の欄の太字は、2. の表で「残す」とした機能（名前を変える案を含む）

## 1. 上の側の約束（21 件）

### A. 約束リストとアノテーションを正しく書ける

| ID の案 | 文 | 正しさ | 大まかな反証の条件 | 機能の案 | 受け入れ基準の候補 |
|---|---|---|---|---|---|
| `mistakes-reported-with-fix` | 約束リスト・承認の記録・設定・アノテーションの誤りと、vanda が処理を続けられない状態を、黙って見逃さず、どのファイルの何行目の何がいけないかとどう直せばよいかを添えて知らせる | 機・目 | 誤りがあるのにチェックが通る、画面が黙って空になる、または知らせを読んだ人が直す場所か直し方を分からない | **read-promise-list**、**read-annotations**、**run-checks**（設定の読み込みを含む） | promise-list-unknown-entries-rejected、promise-list-required-fields、promise-list-id-unique、promise-list-subjects-exist、reference-broken、config-unknown-entries-rejected、config-pattern-syntax、check-error-form、check-unread-files、api-error-shape、api-error-fix-shown、review-error-shown、story-error-shown-on-screen |
| `annotations-link-evidence` | 開発者は、テスト・モデル・シナリオ・ガードレールの定義などに付けたアノテーションで、それが支える約束・用語・機能を示すことができ、指す先のないアノテーションはチェックで見つかる | 機・規 | 正しく付けたアノテーションが数えられない、指す先のないアノテーションや書けない場所のアノテーションでチェックが通る | **read-annotations** | annotation-syntax-errors、annotation-outside-comment-ignored、annotation-go-func-doc、annotation-ts-test-call、annotation-ts-only-rejected、annotation-links-kinds、annotation-links-tla-types、annotation-one-rule-per-file |
| `adapter-contract-stable` | 外の人は、`docs/formats.md` に書いた取り決めどおりにカスタムアダプタを作れば、組み込みのアダプタにない言語のアノテーションと、ほかのストーリーストアを vanda につなげられる | 規・機 | 取り決めどおりのカスタムアダプタが動かない、取り決めにない答えを黙って受け入れる、v1.0.0 の後に取り決めが変わる | **run-custom-adapters** | project-adapter-name、project-adapter-args-and-cwd、project-adapter-reads-committed-content、project-adapter-response-errors、project-adapter-missing-answer、project-adapter-annotation-timeout、story-project-adapter |

### B. CI のチェックが、承認とエビデンスの抜けを止める

| ID の案 | 文 | 正しさ | 大まかな反証の条件 | 機能の案 | 受け入れ基準の候補 |
|---|---|---|---|---|---|
| `check-passes-only-when-agreed` | 承認のチェックが通れば、対象の約束・用語・機能・承認の対象のファイル（とそれを消したこと）は、どれも今の中身で PO に承認されている | 機 | PO が今の中身に承認していないもの（消したものを含む）があるのに、承認のチェックが通る | **run-checks** | pass-only-when-all-seen、story-pass-only-when-seen、check-agreement-targets、deleted-item-needs-agreement、check-agreement-flags-left、check-agreement-story-scope、agreements-point-to-existing-items |
| `check-passes-only-with-evidence` | エビデンス完備のチェックが通れば、約束に予定したエビデンスのうちアノテーションで確かめられるものは、どれも揃っている | 機 | 予定したエビデンスが揃っていない（スキップしたテスト、定義だけのガードレール、印だけのセンサーを含む）のに通る | **run-checks**、**read-annotations** | check-evidence-present、check-evidence-guardrail-needs-definition、check-evidence-sensor-needs-check、check-evidence-proof-and-unchecked、annotation-ts-skipped-not-counted、check-guardrail-file-missing、annotation-types-empty |
| `check-runs-in-ci` | 利用者は、`docs/formats.md` に書いた呼び方で CI から vanda check を動かし、通ったか・何が失敗したかを終了コードと出力で知ることができる | 規・機 | 取り決めの呼び方で動かない、失敗があるのに終了コードが 0、どのチェックの何が失敗したかが出力から分からない | **run-checks** | check-config-required、check-base-required-for-story、check-runs-in-order-and-fails、check-errors-before-run、check-output-lines、check-blocked-by-unreadable-input、check-story-from-commits |

### C. PO がストーリーの変更を漏れなく読み、承認できる

| ID の案 | 文 | 正しさ | 大まかな反証の条件 | 機能の案 | 受け入れ基準の候補 |
|---|---|---|---|---|---|
| `po-reviews-every-change` | PO は、ストーリー（またはブランチ）で変わった約束・用語・機能と承認の対象のファイルを、漏れも重複もなく閲覧し、1 件ずつ承認できる | 機・目 | 変わったものが一覧から漏れる・2 度出る、変わったものを一覧の中で探せない | **list-changes**（「ストーリーをレビューする」に名前を変える案） | review-no-omission-no-duplicate、review-count-verified、review-renamed-file、review-deleted-shown、review-view-branch、review-view-story、context-bases、review-filter-text、review-filter-not-agreed、back-restores-state |
| `po-understands-each-change` | PO は、1 件ごとに何がどう変わったかを、ソースコードや git を読まずに理解できる | 目 | 詳細を見た PO が、変更の中身を判断するためにファイルや git の差分を開く必要がある、文の中の参照が何を指すか分からない、TLA+ を読み違える | **compare-contents** | review-before-after、review-added-content、review-file-diff-lines、review-evidence-diff-pairing、reference-shown、reference-popup、reference-diff、tla-math-symbols、keep-user-whitespace、diff-whitespace-marked、highlight-fallback、review-detail-shown-before-list |
| `po-notices-what-needs-care` | PO は、注意して読むべきもの（AI 案・推測・承認の後の変更・失効した承認）を、1 件を開く前に見分けられる | 目 | フラグや失効した承認があるのに一覧で気づけない、失効した承認が変わったものの一覧の外にあって見落とす | **raise-flags** | review-flags、review-icon-names、detached-agreement、detached-until-story-reagrees、review-detached-shown、review-detached-unchanged-shown |
| `po-reads-story-with-review` | PO は、レビューしている変更が何のためか（ストーリーの題・本文・添付ファイル）を、同じ画面で読める | 目 | ストーリーストアにあるストーリーが画面に出ない、読めない理由が分からないまま空になる | **connect-story-store** | story-shown-on-screen、story-github-read、story-github-attachments、story-body-markdown、story-errors、story-error-shown-on-screen、story-undecided-note、context-story-store |

### D. 承認が、承認した中身を正しく表し、記録が壊れない

| ID の案 | 文 | 正しさ | 大まかな反証の条件 | 機能の案 | 受け入れ基準の候補 |
|---|---|---|---|---|---|
| `agreement-follows-content` | 承認は、PO が見た時点の中身に結びつき、中身が変われば（ID や場所を変えた時を含む）承認済でなくなる | 機 | 中身・ID・場所が変わったのに承認済のまま、または書き方（TOML の書き方・フラグ）の違いだけで承認済でなくなる | **agree-on-review**（コア機能・形式手法） | agreed-when-fingerprint-matches、agree-rejects-stale、changed-id-is-new、renamed-file-needs-agreement、fingerprint-excludes-flags、fingerprint-ignores-formatting、fingerprint-as-written、fingerprint-file-content |
| `agreement-history-kept` | 承認と取り消しは、誰が・いつ・どのストーリーで・どの中身に行ったかが、承認の記録と git の履歴に残り、あとから辿れる | 機・規 | 承認か取り消しの後に、誰の・どのストーリーの承認かが記録や履歴から分からない、古い承認の記録が消える | **agree-on-review**、**revoke-agreement** | agree-writes-and-commits、agree-record-user-from-git、agree-commit-has-story-trailer、old-agreements-kept-as-history、deletion-agreement-record、revoke-commits-and-keeps-flags、context-no-user-cannot-agree |
| `agreement-keeps-files-safe` | 承認と取り消しは、約束リストと承認の記録を壊さず、承認していない変更をコミットに巻き込まない | 機 | 承認か取り消しの後に、約束リストか承認の記録が読めない、ほかの変更が承認のコミットに入る、同時の操作で記録が欠ける | **agree-on-review**、**revoke-agreement** | agree-rejects-uncommitted、serve-agree-one-at-a-time、agree-inline-flags-refused、agree-removes-flags、agree-rejects-duplicate、agree-record-file |
| `revoke-own-agreement-only` | 利用者は、自分が今のブランチで行った承認を取り消すことができ、ほかの人の承認やほかのブランチ・ほかのストーリーの承認は消えない | 機 | 取り消した後もその利用者の承認が残る、ほかの人・ほかのストーリーの承認の記録が消える | **revoke-agreement**（コア機能・形式手法） | revoke-own-branch-only、revoke-removes-all-duplicates、revoke-returns-to-proposed、revoke-refused、revoke-others-shown、revoke-reason-shown |
| `screen-matches-check` | PO が画面で見る中身と判定（承認の状態、エビデンスが揃っているか）は、CI の vanda check が使うコミットした中身と判定に一致する | 機 | 同じコミットで、画面では承認済・揃っているのに check が失敗する（またはその逆）、コミットしていない変更で画面の判定が変わる | **show-evidence-status**、**list-changes** | reads-committed-files-only、evidence-presence-shown、evidence-presence-three-states、found-file-state-shown、review-state-shown、serve-reloads-config |

### E. 約束リストの全体と履歴を辿れる

| ID の案 | 文 | 正しさ | 大まかな反証の条件 | 機能の案 | 受け入れ基準の候補 |
|---|---|---|---|---|---|
| `changes-traceable-to-story` | 約束リストの変更は、どれもストーリーに結びつき、どのストーリーで何が変わったかを辿れる | 機 | ストーリーのトレーラーのないコミットで約束リストを変えてもチェックが通る、変更履歴のコミットからストーリーが分からない | **run-checks**、**show-claim-detail** | check-story-origin、story-id-from-trailer、story-trailer-at-most-one、story-trailer-matches-pattern、story-trailer-errors-shown、origin-commit-stories、review-other-commits |
| `promise-detail-in-one-place` | PO は、1 つの約束について、中身・エビデンスの予定と実際・変更履歴・承認の履歴・過去の時点の中身を、1 か所で確かめられる | 目 | 約束の詳細を見た PO が、そのどれかを知るためにファイルや git を開く必要がある | **show-claim-detail**（今の show-claim-fields の名前を変える案）、**show-evidence-status** | claim-detail-fields、claim-detail-state、claim-detail-evidence-content、claim-detail-found-location、claim-detail-origin、claim-detail-agreements、claim-at-commit、detail-origin-and-history、claim-detail-shown-before-history |
| `browse-by-term-and-feature` | PO は、用語と機能から、それを対象にする約束を辿れる | 目 | 用語か機能を対象にする約束が、その用語・機能の詳細から見つからない | **show-term-feature-pages** | term-page-content、feature-page-content、term-feature-page-claims、review-term-feature-claims、term-feature-page-missing |

### F. 使い始められ、安全に動く

| ID の案 | 文 | 正しさ | 大まかな反証の条件 | 機能の案 | 受け入れ基準の候補 |
|---|---|---|---|---|---|
| `start-with-init` | 新しいリポジトリで使い始める人は、端末の質問に答えるだけで、すぐ vanda check と vanda doctor が通る .vanda/ を作れ、すでにある .vanda/ は壊されない | 目・機 | vanda init の後に手で直さないとチェックか doctor が通らない、すでにある .vanda/ が書き換わる、途中でやめると作りかけのファイルが残る | **create-vanda-dir** | init-defaults-from-repository、init-writes-files、init-check-configs、init-keeps-existing、init-decline-creates-nothing、init-write-failure-cleans、init-check-failure-keeps、init-validates-answers、init-restores-terminal、init-guide |
| `setup-diagnosable` | 利用者は、レビューやチェックの前に、設定とバージョンが合っているか、ストーリーストアにつながるかを確かめられる | 機・目 | 設定かバージョンかストーリーストアのつながりに問題があるのに vanda doctor が通る、バージョンの違うまま vanda check・serve が動く | **diagnose-setup** | doctor-ok-line、doctor-github-issues、doctor-github-token、doctor-project-adapter、config-version-mismatch、version-always-runs、config-file-required |
| `local-only-and-secret-safe` | vanda サーバーは、利用者の PC のブラウザの vanda の画面からしか操作できず、ストーリーストアのトークンは、それを使うホストにだけ送る | 規（セキュリティ） | ほかの PC・ほかのサイト・ストーリーの本文から開いたページから、承認・取り消し・ファイルの読み出しができる、トークンがほかのホストに送られる、暗号化されない時に知らせない | **start-server**、**connect-story-store** | serve-listens-localhost、serve-rejects-foreign-host、serve-rejects-foreign-origin、file-api、story-links-new-tab、story-github-token、story-github-token-missing、story-github-http-warning |

## 2. 今の機能 34 件の案

| 今の機能 | 案 | 理由 |
|---|---|---|
| start-server | 残す | `local-only-and-secret-safe` を実現する。画面の約束の入口でもある |
| read-promise-list | 残す | `mistakes-reported-with-fix` を実現する（約束リストの形を確かめる） |
| show-references | 機能でなくなる | 文の表示の部品。約束は `po-understands-each-change` の受け入れ基準に移す |
| store-agreements | まとめる → agree-on-review | 承認の記録の読み書きは、承認と取り消しの部品 |
| build-origins | 機能でなくなる | 変更履歴は約束の詳細の部品。約束は `changes-traceable-to-story`・`promise-detail-in-one-place` へ |
| choose-base-branch | まとめる → list-changes | 比べる元を選ぶのは、レビューの一部 |
| connect-story-store | 残す | `po-reads-story-with-review` を実現する |
| store-config | 機能でなくなる | 設定は用語（設定のファイル）で、実現する約束を単独で言えない。約束は `mistakes-reported-with-fix`・`setup-diagnosable` へ |
| list-changes | 残す（「ストーリーをレビューする」に名前を変える案） | `po-reviews-every-change` を実現する |
| number-changes | 機能でなくなる | 通し番号と総数は手段。テストに置く |
| raise-flags | 残す | `po-notices-what-needs-care` を実現する |
| compare-contents | 残す | `po-understands-each-change` を実現する |
| agree-on-review | 残す | `agreement-follows-content` などを実現する。コア機能 |
| show-evidence-status | 残す | `screen-matches-check`・`promise-detail-in-one-place` を実現する |
| fold-groups | 機能でなくなる | 画面の部品。たたむのは `po-reviews-every-change` の手段 |
| switch-review-unit | まとめる → list-changes | レビュー単位はレビューの一部 |
| show-detached | まとめる → raise-flags | 失効した承認も「注意して読むもの」として示す |
| show-entry-history | まとめる → show-claim-detail | レビューの 1 件の履歴は、約束の詳細と同じ中身 |
| revoke-agreement | 残す | `revoke-own-agreement-only` を実現する。コア機能 |
| agree-on-detail-pages | まとめる → agree-on-review | 承認する画面が違うだけで、同じ承認 |
| show-term-feature-pages | 残す | `browse-by-term-and-feature` を実現する |
| show-claim-fields | 残す（show-claim-detail「約束の詳細を示す」に名前を変え、下の 4 件をまとめる案） | `promise-detail-in-one-place` を実現する |
| show-claim-evidence | まとめる → show-evidence-status | 見せるものはエビデンスの予定と実際で同じ |
| show-claim-origin | まとめる → show-claim-detail | 約束の詳細の一部 |
| show-claim-agreements | まとめる → show-claim-detail | 約束の詳細の一部 |
| render-tla-symbols | 機能でなくなる | 表示の細部。約束は `po-understands-each-change` の受け入れ基準に移す |
| show-claim-at-commit | まとめる → show-claim-detail | 約束の詳細の一部 |
| diagnose-setup | 残す | `setup-diagnosable` を実現する |
| run-checks | 残す | `check-passes-only-when-agreed`・`check-passes-only-with-evidence`・`check-runs-in-ci` を実現する |
| list-check-kinds | まとめる → run-checks | チェックの種類は vanda check の中身 |
| create-vanda-dir | 残す | `start-with-init` を実現する |
| show-version | まとめる → diagnose-setup | バージョンを確かめるのは `setup-diagnosable` の一部 |
| read-annotations | 残す | `annotations-link-evidence` を実現する |
| run-custom-adapters | 残す | `adapter-contract-stable` を実現する（外との取り決め） |

集計: 残す 16（うち名前を変える案 2）、まとめる 12、機能でなくなる 6。案のとおりなら機能は 16 件になる。

## 3. どの上の側の約束にも入らない今の約束

多くは手段（並び・番号・文言・既定の値）で、規則に沿えばテストに移す。上の側の約束を足すかは「4. 迷った所」に書いた。

| 傾向 | 代表の ID | 理由 |
|---|---|---|
| 一覧の並び・番号・見た目 | review-numbering、review-order、review-group-first-layer、review-groups-fold、review-groups-fold-over-ten、review-change-counts、review-list-first-line、review-list-icons、review-flag-labels、entry-select-anywhere、review-filter-count、review-filter-kept、review-empty-note、loading-indicator、context-default-base、context-base-always-shown | 正しさを実現する手段。`po-reviews-every-change` などのテストに置く |
| 文言・出力の書式 | agree-commit-message、revoke-commit-message、agree-button-label、agree-cannot-reason-shown、check-output-story-id、check-error-order、agree-record-form、agree-from-detail-pages-notes、init-values-only | 文言と書式は手段。外の取り決め（`docs/formats.md`）に当たるものだけ `check-runs-in-ci`・`agreement-history-kept` の受け入れ基準に残す |
| TLA+ の描き方の細部 | tla-symbol-font、tla-symbol-keeps-width、tla-copy-ascii、tla-no-replace-in-strings-comments | 表示の細部。tla-math-symbols だけを `po-understands-each-change` に残し、ほかはテストへ |
| フィンガープリントの作り方の細部 | fingerprint-form、fingerprint-item-json、fingerprint-json-strings | 外の人が承認の記録を作るなら外の取り決め（規範的）、そうでなければ手段 |
| 約束リスト・設定の決まった値の一覧 | promise-list-required-field-names、promise-list-fixed-value-list、check-scope-table、check-name-known | 列挙は `docs/formats.md` とテストに置き、「決まっていない値は誤りにする」約束だけ残す |
| 設定の既定の値・ファイルがない時の扱い | promise-list-reads-all-patterns、promise-list-default-file-may-be-absent、agreements-default-file-may-be-absent、agreements-explicit-file-may-be-absent、config-id-pattern-default、config-api-url-default、annotation-types-unsatisfiable-allowed | 設定の解釈で、受け入れる上の側の約束がない。外の取り決めとみなすか決めてほしい |
| サーバーの起動の使い勝手 | serve-prints-url、serve-port-unavailable、serve-browser-open-failure、header-context | start-server が実現する約束がセキュリティだけになるため、受け入れる先がない |
| vanda init の端末の扱い | init-requires-terminal、init-terminal-streams、init-reads-answers-sent-after-prompt、init-accepts-builtin-name、init-missing-project-adapter | `start-with-init` に入れられるが、受け入れ基準の粒度より細かい |
| 状態のない閲覧の細部 | claim-at-commit-multiple-open、claim-at-commit-toggle、claim-at-commit-unknown-layer、claim-detail-subjects、claim-detail-guesses、claim-detail-evidence-grouping、evidence-detail-no-truncate、review-detail-links | `promise-detail-in-one-place` の手段 |

## 4. 迷った所・PO に決めてほしい所

- **新しい用語が要る**: 上の側の約束の主語に「PO」「開発者」「外の人（アダプタを作る人）」「利用者」を使った。今の用語にはどれもない（定義や理由の文に「PO」「利用者」が出てくるだけ）。用語にするか、言い方を決めてほしい。「レビュー単位」も約束の文にあるが用語にない
- **約束リストの形が関係を表せない**: 今の形では、約束の対象（subjects）は用語か機能だけで、「上の側の約束 ←具体化― 機能」「機能 ←具体化― 受け入れ基準の約束」を書く項目がない。また約束には evidence が必須なので、上の側の約束に何をエビデンスとして書くか（受け入れ基準の約束で足りるとみなすか、human-review を書くか）を決める必要がある
- **`screen-matches-check` を独立させるか**: `check-passes-only-when-agreed`・`check-passes-only-with-evidence` と重なる。分けたのは、「画面で揃って見えるのに CI で落ちる」食い違いが PO にとって別の害だから
- **`mistakes-reported-with-fix` が大きい**: 約束リスト・設定・アノテーション・API・画面の誤りをまとめた。約束リストの形のチェック（機能的）と、誤りの伝え方（目的的）の 2 つに分ける案もある
- **失効した承認をフラグと同じ機能にまとめてよいか**: 失効した承認は用語「フラグ」ではない。まとめるなら機能の名前を「注意して読むものを示す」などに変える必要がある
- **start-server を機能として残すか**: 実現する約束がセキュリティだけになる。「利用者は 1 つのコマンドで画面を開ける」のような目的的な上の側の約束を足すか、start-server を機能から外すか
- **store-config を機能から外してよいか**: 設定の既定の値・ファイルがない時の扱い（3. の表）を受ける上の側の約束がなくなる
- **外の取り決めの範囲**: CLAUDE.md の「`.vanda/` の外の取り決め」に、承認の記録の形やフィンガープリントの作り方は入るか。入るなら `agreement-history-kept` と fingerprint-* を規範的として受け入れ基準に残す
- **コア機能・形式手法の扱い**: agree-on-review に store-agreements と agree-on-detail-pages をまとめると、まとめた約束も形式手法の対象（モデル検査か waiver）になる
- **機能の名前を変える案**: list-changes →「ストーリーをレビューする」、show-claim-fields → show-claim-detail「約束の詳細を示す」。ID を変えると承認し直しになる

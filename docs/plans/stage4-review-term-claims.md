# レビューの用語・機能の詳細に、関わる約束を並べる（段 4 の 6 の直し）

## 約束

- review-term-feature-claims: ストーリーのレビューで、追加・変更した用語・機能の 1 件の詳細に、term-feature-page-claims と同じ約束の一覧を示す
- term-feature-page-claims（今の約束）: 用語と機能の詳細のページに、それを subjects に持つ約束を、約束リストに書いた順に、ID と文で並べ、それぞれをその約束の詳細へのリンクにする
- 読み込みの間の見せ方は、今の約束（loading-indicator など）に従う

## 作り方（開発者）

1. `web/src/pages/TermFeaturePage.tsx` の `ClaimList` を、ほかの画面からも使えるようにする（置き場所は開発者が決めてよい。見出しと「〜を対象に持つ約束はありません。」の文言は今のまま）
2. `web/src/pages/ReviewPage.tsx` の 1 件の詳細で、種類が用語か機能で、変更の種類が追加か変更のときに、今ある `api.term(id)`・`api.feature(id)`（`/api/terms/<ID>`・`/api/features/<ID>`）を読み、その `claims` を `ClaimList` で、中身（名前・定義などの表）の下に出す
   - 削除した用語・機能には出さない
   - 読み込み中は、今の読み込み中のマーク（`Loading`）を出す。読めなければ、今の `ErrorBox` で理由を出す
   - 約束の詳細へのリンクには、レビューの画面の状態（query）を引き継ぐ（詳細のページと同じ `detailURL`）
3. API・サーバーは変えない

## テスト（テスト担当）

- 期待値は、約束 review-term-feature-claims・term-feature-page-claims の文から作る。実装を読んで期待値を作らない
- `web/src/__tests__/ReviewPage.test.tsx` に足す（msw で `/api/terms/<ID>`・`/api/features/<ID>` を差し替える）。新しいファイルは作らない
- 確かめること（変更の種類 追加・変更・削除 と、用語・機能の組を全部回す）
  - 追加・変更した用語・機能では、関わる約束が、渡した順に ID と文で並び、ID が約束の詳細へのリンクになっている（query を引き継ぐ）
  - 関わる約束がなければ、「この用語を対象に持つ約束はありません。」（機能なら「この機能を…」）を出す
  - 削除した用語・機能では、一覧を出さない
- アノテーションで約束 review-term-feature-claims（unit-test、gui）とつなぐ
- 判定をわざと壊して（削除にも出す、順を逆にする など）、テストが落ちることを確かめる

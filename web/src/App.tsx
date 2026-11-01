import { api } from "./api";
import { LayerNames } from "./components/Chips";
import { ErrorBox, useLoad } from "./components/common";
import { ClaimPage, FilePage } from "./pages/ClaimPage";
import { ReviewPage } from "./pages/ReviewPage";
import { Link, useRoute } from "./route";
import "./styles.css";

export function App() {
  const route = useRoute();
  const ctx = useLoad(() => api.context(), "context");
  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="logo">
          vanda
        </Link>
        <nav>
          <Link to="/" className={route.page === "review" ? "current" : undefined}>
            ストーリーのレビュー
          </Link>
        </nav>
        {ctx.data && (
          <span className="where">
            {ctx.data.branch ? `ブランチ ${ctx.data.branch}` : "ブランチにいません"} · {ctx.data.user ? `${ctx.data.user.name} <${ctx.data.user.email}>` : "git の user.name・user.email が設定されていません"} · vanda {ctx.data.version}
          </span>
        )}
      </header>
      <LayerNames.Provider value={ctx.data?.layers ?? []}>
      <main>
        {ctx.error != null && <ErrorBox error={ctx.error} title="vanda の設定を読めません" />}
        {ctx.data && ctx.data.trailerErrors.length > 0 && (
          <section className="error" role="alert" aria-label="Story: のトレーラーの誤り">
            <strong>次のコミットの Story: のトレーラーに誤りがあるため、ストーリーの分からないコミットとして扱っています。</strong>
            <ul>
              {ctx.data.trailerErrors.map((t) => (
                <li key={t.commit}>
                  <code>{t.commit.slice(0, 7)}</code>「{t.subject}」: {t.message}
                </li>
              ))}
            </ul>
            <div className="fix">コミットのメッセージの末尾の Story: を、1 つだけ、設定の id_pattern に合う形で書いてください。</div>
          </section>
        )}
        {route.page === "claim" && <ClaimPage id={route.id} params={route.params} ctx={ctx.data} />}
        {route.page === "file" && <FilePage path={route.path} params={route.params} ctx={ctx.data} />}
        {route.page === "review" && ctx.data && <ReviewPage ctx={ctx.data} params={route.params} onAgreed={ctx.reload} />}
      </main>
      </LayerNames.Provider>
    </div>
  );
}

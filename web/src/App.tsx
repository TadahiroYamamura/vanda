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
        {route.page === "claim" && <ClaimPage id={route.id} params={route.params} ctx={ctx.data} />}
        {route.page === "file" && <FilePage path={route.path} params={route.params} ctx={ctx.data} />}
        {route.page === "review" && ctx.data && <ReviewPage ctx={ctx.data} params={route.params} onAgreed={ctx.reload} />}
      </main>
      </LayerNames.Provider>
    </div>
  );
}

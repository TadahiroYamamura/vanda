import { api } from "./api";
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
            {ctx.data.branch ? `ブランチ ${ctx.data.branch}` : "ブランチにいない"} · {ctx.data.user ? `${ctx.data.user.name} <${ctx.data.user.email}>` : "git の user.name・user.email がない"} · vanda {ctx.data.version}
          </span>
        )}
      </header>
      <main>
        {ctx.error != null && <ErrorBox error={ctx.error} title="vanda の設定を読めない" />}
        {route.page === "claim" && <ClaimPage id={route.id} />}
        {route.page === "file" && <FilePage path={route.path} />}
        {route.page === "review" && ctx.data && <ReviewPage ctx={ctx.data} params={route.params} onAgreed={ctx.reload} />}
      </main>
    </div>
  );
}

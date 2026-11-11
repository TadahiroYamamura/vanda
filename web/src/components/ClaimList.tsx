import type { ClaimSummary } from "../api";
import { Link } from "../route";
import { detailURL, Prose } from "./Prose";

// ClaimList は、用語・機能を subjects に持つ約束の一覧（term-feature-page-claims・review-term-feature-claims）。
// 用語・機能の詳細のページと、ストーリーのレビューの 1 件の詳細で使う
export function ClaimList(props: { claims: ClaimSummary[]; what: string; query: string; level: "h2" | "h3" }) {
  const H = props.level;
  return (
    <>
      <H>この{props.what}を対象に持つ約束</H>
      {props.claims.length === 0 ? (
        <p className="note">この{props.what}を対象に持つ約束はありません。</p>
      ) : (
        <ul className="claim-list">
          {props.claims.map((c) => (
            <li key={c.id}>
              <Link to={detailURL("claim", c.id, props.query)} className="claim-id">
                {c.id}
              </Link>
              <span className="user-text">
                <Prose pieces={c.statement} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

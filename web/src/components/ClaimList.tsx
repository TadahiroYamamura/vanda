import type { ClaimSummary } from "../api";
import { Link } from "../route";
import { detailURL, Prose } from "./Prose";

// ClaimList は、機能を refines する仕様の一覧。
// 機能の詳細のページと、ストーリーのレビューの 1 件の詳細で使う
export function ClaimList(props: { claims: ClaimSummary[]; query: string; level: "h2" | "h3" }) {
  const H = props.level;
  return (
    <>
      <H>この機能を refines する仕様</H>
      {props.claims.length === 0 ? (
        <p className="note">この機能を refines する仕様はありません。</p>
      ) : (
        <ul className="claim-list">
          {props.claims.map((c) => (
            <li key={c.id}>
              <Link to={detailURL("spec", c.id, props.query)} className="claim-id">
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

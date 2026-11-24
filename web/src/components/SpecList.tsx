import type { SpecSummary } from "../api";
import { Link } from "../route";
import { detailURL, Prose } from "./Prose";

// SpecList は、機能を refines する仕様の一覧。
// 機能の詳細のページと、ストーリーのレビューの 1 件の詳細で使う
export function SpecList(props: { specs: SpecSummary[]; query: string; level: "h2" | "h3" }) {
  const H = props.level;
  return (
    <>
      <H>この機能を具体化する仕様</H>
      {props.specs.length === 0 ? (
        <p className="note">この機能を具体化する仕様はありません。</p>
      ) : (
        <ul className="spec-list">
          {props.specs.map((c) => (
            <li key={c.id}>
              <Link to={detailURL("spec", c.id, props.query)} className="spec-id">
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

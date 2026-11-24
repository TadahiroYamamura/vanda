import type { ItemRef } from "../api";
import { Link } from "../route";
import { detailURL } from "./Prose";

// ItemRefs は、refines・assumes の先の項目を並べ、それぞれの詳細のページへのリンクにする。
// 用語と機能は名前と ID を、名前を持たない性質・前提・仕様は ID だけを出す
export function ItemRefs(props: { items: ItemRef[]; query: string }) {
  if (props.items.length === 0) return <>（なし）</>;
  return (
    <ul>
      {props.items.map((r) => (
        <li key={r.id}>
          {r.kind === null ? (
            <span className="user-text">{`${r.id}（約束リストにありません）`}</span>
          ) : (
            <Link to={detailURL(r.kind, r.id, props.query)}>
              {r.name !== null && <span className="user-text">{r.name}</span>}
              {r.name !== null ? (
                <>
                  （<code>{r.id}</code>）
                </>
              ) : (
                <code>{r.id}</code>
              )}
            </Link>
          )}
        </li>
      ))}
    </ul>
  );
}

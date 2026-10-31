import type { EvidenceContent, EvidenceMark } from "../api";
import { stageLabel, typeLabel } from "../labels";
import { Parts } from "./DiffView";

// EvidenceList は、エビデンスの予定を、エビデンス 1 つを 1 項目にしたリストで出す（ステージ / タイプ / レイヤーと、その下に詳細）。
// 変更前と変更後の表、その時点の中身（CL-08）で、同じ出し方にそろえる。詳細は改行を保つ。
// marks（API の diff.evidence）があれば、足した・消したエビデンスに印を付け、中身の変わったエビデンスは文字の単位の差分を出す
export function EvidenceList(props: { items: EvidenceContent[]; marks?: EvidenceMark[]; side?: "before" | "after" }) {
  if (props.items.length === 0) return <span>（なし）</span>;
  const side = props.side ?? "after";
  return (
    <ul className="evidence-list">
      {props.items.map((ev, i) => {
        const m = props.marks?.[i];
        const op = m?.op ?? "equal";
        return (
          <li key={i} className={`evidence-item evidence-${op}`}>
            {op === "added" && <span className="badge evidence-badge evidence-badge-added">足した</span>}
            {op === "removed" && <span className="badge evidence-badge evidence-badge-removed">消した</span>}
            <span className="evidence-head">
              {stageLabel[ev.stage] ?? ev.stage} / {typeLabel[ev.type] ?? ev.type} / {op === "modified" && m?.layers ? <Parts parts={m.layers} side={side} /> : ev.layers.join("・")}
            </span>
            {op === "modified" && m?.detail && m.detail.length > 0 ? (
              <div className="evidence-detail">
                <Parts parts={m.detail} side={side} />
              </div>
            ) : (
              ev.detail && <div className="evidence-detail">{ev.detail}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

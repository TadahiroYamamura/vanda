import type { EvidenceContent } from "../api";
import { stageLabel, typeLabel } from "../labels";

// EvidenceList は、エビデンスの予定を、エビデンス 1 つを 1 項目にしたリストで出す（ステージ / タイプ / レイヤーと、その下に詳細）。
// 変更前と変更後の表、その時点の中身（CL-08）で、同じ出し方にそろえる。詳細は改行を保つ
export function EvidenceList(props: { items: EvidenceContent[] }) {
  if (props.items.length === 0) return <span>（なし）</span>;
  return (
    <ul className="evidence-list">
      {props.items.map((ev, i) => (
        <li key={i} className="evidence-item">
          <span className="evidence-head">{evidenceHead(ev)}</span>
          {ev.detail && <div className="evidence-detail">{ev.detail}</div>}
        </li>
      ))}
    </ul>
  );
}

export function evidenceHead(ev: EvidenceContent): string {
  return `${stageLabel[ev.stage] ?? ev.stage} / ${typeLabel[ev.type] ?? ev.type} / ${ev.layers.join("・")}`;
}

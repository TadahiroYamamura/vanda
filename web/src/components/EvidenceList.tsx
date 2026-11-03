import type { EvidenceContent, EvidenceMark } from "../api";
import { stageLabel, typeLabel } from "../labels";
import { LayerChips, LayerChipsDiff } from "./Chips";
import { Parts } from "./DiffView";
import { Prose } from "./Prose";

// EvidenceList は、エビデンスの予定を、エビデンス 1 つを 1 項目にしたリストで出す（「ステージ · タイプ」とレイヤーの札の行と、その下に詳細）。
// 変更前と変更後の表、その時点の中身（CL-08）で、同じ出し方にそろえる。詳細は改行を保つ。
// marks（API の diff.evidence）があれば、足した・消したエビデンスに印を付け、中身の変わったエビデンスは、レイヤーの札と詳細の文字に差分の印を付ける。
// others は、marks の pair が指す、もう片方の側のエビデンス（レイヤーの札の差分に使う）
export function EvidenceList(props: { items: EvidenceContent[]; marks?: EvidenceMark[]; side?: "before" | "after" | "both"; others?: EvidenceContent[] }) {
  if (props.items.length === 0) return <span>（なし）</span>;
  const side = props.side ?? "after";
  return (
    <ul className="evidence-list">
      {props.items.map((ev, i) => {
        const m = props.marks?.[i];
        const op = m?.op ?? "equal";
        const other = m?.pair != null ? props.others?.[m.pair] : undefined;
        const layersNode =
          op === "modified" && other ? (
            <LayerChipsDiff before={side === "before" ? ev.layers : other.layers} after={side === "before" ? other.layers : ev.layers} side={side} />
          ) : undefined;
        return (
          <li key={i} className={`evidence-item evidence-${op}`}>
            <div className="evidence-head">
              {op === "added" && <span className="badge evidence-badge evidence-badge-added">足した</span>}
              {op === "removed" && <span className="badge evidence-badge evidence-badge-removed">消した</span>}
              <span className="evidence-kind">
                {stageLabel[ev.stage] ?? ev.stage} · {typeLabel[ev.type] ?? ev.type}
              </span>
              {layersNode ?? <LayerChips layers={ev.layers} />}
            </div>
            {op === "modified" && m?.detail && m.detail.length > 0 ? (
              <div className="evidence-detail">
                <Parts parts={m.detail} side={side} />
              </div>
            ) : (
              ev.detail && (
                <div className="evidence-detail">
                  <Prose pieces={ev.detail} />
                </div>
              )
            )}
          </li>
        );
      })}
    </ul>
  );
}

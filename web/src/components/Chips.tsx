import { createContext, useContext } from "react";
import type { LayerRef } from "../api";

// LayerNames は、今の設定のレイヤーの ID から名前を引くための一覧（/api/context の layers）。App が渡す
export const LayerNames = createContext<LayerRef[]>([]);

export function useLayerName(): (id: string) => string {
  const layers = useContext(LayerNames);
  return (id) => layers.find((l) => l.id === id)?.name || id;
}

// LayerChip は、レイヤーの札（レイヤーは利用者が決める値で、1 つのエビデンスや約束に複数付くので、札で並べる）。
// 触れたときの説明と読み上げに「レイヤー: モデル」のように出す
function LayerChip(props: { text: string; mark?: "delete" | "insert" }) {
  const label = `レイヤー: ${props.text}`;
  const chip = (
    <span className="chip chip-layer" title={label} aria-label={label} role="img">
      {props.text}
    </span>
  );
  if (props.mark === "delete") return <del className="chip-mark">{chip}</del>;
  if (props.mark === "insert") return <ins className="chip-mark">{chip}</ins>;
  return chip;
}

// LayerChips は、レイヤーの札を並べる。名前は names（その時点の名前など）があればそれを、なければ今の設定から引き、ID しかなければ ID
export function LayerChips(props: { layers: string[]; names?: LayerRef[] }) {
  const name = useLayerName();
  return (
    <span className="chips">
      {props.layers.map((id) => (
        <LayerChip key={id} text={props.names?.find((l) => l.id === id)?.name || name(id)} />
      ))}
    </span>
  );
}

// LayerChipsDiff は、レイヤーの文字の差分（API の「・」でつないだ文字の差分）から、消したレイヤーと足したレイヤーに印を付けた札を並べる
export function LayerChipsDiff(props: { before: string[]; after: string[]; side: "before" | "after" | "both" }) {
  const name = useLayerName();
  const removed = props.before.filter((l) => !props.after.includes(l));
  const added = props.after.filter((l) => !props.before.includes(l));
  const list: [string, "delete" | "insert" | undefined][] = [];
  const base = props.side === "before" ? props.before : props.after;
  for (const l of base) list.push([l, props.side !== "before" && added.includes(l) ? "insert" : props.side === "before" && removed.includes(l) ? "delete" : undefined]);
  if (props.side === "both") for (const l of removed) list.push([l, "delete"]);
  return (
    <span className="chips">
      {list.map(([id, mark]) => (
        <LayerChip key={id + (mark ?? "")} text={name(id)} mark={mark} />
      ))}
    </span>
  );
}

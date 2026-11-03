import { createContext, type ReactNode, useContext, useId } from "react";
import type { Piece, Refs } from "../api";
import { Link } from "../route";
import { useTooltip } from "./common";

// 参照の指す先（API の refs）と、約束の詳細へのリンクに引き継ぐクエリ。片を出す所は、応答ごとに入れ子が深いので、props で渡さずに文脈で渡す
const RefsContext = createContext<{ refs: Refs; query: string }>({ refs: {}, query: "" });

// RefsProvider は、応答の refs を渡す。query を省けば、外の文脈のものを引き継ぐ
export function RefsProvider(props: { refs: Refs | undefined; query?: string; children: ReactNode }) {
  const parent = useContext(RefsContext);
  return <RefsContext.Provider value={{ refs: props.refs ?? {}, query: props.query ?? parent.query }}>{props.children}</RefsContext.Provider>;
}

// proseText は、片の見せる文字をつないだ文。絞り込みと、同じかどうかの比べに使う
export function proseText(ps: Piece[] | null | undefined): string {
  return (ps ?? []).map((p) => p.text).join("");
}

// RefView は、参照 1 つ。触れたとき・フォーカスしたときに、指す先の定義（用語・機能）か文（約束）を出す。
// 今の約束リストにある約束を指すものは、約束の詳細へのリンクにする（リンクがフォーカスを受けるので、外の span はフォーカスさせない）
export function RefView(props: { id: string; children: ReactNode }) {
  const { refs, query } = useContext(RefsContext);
  const tipId = useId();
  const t = useTooltip();
  const target = refs[props.id];
  const link = target?.kind === "claim" && target.current;
  return (
    <span ref={t.anchor} className={`ref ref-${target?.kind ?? "unknown"}`} data-ref={props.id} tabIndex={link ? undefined : 0} aria-describedby={target && !link ? tipId : undefined} {...t.handlers}>
      {link ? (
        <Link to={`/claims/${encodeURIComponent(props.id)}${query ? `?${query}` : ""}`} describedBy={tipId}>
          {props.children}
        </Link>
      ) : (
        props.children
      )}
      {target && (
        <span id={tipId} ref={t.tip} role="tooltip" className="tooltip" hidden={!t.open} style={t.tipStyle}>
          {target.text}
        </span>
      )}
    </span>
  );
}

// Prose は、自然言語で書く項目の文（片の配列）を出す。
export function Prose(props: { pieces: Piece[] }) {
  return (
    <>
      {props.pieces.map((p, i) =>
        p.ref ? (
          <RefView key={i} id={p.ref}>
            {p.text}
          </RefView>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </>
  );
}

// ProseLines は、文の並び（推測）を 1 行に 1 つずつ出す。
export function ProseLines(props: { list: Piece[][] }) {
  return (
    <>
      {props.list.map((ps, i) => (
        <span key={i}>
          {i > 0 && "\n"}
          <Prose pieces={ps} />
        </span>
      ))}
    </>
  );
}

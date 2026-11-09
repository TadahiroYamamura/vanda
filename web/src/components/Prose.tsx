import { createContext, type ReactNode, useContext, useEffect, useId } from "react";
import type { Piece, RefTarget, Refs } from "../api";
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

// detailURL は、約束・用語・機能の詳細のページの URL。レビューの画面の状態（query）を引き継ぐ
export function detailURL(kind: RefTarget["kind"], id: string, query: string): string {
  const path = kind === "claim" ? "claims" : kind === "term" ? "terms" : "features";
  return `/${path}/${encodeURIComponent(id)}${query ? `?${query}` : ""}`;
}

// FeatureMarks は、コア機能・形式手法を使う機能であることの印。当てはまるものだけを出す
export function FeatureMarks(props: { core?: boolean; formal?: boolean }) {
  return (
    <>
      {props.core && <span className="badge mark-core">コア機能</span>}
      {props.formal && <span className="badge mark-formal">形式手法を使う機能</span>}
    </>
  );
}

// RefView は、参照 1 つ。触れたとき・フォーカスしたときに、指す先の中身を枠で出し、Esc で閉じる。
// 今の約束リストにある指す先は、その詳細のページへのリンクにする（リンクがフォーカスを受けるので、外の span はフォーカスさせない）
export function RefView(props: { id: string; children: ReactNode }) {
  const { refs, query } = useContext(RefsContext);
  const tipId = useId();
  const t = useTooltip();
  const target = refs[props.id];
  const link = target?.current === true;
  const { open, close } = t;
  useEffect(() => {
    if (!open) return;
    // マウスを載せただけで開いた枠も閉じられるよう、フォーカスの場所によらず画面全体で Esc を受ける
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);
  return (
    <span ref={t.anchor} className={`ref ref-${target?.kind ?? "unknown"}`} data-ref={props.id} tabIndex={link ? undefined : 0} aria-describedby={target && !link ? tipId : undefined} {...t.handlers}>
      {link ? (
        <Link to={detailURL(target.kind, props.id, query)} describedBy={tipId}>
          {props.children}
        </Link>
      ) : (
        props.children
      )}
      {target && (
        <span id={tipId} ref={t.tip} role="tooltip" className="ref-popup" hidden={!t.open} style={t.tipStyle}>
          <span className="ref-popup-head">
            <code>{props.id}</code>
            {target.kind === "feature" && <FeatureMarks core={target.core} formal={target.formal} />}
          </span>
          <span className="ref-popup-text">
            <PlainProse pieces={target.pieces} />
          </span>
        </span>
      )}
    </span>
  );
}

// PlainProse は、枠の中の文を出す。参照は参照の見た目にするだけで、リンクにも枠にもしない（枠から次の枠を開かないと PO が決めたため）
function PlainProse(props: { pieces: Piece[] }) {
  return (
    <>
      {props.pieces.map((p, i) =>
        p.ref ? (
          <span key={i} className="ref ref-plain" data-ref={p.ref}>
            {p.text}
          </span>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </>
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

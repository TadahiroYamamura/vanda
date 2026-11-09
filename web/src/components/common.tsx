import { type ReactNode, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { ApiError, type Flag, type State } from "../api";
import { detachedLabel, flagHelp, flagLabel, stateLabel } from "../labels";

export function ErrorBox(props: { error: unknown; title?: string }) {
  const e = props.error;
  const message = e instanceof Error ? e.message : String(e);
  const fix = e instanceof ApiError ? e.fix : "";
  return (
    <div className="error" role="alert">
      {props.title && <strong>{props.title}: </strong>}
      {message}
      {fix && <div className="fix">{fix}</div>}
    </div>
  );
}

// useTooltip は、マウスを乗せたとき・フォーカスしたときに出す説明の、開け閉めと位置を持つ。
// 説明は、画面に対して位置を決める（position: fixed）。マウスを乗せる所が一覧の欄（スクロールする欄）の中にあると、
// 欄に対して置いた説明は欄の端で切れるため。画面の右端に近いときは左に寄せ、長いときは折り返す
export function useTooltip() {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const anchor = useRef<HTMLSpanElement>(null);
  const tip = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    if (!open || !anchor.current || !tip.current) {
      setPos(null);
      return;
    }
    const margin = 8;
    const b = anchor.current.getBoundingClientRect();
    const w = tip.current.offsetWidth;
    const h = tip.current.offsetHeight;
    const left = Math.max(margin, Math.min(b.left, window.innerWidth - w - margin));
    // 下に収まらなければ、マウスを乗せた所の上に出す
    const below = b.bottom + 4;
    const top = below + h + margin > window.innerHeight ? Math.max(margin, b.top - 4 - h) : below;
    setPos({ left, top });
  }, [open]);
  useEffect(() => {
    if (!open) return;
    // 欄や画面がスクロールするとマウスを乗せた所と説明がずれるので、閉じる
    const close = () => setOpen(false);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
  }, [open]);
  return {
    anchor,
    tip,
    open,
    close: () => setOpen(false),
    handlers: {
      onMouseEnter: () => setOpen(true),
      onMouseLeave: () => setOpen(false),
      onFocus: () => setOpen(true),
      onBlur: () => setOpen(false),
    },
    tipStyle: pos ? { left: pos.left, top: pos.top } : { left: 0, top: 0, visibility: open ? ("hidden" as const) : undefined },
  };
}

// FlagBadge は、バッジに短い言葉だけを出し、説明はバッジにマウスを乗せたとき・フォーカスしたときに出す（RV-04）
export function FlagBadge(props: { flag: Flag }) {
  const id = useId();
  const t = useTooltip();
  return (
    <span ref={t.anchor} className={`badge flag flag-${props.flag}`} tabIndex={0} aria-describedby={id} {...t.handlers}>
      {flagLabel[props.flag]}
      <span id={id} ref={t.tip} role="tooltip" className="tooltip" hidden={!t.open} style={t.tipStyle}>
        {flagHelp[props.flag]}
      </span>
    </span>
  );
}

// 一覧のアイコンの形（PO が見本で選んだもの）。状態は丸の形、フラグは丸のない形にし、色だけに頼らずに見分けられるようにする（RV-01・RV-04）
export type ListIconName = State | "detached" | Flag;

const listIconShape: Record<ListIconName, ReactNode> = {
  agreed: (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <circle cx="8" cy="8" r="6.5" />
      <path d="M5 8.2l2 2 4-4.2" />
    </svg>
  ),
  proposed: (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" aria-hidden="true" focusable="false">
      <circle cx="8" cy="8" r="6.5" strokeDasharray="2.6 2" />
      <circle cx="8" cy="8" r="1.4" fill="currentColor" stroke="none" />
    </svg>
  ),
  detached: (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <circle cx="8" cy="8" r="6.5" />
      <path d="M8 4.6v4.2" />
      <circle cx="8" cy="11.2" r=".9" fill="currentColor" stroke="none" />
    </svg>
  ),
  "ai-proposal": (
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true" focusable="false">
      <path d="M7 1.5l1.3 3.6 3.6 1.3-3.6 1.3L7 11.3 5.7 7.7 2.1 6.4l3.6-1.3z" />
      <path d="M12.3 9.5l.6 1.6 1.6.6-1.6.6-.6 1.6-.6-1.6-1.6-.6 1.6-.6z" />
    </svg>
  ),
  guess: (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M5.2 5.4a2.9 2.9 0 1 1 4 2.7c-.8.4-1.2.9-1.2 1.8v.5" />
      <circle cx="8" cy="13.1" r=".95" fill="currentColor" stroke="none" />
    </svg>
  ),
  "changed-since-agreement": (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M8 2.5l5.5 10.5h-11z" />
    </svg>
  ),
};

const isFlag = (n: ListIconName): n is Flag => n in flagLabel;

// ListIcon は、一覧の状態とフラグのアイコン。マウスを乗せたとき・フォーカスしたときに言葉を出し、スクリーンリーダーにも同じ言葉を渡す（RV-01・RV-04）。
// フラグは、詳細のバッジと同じ説明を言葉に続けて出し、スクリーンリーダーには説明として渡す
export function ListIcon(props: { name: ListIconName }) {
  const id = useId();
  const t = useTooltip();
  const n = props.name;
  const word = isFlag(n) ? flagLabel[n] : n === "detached" ? detachedLabel : stateLabel[n];
  const help = isFlag(n) ? flagHelp[n] : null;
  return (
    <span
      ref={t.anchor}
      className={`list-icon ${isFlag(n) ? "list-icon-flag" : "list-icon-state"} list-icon-${n}`}
      tabIndex={0}
      role="img"
      aria-label={word}
      aria-describedby={help ? id : undefined}
      {...t.handlers}
    >
      {listIconShape[n]}
      <span ref={t.tip} role="tooltip" className="tooltip" hidden={!t.open} style={t.tipStyle}>
        {word}
        {help && (
          <span id={id} className="tooltip-help">
            {help}
          </span>
        )}
      </span>
    </span>
  );
}

export function StateBadge(props: { state: State | null }) {
  if (!props.state) return null;
  return <span className={`badge state state-${props.state}`}>{stateLabel[props.state]}</span>;
}

// Loading は、読み込みの終わっていない欄のマーク（loading-indicator）。スピナーは飾りなのでスクリーンリーダーに読み上げさせず、文字だけを読ませる
export function Loading(props: { className?: string }) {
  return (
    <p className={props.className ? `note loading ${props.className}` : "note loading"}>
      <span className="spinner" aria-hidden="true" />
      読み込んでいます…
    </p>
  );
}

// useLoad は、非同期の読み込みの結果と誤りを持つ。key が変わるたびに読み直す。
export function useLoad<T>(load: (() => Promise<T>) | null, key: string): { data: T | null; error: unknown; loading: boolean; reload: () => void } {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [n, setN] = useState(0);
  useEffect(() => {
    if (!load) {
      setData(null);
      setError(null);
      return;
    }
    let live = true;
    setLoading(true);
    setError(null);
    load().then(
      (d) => {
        if (live) {
          setData(d);
          setLoading(false);
        }
      },
      (e) => {
        if (live) {
          setData(null);
          setError(e);
          setLoading(false);
        }
      },
    );
    return () => {
      live = false;
    };
    // load は毎回作り直されるので、key と n で読み直しを決める
  }, [key, n]);
  return { data, error, loading, reload: () => setN((x) => x + 1) };
}

export function shortCommit(c: string) {
  return c.slice(0, 7);
}

// PresentIcon は、エビデンスが揃っているか（API の present）を、アイコンで示す（RV-08）。依存を足さないよう、インラインの SVG で書く
export function PresentIcon(props: { present: boolean | null }) {
  const [label, cls, path] =
    props.present === null
      ? ["アノテーションでは確かめません", "present-none", "M4 8h8"]
      : props.present
        ? ["揃っています", "present-yes", "M3.5 8.5l3 3 6-7"]
        : ["ありません", "present-no", "M4.5 4.5l7 7M11.5 4.5l-7 7"];
  return (
    <span className={`present-icon ${cls}`} role="img" aria-label={label} title={label}>
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
        <path d={path} fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </span>
  );
}

// AgreementIcon は、エビデンスの本体のファイルの承認の状態のアイコン（API の found の state）。
// 揃っているかのアイコン（丸のないチェック・バツ・横棒）と見間違えないよう、丸の中に描く。承認済は青の塗りの丸に白のチェック、
// 提案中は橙の枠の丸に時計の針（まだ決まっていない、待っている）
export function AgreementIcon(props: { state: State | null | undefined }) {
  if (!props.state) return null;
  const agreed = props.state === "agreed";
  const label = agreed ? "承認済" : "提案中";
  return (
    <span className={`agreement-icon ${agreed ? "agreement-agreed" : "agreement-proposed"}`} role="img" aria-label={label} title={label}>
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
        {agreed ? (
          <>
            <circle cx="8" cy="8" r="7" fill="currentColor" />
            <path d="M4.8 8.3l2.2 2.2 4.2-4.6" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
          </>
        ) : (
          <>
            <circle cx="8" cy="8" r="6.2" fill="none" stroke="currentColor" strokeWidth="1.6" />
            <path d="M8 4.6V8l2.4 1.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
          </>
        )}
      </svg>
    </span>
  );
}

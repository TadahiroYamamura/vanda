import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { ApiError, type Flag, type State } from "../api";
import { flagHelp, flagLabel, stateLabel } from "../labels";

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

// FlagBadge は、札に短い言葉だけを出し、説明は札に触れたとき（マウスを載せる・フォーカスする）に出す（RV-04）。
// 説明は、画面に対して位置を決める（position: fixed）。札が一覧の欄（スクロールする欄）の中にあると、
// 欄に対して置いた説明は欄の端で切れるため。画面の右端に近いときは左に寄せ、長いときは折り返す
export function FlagBadge(props: { flag: Flag }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const id = useId();
  const badge = useRef<HTMLSpanElement>(null);
  const tip = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    if (!open || !badge.current || !tip.current) {
      setPos(null);
      return;
    }
    const margin = 8;
    const b = badge.current.getBoundingClientRect();
    const w = tip.current.offsetWidth;
    const h = tip.current.offsetHeight;
    const left = Math.max(margin, Math.min(b.left, window.innerWidth - w - margin));
    // 下に収まらなければ、札の上に出す
    const below = b.bottom + 4;
    const top = below + h + margin > window.innerHeight ? Math.max(margin, b.top - 4 - h) : below;
    setPos({ left, top });
  }, [open]);
  useEffect(() => {
    if (!open) return;
    // 欄や画面がスクロールすると札と説明がずれるので、閉じる
    const close = () => setOpen(false);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
  }, [open]);
  return (
    <span
      ref={badge}
      className={`badge flag flag-${props.flag}`}
      tabIndex={0}
      aria-describedby={id}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={() => setOpen(false)}
    >
      {flagLabel[props.flag]}
      <span
        id={id}
        ref={tip}
        role="tooltip"
        className="tooltip"
        hidden={!open}
        style={pos ? { left: pos.left, top: pos.top } : { left: 0, top: 0, visibility: open ? "hidden" : undefined }}
      >
        {flagHelp[props.flag]}
      </span>
    </span>
  );
}

export function StateBadge(props: { state: State | null }) {
  if (!props.state) return null;
  return <span className={`badge state state-${props.state}`}>{stateLabel[props.state]}</span>;
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

// AgreementIcon は、エビデンスの本体のファイルの合意の状態のアイコン（API の found の state）。
// 揃っているかのアイコン（丸のないチェック・バツ・横棒）と見間違えないよう、丸の中に描く。合意済は青の塗りの丸に白のチェック、
// 提案中は橙の枠の丸に時計の針（まだ決まっていない、待っている）
export function AgreementIcon(props: { state: State | null | undefined }) {
  if (!props.state) return null;
  const agreed = props.state === "agreed";
  const label = agreed ? "合意済" : "提案中";
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

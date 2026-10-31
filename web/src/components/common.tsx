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

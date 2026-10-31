import { useEffect, useId, useState } from "react";
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
export function FlagBadge(props: { flag: Flag }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <span
      className={`badge flag flag-${props.flag}`}
      tabIndex={0}
      aria-describedby={id}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={() => setOpen(false)}
    >
      {flagLabel[props.flag]}
      <span id={id} role="tooltip" className="tooltip" hidden={!open}>
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

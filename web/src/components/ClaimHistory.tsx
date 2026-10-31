import { Fragment, useState } from "react";
import { api, type ClaimDetail } from "../api";
import { stageLabel, typeLabel } from "../labels";
import { ErrorBox, shortCommit, useLoad } from "./common";

// ClaimHistory は、約束の由来（CL-04）と合意の履歴（CL-05）。約束の詳細の画面と、レビューの画面の 1 件の詳細（RV-16）で使う。
export function ClaimHistory(props: { claim: ClaimDetail; level: "h2" | "h3" }) {
  const c = props.claim;
  const H = props.level;
  // 選んだ時点（由来のコミットか、合意の記録を書いたコミット）。押した行のすぐ下に、表の行を開く形で中身を出す（CL-08）。
  // 表の下にまとめて出すと、押した行から離れて画面の外に出ることがあり、押しても何も起きないように見えたため
  // 開いている行の鍵（"origin:<コミット>" か "agreement:<番号>"）。見比べられるよう、いくつでも同時に開ける
  const [open, setOpen] = useState<Set<string>>(new Set());
  const isOpen = (key: string) => open.has(key);
  const toggle = (key: string) =>
    setOpen((cur) => {
      const next = new Set(cur);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  // 行のどこを押しても開く・たたむ。文字をドラッグで選んだ後に離したとき（コピーしようとしたとき）は、開け閉めしない
  const rowToggle = (key: string) => () => {
    const sel = window.getSelection();
    if (sel && !sel.isCollapsed && sel.toString() !== "") return;
    toggle(key);
  };
  return (
    <>
      <H>由来</H>
      {c.origin.length === 0 ? (
        <p className="note">まだコミットしていません。</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>コミット</th>
              <th>日時</th>
              <th>ストーリー</th>
              <th>何をしたか</th>
              <th>メッセージ</th>
              <th>その時点の中身</th>
            </tr>
          </thead>
          <tbody>
            {c.origin.map((o) => (
              <Fragment key={o.commit}>
              <tr className="at-toggle-row" onClick={rowToggle(`origin:${o.commit}`)}>
                <td>{shortCommit(o.commit)}</td>
                <td>{o.date}</td>
                <td>{o.stories.join("・") || "（なし）"}</td>
                <td>{o.change === "added" ? "作成" : "変更"}</td>
                <td>{o.subject}</td>
                <td>
                  <ExpandButton open={isOpen(`origin:${o.commit}`)} onClick={() => toggle(`origin:${o.commit}`)} />
                </td>
              </tr>
              {isOpen(`origin:${o.commit}`) && <ClaimAtRow cols={6} claim={c} commit={o.commit} />}
              </Fragment>
            ))}
          </tbody>
        </table>
      )}

      <H>合意の履歴</H>
      {c.agreements.length === 0 ? (
        <p className="note">まだ合意していません。</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>誰が</th>
              <th>いつ</th>
              <th>ストーリー</th>
              <th>今の中身に</th>
              <th>合意を書いた時点の中身</th>
            </tr>
          </thead>
          <tbody>
            {c.agreements.map((a, i) => (
              <Fragment key={i}>
              <tr className={a.commit ? "at-toggle-row" : undefined} onClick={a.commit ? rowToggle(`agreement:${i}`) : undefined}>
                <td>
                  {a.user.name} &lt;{a.user.email}&gt;
                </td>
                <td>{a.agreedAt}</td>
                <td>{a.story}</td>
                <td>{a.current ? "合います" : "合いません（前の中身への合意です）"}</td>
                <td>
                  {a.commit ? (
                    <ExpandButton open={isOpen(`agreement:${i}`)} onClick={() => toggle(`agreement:${i}`)} />
                  ) : (
                    <span className="note">まだコミットしていません</span>
                  )}
                </td>
              </tr>
              {a.commit && isOpen(`agreement:${i}`) && <ClaimAtRow cols={5} claim={c} commit={a.commit} />}
              </Fragment>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

// ExpandButton は、その時点の中身の行を開く・たたむアイコンのボタン（閉じているときは右向き、開いているときは下向きの山形）。
// 依存を足さないよう、アイコンはインラインの SVG で書く
function ExpandButton(props: { open: boolean; onClick: () => void }) {
  const label = props.open ? "その時点の中身をたたむ" : "その時点の中身を開く";
  return (
    <button
      type="button"
      className="at-button"
      aria-label={label}
      title={label}
      aria-expanded={props.open}
      onClick={(e) => {
        // 行にも同じ動きがあるので、行へ伝えず、二重に切り替わらないようにする
        e.stopPropagation();
        props.onClick();
      }}
    >
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
        <path d={props.open ? "M4 6l4 4 4-4" : "M6 4l4 4-4 4"} fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </button>
  );
}

function evidenceText(evs: { stage: string; type: string; layers: string[]; detail: string | null }[]): string {
  return evs.map((ev) => `${stageLabel[ev.stage] ?? ev.stage} / ${typeLabel[ev.type] ?? ev.type} / ${ev.layers.join("・")}${ev.detail ? ` — ${ev.detail}` : ""}`).join("\n");
}

// ClaimAtRow は、ある時点の約束の中身（文・理由・条件・レイヤー・エビデンスの予定）を、押した行のすぐ下の行に出す。中身は API が、その時点の約束リストから読んだもの。
function ClaimAtRow(props: { cols: number; claim: ClaimDetail; commit: string }) {
  const c = props.claim;
  const d = useLoad(() => api.claimAt(c.id, props.commit), `${c.id}@${props.commit}`);
  const none = "（なし）";
  const rows: [string, string][] = d.data
    ? [
        ["文", d.data.content.statement],
        ["理由", d.data.content.rationale ?? none],
        ["条件", d.data.content.condition ?? none],
        ["レイヤー", (d.data.layers ?? d.data.content.layers.map((id) => ({ id, name: "" }))).map((l) => l.name || l.id).join("・")],
        ["エビデンスの予定", evidenceText(d.data.content.evidence)],
      ]
    : [];
  return (
    <tr className="claim-at-row">
      <td colSpan={props.cols}>
        <section className="claim-at" aria-label="その時点の約束の中身">
          <div className="claim-at-head">
            <strong>コミット {shortCommit(props.commit)} の時点の中身</strong>
            {d.data && (
              <span className="note">
                {d.data.date} · {d.data.subject}
              </span>
            )}
          </div>
          {d.loading && <p className="note">読み込んでいます…</p>}
          {d.error != null && <ErrorBox error={d.error} title="その時点の中身を読めません" />}
          {d.data && (
            <dl className="fields">
              {rows.map(([label, then]) => (
                <Fragment key={label}>
                  <dt>{label}</dt>
                  <dd className="text">{label === "エビデンスの予定" ? <ul>{then.split("\n").filter(Boolean).map((l, i) => <li key={i}>{l}</li>)}</ul> : then}</dd>
                </Fragment>
              ))}
            </dl>
          )}
        </section>
      </td>
    </tr>
  );
}

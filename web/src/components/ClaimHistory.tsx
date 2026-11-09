import { Fragment, type ReactNode, useState } from "react";
import { api, type ClaimHistory as History } from "../api";
import { ErrorBox, Loading, shortCommit, useLoad } from "./common";
import { LayerChips } from "./Chips";
import { EvidenceGroups } from "./EvidenceGroups";
import { Prose, ProseLines, RefsProvider } from "./Prose";

// ClaimHistory は、約束の変更履歴（CL-04）と承認の履歴（CL-05）。約束の詳細の画面と、レビューの画面の 1 件の詳細（RV-16）で使う。
// 変更履歴と承認の履歴は、約束の詳細と同時に読み始めるため、読み込みは使う側が持つ（ここで読むと、詳細を読み終えるまで読み始められない）
export function ClaimHistory(props: { id: string; history: { data: History | null; error: unknown }; level: "h2" | "h3" }) {
  const id = props.id;
  const c = props.history.data;
  const error = props.history.error;
  const H = props.level;
  // その時点の中身のステージの見出しは、変更履歴・承認の履歴の見出しの 1 つ下の段にする
  const sub = props.level === "h2" ? "h3" : "h4";
  // 選んだ時点（変更履歴のコミットか、承認の記録を書いたコミット）。押した行のすぐ下に、表の行を開く形で中身を出す（CL-08）。
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
      <H>変更履歴</H>
      {error != null ? (
        <ErrorBox error={error} title="変更履歴を読めません" />
      ) : !c ? (
        <Loading />
      ) : c.origin.length === 0 ? (
        <p className="note">まだコミットしていません。</p>
      ) : (
        <table className="history">
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
                <td className="user-text">{o.subject}</td>
                <td>
                  <ExpandButton open={isOpen(`origin:${o.commit}`)} onClick={() => toggle(`origin:${o.commit}`)} />
                </td>
              </tr>
              {isOpen(`origin:${o.commit}`) && <ClaimAtRow cols={6} id={id} commit={o.commit} level={sub} />}
              </Fragment>
            ))}
          </tbody>
        </table>
      )}

      <H>承認の履歴</H>
      {error != null ? (
        <ErrorBox error={error} title="承認の履歴を読めません" />
      ) : !c ? (
        <Loading />
      ) : c.agreements.length === 0 ? (
        <p className="note">まだ承認していません。</p>
      ) : (
        <table className="history">
          <thead>
            <tr>
              <th>誰が</th>
              <th>いつ</th>
              <th>ストーリー</th>
              <th>今の中身に</th>
              <th>承認を書いた時点の中身</th>
            </tr>
          </thead>
          <tbody>
            {c.agreements.map((a, i) => (
              <Fragment key={i}>
              <tr className={a.commit ? "at-toggle-row" : undefined} onClick={a.commit ? rowToggle(`agreement:${i}`) : undefined}>
                <td>
                  <span className="user-text">{a.user.name}</span> &lt;{a.user.email}&gt;
                </td>
                <td>{a.agreedAt}</td>
                <td>{a.story}</td>
                <td>{a.current ? "合います" : "合いません（前の中身への承認です）"}</td>
                <td>
                  {a.commit ? (
                    <ExpandButton open={isOpen(`agreement:${i}`)} onClick={() => toggle(`agreement:${i}`)} />
                  ) : (
                    <span className="note">まだコミットしていません</span>
                  )}
                </td>
              </tr>
              {a.commit && isOpen(`agreement:${i}`) && <ClaimAtRow cols={5} id={id} commit={a.commit} level={sub} />}
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

// ClaimAtRow は、ある時点の約束の中身（変更前と変更後の表（RV-05）と同じ項目と並び）を、押した行のすぐ下の行に出す。中身は API が、その時点の約束リストから読んだもの。
function ClaimAtRow(props: { cols: number; id: string; commit: string; level: "h3" | "h4" }) {
  const d = useLoad(() => api.claimAt(props.id, props.commit), `${props.id}@${props.commit}`);
  const none = "（なし）";
  const rows: [string, ReactNode][] = d.data
    ? [
        ["文", <Prose pieces={d.data.content.statement} />],
        ["条件", d.data.content.condition ? <Prose pieces={d.data.content.condition} /> : none],
        ["理由", d.data.content.rationale ? <Prose pieces={d.data.content.rationale} /> : none],
        ["対象", d.data.content.subjects.join("・")],
        ["レイヤー", <LayerChips layers={d.data.content.layers} names={d.data.layers ?? undefined} />],
        [
          "エビデンスの予定",
          d.data.content.evidence.length === 0 ? (
            none
          ) : (
            <EvidenceGroups
              items={d.data.content.evidence}
              level={props.level}
              render={(ev) => (
                <>
                  <LayerChips layers={ev.layers} names={d.data?.layers ?? undefined} />
                  {ev.detail && (
                    <div className="evidence-detail">
                      <Prose pieces={ev.detail} />
                    </div>
                  )}
                </>
              )}
            />
          ),
        ],
        ["推測", d.data.content.guesses.length > 0 ? <ProseLines list={d.data.content.guesses} /> : none],
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
          {d.loading && <Loading />}
          {d.error != null && <ErrorBox error={d.error} title="その時点の中身を読めません" />}
          {d.data && (
            <RefsProvider refs={d.data.refs}>
              <dl className="fields">
                {rows.map(([label, then]) => (
                  <Fragment key={label}>
                    <dt>{label}</dt>
                    <dd className="text user-text">{then}</dd>
                  </Fragment>
                ))}
              </dl>
            </RefsProvider>
          )}
        </section>
      </td>
    </tr>
  );
}

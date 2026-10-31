import { useState } from "react";
import { api, type ClaimDetail } from "../api";
import { stageLabel, typeLabel } from "../labels";
import { ErrorBox, shortCommit, useLoad } from "./common";

// ClaimHistory は、約束の由来（CL-04）と合意の履歴（CL-05）。約束の詳細の画面と、レビューの画面の 1 件の詳細（RV-16）で使う。
export function ClaimHistory(props: { claim: ClaimDetail; level: "h2" | "h3" }) {
  const c = props.claim;
  const H = props.level;
  // 選んだ時点（由来のコミットか、合意の記録を書いたコミット）。同じ画面の中で、表の下に中身を出す（CL-08）
  const [at, setAt] = useState<{ commit: string; what: string } | null>(null);
  const pick = (commit: string, what: string) => setAt(at?.commit === commit && at.what === what ? null : { commit, what });
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
              <tr key={o.commit}>
                <td>{shortCommit(o.commit)}</td>
                <td>{o.date}</td>
                <td>{o.stories.join("・") || "（なし）"}</td>
                <td>{o.change === "added" ? "作成" : "変更"}</td>
                <td>{o.subject}</td>
                <td>
                  <button type="button" className="at-button" aria-pressed={at?.commit === o.commit && at.what === "origin"} onClick={() => pick(o.commit, "origin")}>
                    見る
                  </button>
                </td>
              </tr>
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
              <tr key={i}>
                <td>
                  {a.user.name} &lt;{a.user.email}&gt;
                </td>
                <td>{a.agreedAt}</td>
                <td>{a.story}</td>
                <td>{a.current ? "合います" : "合いません（前の中身への合意です）"}</td>
                <td>
                  {a.commit ? (
                    <button type="button" className="at-button" aria-pressed={at?.commit === a.commit && at.what === `agreement-${i}`} onClick={() => pick(a.commit as string, `agreement-${i}`)}>
                      見る
                    </button>
                  ) : (
                    <span className="note">まだコミットしていません</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {at && <ClaimAtView id={c.id} commit={at.commit} onClose={() => setAt(null)} />}
    </>
  );
}

// ClaimAtView は、ある時点の約束の中身（文・理由・条件・レイヤー・エビデンスの予定）。中身は API が、その時点の約束リストから読んだもの。
function ClaimAtView(props: { id: string; commit: string; onClose: () => void }) {
  const d = useLoad(() => api.claimAt(props.id, props.commit), `${props.id}@${props.commit}`);
  return (
    <section className="claim-at" aria-label="その時点の約束の中身">
      <div className="claim-at-head">
        <strong>コミット {shortCommit(props.commit)} の時点の中身</strong>
        {d.data && (
          <span className="note">
            {d.data.date} · {d.data.subject}
          </span>
        )}
        <button type="button" onClick={props.onClose}>
          閉じる
        </button>
      </div>
      {d.loading && <p className="note">読み込んでいます…</p>}
      {d.error != null && <ErrorBox error={d.error} title="その時点の中身を読めません" />}
      {d.data && (
        <dl className="fields">
          <dt>文</dt>
          <dd>{d.data.content.statement}</dd>
          <dt>理由</dt>
          <dd>{d.data.content.rationale ?? "（なし）"}</dd>
          <dt>条件</dt>
          <dd>{d.data.content.condition ?? "（なし）"}</dd>
          <dt>レイヤー</dt>
          <dd>{d.data.content.layers.join("・")}</dd>
          <dt>エビデンスの予定</dt>
          <dd>
            <ul>
              {d.data.content.evidence.map((ev, i) => (
                <li key={i}>
                  {stageLabel[ev.stage] ?? ev.stage} / {typeLabel[ev.type] ?? ev.type} / {ev.layers.join("・")}
                  {ev.detail ? ` — ${ev.detail}` : ""}
                </li>
              ))}
            </ul>
          </dd>
        </dl>
      )}
    </section>
  );
}

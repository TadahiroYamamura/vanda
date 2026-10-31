import type { ClaimDetail } from "../api";
import { shortCommit } from "./common";

// ClaimHistory は、約束の由来（CL-04）と合意の履歴（CL-05）。約束の詳細の画面と、レビューの画面の 1 件の詳細（RV-16）で使う。
export function ClaimHistory(props: { claim: ClaimDetail; level: "h2" | "h3" }) {
  const c = props.claim;
  const H = props.level;
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
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

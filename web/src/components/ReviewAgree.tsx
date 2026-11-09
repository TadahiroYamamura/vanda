import { useState } from "react";
import { api, type Context, type Entry } from "../api";
import { AgreeControls, entryKey } from "../pages/ReviewPage";
import { ErrorBox, Loading, StateBadge, useLoad } from "./common";

// ReviewAgree は、約束の詳細とファイルの画面で、レビューの画面から移ったときだけ、承認の状態と承認・取り消しを出す（RV-18）。
// レビューの画面のクエリ（view・base・story）で /api/review を引き、その一覧にそのものがあれば、レビューの画面と同じ部品を出す。
// 承認済か・承認できるかの判定は、一覧（Go の側）のものをそのまま使う
export function ReviewAgree(props: { params?: URLSearchParams; ctx: Context | null; target: { kind: "claim"; id: string } | { kind: "file"; path: string }; onDone: () => void }) {
  const p = props.params;
  const view = p?.get("view") ?? null;
  const base = p?.get("base") ?? (props.ctx?.defaultBase || null);
  const story = p?.get("story") ?? null;
  const [n, setN] = useState(0);
  const review = useLoad(view ? () => api.review(view, base, story) : null, `${view}|${base}|${story}|${n}`);
  if (!view) return <p className="note review-agree">ストーリーのレビューから開くと、承認できます。</p>;
  if (review.error != null) return <ErrorBox error={review.error} title="承認の状態を読めません" />;
  if (!review.data) {
    // 読み込んだ後と同じ形の枠を先に置き、承認のボタンが出ても下の中身を押し下げない
    return (
      <div className="review-agree">
        <p className="note" aria-hidden="true">
          <span className="skeleton skeleton-agree-state" />
        </p>
        <div className="agree">
          <Loading />
        </div>
      </div>
    );
  }
  const key = props.target.kind === "claim" ? `claim:${props.target.id}` : `path:${props.target.path}`;
  const entry: Entry | undefined = [...review.data.entries, ...review.data.detached].find((e) => entryKey(e) === key);
  if (!entry) {
    return <p className="note review-agree">{props.target.kind === "claim" ? "このストーリーでは変わっていない約束です。" : "このストーリーでは変わっていないファイルです。"}</p>;
  }
  return (
    <div className="review-agree">
      <p className="note">
        ストーリー {review.data.story ?? "（未選択）"} のレビュー: <StateBadge state={entry.state} />
      </p>
      <AgreeControls
        entry={entry}
        story={review.data.story}
        base={base}
        user={props.ctx?.user ?? null}
        onDone={() => {
          setN((x) => x + 1);
          props.onDone();
        }}
      />
    </div>
  );
}

import { api, type Context } from "../api";
import { SpecHistory } from "../components/SpecHistory";
import { ErrorBox, Loading, StateBadge, useLoad } from "../components/common";
import { ItemRefs } from "../components/ItemRefs";
import { Prose, RefsProvider } from "../components/Prose";
import { ReviewAgree } from "../components/ReviewAgree";
import { Link, reviewQuery, reviewURL } from "../route";

// PropertyPage は、性質の詳細。
export function PropertyPage(props: { id: string; params?: URLSearchParams; ctx?: Context | null }) {
  const d = useLoad(() => api.property(props.id), props.id);
  // 変更履歴と承認の履歴は時間がかかるので、仕様の詳細と同じく、詳細と別に読む
  const h = useLoad(() => api.propertyHistory(props.id), props.id);
  const query = reviewQuery(props.params);
  return (
    <div className="spec">
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      {d.error != null && <ErrorBox error={d.error} title="性質の詳細を読めません" />}
      {!d.data && d.error == null && <Loading />}
      {d.data && (
        <RefsProvider refs={d.data.refs} query={query}>
          <h1>
            {d.data.id} <StateBadge state={d.data.state} />
          </h1>
          <ReviewAgree
            params={props.params}
            ctx={props.ctx ?? null}
            target={{ kind: "property", id: d.data.id }}
            onDone={() => {
              d.reload();
              h.reload();
            }}
          />
          <p className="statement user-text">
            <Prose pieces={d.data.statement} />
          </p>
          <dl className="fields">
            <dt>理由</dt>
            <dd className="user-text">{d.data.rationale ? <Prose pieces={d.data.rationale} /> : "（なし）"}</dd>
            <dt>refines</dt>
            <dd>
              <ItemRefs items={d.data.refines} query={query} />
            </dd>
            <dt>assumes</dt>
            <dd>
              <ItemRefs items={d.data.assumes} query={query} />
            </dd>
          </dl>
          <SpecHistory id={d.data.id} history={h} level="h2" noAt />
        </RefsProvider>
      )}
    </div>
  );
}

// TruthPage は、前提の詳細。
export function TruthPage(props: { id: string; params?: URLSearchParams; ctx?: Context | null }) {
  const d = useLoad(() => api.truth(props.id), props.id);
  const h = useLoad(() => api.truthHistory(props.id), props.id);
  const query = reviewQuery(props.params);
  return (
    <div className="spec">
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      {d.error != null && <ErrorBox error={d.error} title="前提の詳細を読めません" />}
      {!d.data && d.error == null && <Loading />}
      {d.data && (
        <RefsProvider refs={d.data.refs} query={query}>
          <h1>
            {d.data.id} <StateBadge state={d.data.state} />
          </h1>
          <ReviewAgree
            params={props.params}
            ctx={props.ctx ?? null}
            target={{ kind: "truth", id: d.data.id }}
            onDone={() => {
              d.reload();
              h.reload();
            }}
          />
          <p className="statement user-text">
            <Prose pieces={d.data.statement} />
          </p>
          <SpecHistory id={d.data.id} history={h} level="h2" noAt />
        </RefsProvider>
      )}
    </div>
  );
}

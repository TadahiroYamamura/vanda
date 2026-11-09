import { api, type ClaimSummary, type NamedRef } from "../api";
import { ErrorBox, useLoad } from "../components/common";
import { detailURL, FeatureMarks, Prose, RefsProvider } from "../components/Prose";
import { Link, reviewQuery, reviewURL } from "../route";

// TermPage は、用語の詳細（term-page-content・term-feature-page-claims・term-feature-page-missing）。
export function TermPage(props: { id: string; params?: URLSearchParams }) {
  const d = useLoad(() => api.term(props.id), props.id);
  const query = reviewQuery(props.params);
  return (
    <div className="term-feature">
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      {d.error != null && <ErrorBox error={d.error} title="用語の詳細を読めません" />}
      {!d.data && d.error == null && <p className="note">読み込んでいます…</p>}
      {d.data && (
        <RefsProvider refs={d.data.refs} query={query}>
          <h1 className="user-text">{d.data.name}</h1>
          <dl className="fields">
            <dt>ID</dt>
            <dd>
              <code>{d.data.id}</code>
            </dd>
            <dt>定義</dt>
            <dd className="user-text">
              <Prose pieces={d.data.definition} />
            </dd>
            <dt>親の用語</dt>
            <dd>{d.data.partOf ? <TermLink t={d.data.partOf} query={query} /> : "（なし）"}</dd>
            <dt>子の用語</dt>
            <dd>
              {d.data.children.length === 0 ? (
                "（なし）"
              ) : (
                <ul>
                  {d.data.children.map((c) => (
                    <li key={c.id}>
                      <TermLink t={c} query={query} />
                    </li>
                  ))}
                </ul>
              )}
            </dd>
          </dl>
          <ClaimList claims={d.data.claims} what="用語" query={query} />
        </RefsProvider>
      )}
    </div>
  );
}

// FeaturePage は、機能の詳細（feature-page-content・term-feature-page-claims・term-feature-page-missing）。
export function FeaturePage(props: { id: string; params?: URLSearchParams }) {
  const d = useLoad(() => api.feature(props.id), props.id);
  const query = reviewQuery(props.params);
  return (
    <div className="term-feature">
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      {d.error != null && <ErrorBox error={d.error} title="機能の詳細を読めません" />}
      {!d.data && d.error == null && <p className="note">読み込んでいます…</p>}
      {d.data && (
        <RefsProvider refs={d.data.refs} query={query}>
          <h1>
            <span className="user-text">{d.data.name}</span>
            <span className="marks">
              <FeatureMarks core={d.data.core} formal={d.data.formal} />
            </span>
          </h1>
          <dl className="fields">
            <dt>ID</dt>
            <dd>
              <code>{d.data.id}</code>
            </dd>
            <dt>定義</dt>
            <dd className="user-text">
              <Prose pieces={d.data.definition} />
            </dd>
          </dl>
          <ClaimList claims={d.data.claims} what="機能" query={query} />
        </RefsProvider>
      )}
    </div>
  );
}

function TermLink(props: { t: NamedRef; query: string }) {
  const { t } = props;
  if (t.name === null) return <span className="user-text">{`${t.id}（約束リストにありません）`}</span>;
  return (
    <Link to={detailURL("term", t.id, props.query)}>
      <span className="user-text">{t.name}</span>（<code>{t.id}</code>）
    </Link>
  );
}

function ClaimList(props: { claims: ClaimSummary[]; what: string; query: string }) {
  return (
    <>
      <h2>この{props.what}を対象に持つ約束</h2>
      {props.claims.length === 0 ? (
        <p className="note">この{props.what}を対象に持つ約束はありません。</p>
      ) : (
        <ul className="claim-list">
          {props.claims.map((c) => (
            <li key={c.id}>
              <Link to={detailURL("claim", c.id, props.query)} className="claim-id">
                {c.id}
              </Link>
              <span className="user-text">
                <Prose pieces={c.statement} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

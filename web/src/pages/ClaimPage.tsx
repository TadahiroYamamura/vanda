import { api, type Context, type Found } from "../api";
import { ReviewAgree } from "../components/ReviewAgree";
import { AgreementIcon, ErrorBox, Loading, PresentIcon, StateBadge, useLoad } from "../components/common";
import { LayerChips } from "../components/Chips";
import { EvidenceGroups } from "../components/EvidenceGroups";
import { ClaimHistory } from "../components/ClaimHistory";
import { CodeBlock } from "../components/Code";
import { Prose, RefsProvider } from "../components/Prose";
import { langOf } from "../highlight";
import { Link, reviewQuery, reviewURL } from "../route";

// ClaimPage は、約束の詳細（CL-01・02・04・05・07）。
export function ClaimPage(props: { id: string; params?: URLSearchParams; ctx?: Context | null }) {
  const d = useLoad(() => api.claim(props.id), props.id);
  // 由来と合意の履歴は時間がかかるので、詳細と別に読み、詳細を先に出す（claim-detail-shown-before-history）
  const h = useLoad(() => api.claimHistory(props.id), props.id);
  if (d.error != null) return <ErrorBox error={d.error} title="約束の詳細を読めません" />;
  if (!d.data) {
    return (
      <div className="claim">
        <p className="note">
          <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
        </p>
        <Loading />
      </div>
    );
  }
  const c = d.data;
  return (
    <RefsProvider refs={c.refs} query={reviewQuery(props.params)}>
    <div className="claim">
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      <h1>
        {c.id} <StateBadge state={c.state} />
      </h1>
      <ReviewAgree params={props.params} ctx={props.ctx ?? null} target={{ kind: "claim", id: c.id }}
        onDone={() => {
          d.reload();
          h.reload();
        }}
      />
      <p className="statement user-text">
        <Prose pieces={c.statement} />
      </p>
      <dl className="fields">
        <dt>条件</dt>
        <dd className="user-text">{c.condition ? <Prose pieces={c.condition} /> : "（なし）"}</dd>
        <dt>理由</dt>
        <dd className="user-text">{c.rationale ? <Prose pieces={c.rationale} /> : "（なし）"}</dd>
        <dt>レイヤー</dt>
        <dd>
          <LayerChips layers={c.layers.map((l) => l.id)} names={c.layers} />
        </dd>
        <dt>対象</dt>
        <dd className="user-text">{c.subjects.map((s) => (s.name ? `${s.name}（${s.id}）` : `${s.id}（約束リストにありません）`)).join("・")}</dd>
        {c.guesses.length > 0 && (
          <>
            <dt>推測</dt>
            <dd>
              <ul>
                {c.guesses.map((g, i) => (
                  <li key={i} className="user-text">
                    <Prose pieces={g} />
                  </li>
                ))}
              </ul>
            </dd>
          </>
        )}
      </dl>

      <h2>エビデンス</h2>
      {c.evidence.length === 0 ? (
        <p className="note">エビデンスの予定がありません。</p>
      ) : (
        <EvidenceGroups
          items={c.evidence}
          level="h3"
          render={(ev) => (
            <>
              <div className="evidence-line">
                <PresentIcon present={ev.present} />
                <LayerChips layers={ev.layers} names={c.layers} />
              </div>
              {ev.detail && (
                <div className="evidence-detail">
                  <Prose pieces={ev.detail} />
                </div>
              )}
              {ev.found.map((f) => (
                <FoundView key={`${f.file}:${f.line}`} f={f} params={props.params} />
              ))}
            </>
          )}
        />
      )}

      <ClaimHistory id={c.id} history={h} level="h2" />
    </div>
    </RefsProvider>
  );
}

function FoundView(props: { f: Found; params?: URLSearchParams }) {
  const { f } = props;
  const q = reviewQuery(props.params);
  const fileURL = `/files?${q ? `${q}&` : ""}path=${encodeURIComponent(f.file)}`;
  const fileLink = <Link to={fileURL}>{`${f.file}:${f.line}`}</Link>;
  return (
    <div className="found">
      <p>
        <strong>{f.name}</strong> {fileLink} <AgreementIcon state={f.state} />
        {f.adapter === "tla" && (
          <>
            {" "}
            · <Link to={fileURL}>ファイル全体</Link>
          </>
        )}
      </p>
      {f.text !== null && <CodeBlock source={f.text} lang={langOf(f.file)} />}
    </div>
  );
}

export function FilePage(props: { path: string; params?: URLSearchParams; ctx?: Context | null }) {
  const d = useLoad(() => api.file(props.path), props.path);
  return (
    <div>
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      <h1>{props.path}</h1>
      <ReviewAgree params={props.params} ctx={props.ctx ?? null} target={{ kind: "file", path: props.path }} onDone={d.reload} />
      {d.error != null && <ErrorBox error={d.error} title="ファイルを読めません" />}
      {d.data && <CodeBlock source={d.data.text} lang={langOf(props.path)} />}
    </div>
  );
}

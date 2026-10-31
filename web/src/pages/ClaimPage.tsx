import { api, type EvidenceDetail, type Found } from "../api";
import { ErrorBox, PresentIcon, StateBadge, useLoad } from "../components/common";
import { LayerChips } from "../components/Chips";
import { stageLabel, typeLabel } from "../labels";
import { ClaimHistory } from "../components/ClaimHistory";
import { CodeBlock } from "../components/Code";
import { langOf } from "../highlight";
import { Link, reviewQuery, reviewURL } from "../route";

// ClaimPage は、約束の詳細（CL-01・02・04・05・07）。
export function ClaimPage(props: { id: string; params?: URLSearchParams }) {
  const d = useLoad(() => api.claim(props.id), props.id);
  if (d.error != null) return <ErrorBox error={d.error} title="約束の詳細を読めません" />;
  if (!d.data) return <p className="note">読み込んでいます…</p>;
  const c = d.data;
  // タイプごとにまとめる（CL-02）。並びは、約束リストで最初に出てきた順
  const byType = new Map<string, EvidenceDetail[]>();
  for (const ev of c.evidence) byType.set(ev.type, [...(byType.get(ev.type) ?? []), ev]);
  return (
    <div className="claim">
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      <h1>
        {c.id} <StateBadge state={c.state} />
      </h1>
      <p className="statement">{c.statement}</p>
      <dl className="fields">
        <dt>条件</dt>
        <dd>{c.condition ?? "（なし）"}</dd>
        <dt>理由</dt>
        <dd>{c.rationale ?? "（なし）"}</dd>
        <dt>レイヤー</dt>
        <dd>
          <LayerChips layers={c.layers.map((l) => l.id)} names={c.layers} />
        </dd>
        <dt>対象</dt>
        <dd>{c.subjects.map((s) => (s.name ? `${s.name}（${s.id}）` : `${s.id}（約束リストにありません）`)).join("・")}</dd>
        {c.guesses.length > 0 && (
          <>
            <dt>推測</dt>
            <dd>
              <ul>
                {c.guesses.map((g) => (
                  <li key={g}>{g}</li>
                ))}
              </ul>
            </dd>
          </>
        )}
      </dl>

      <h2>エビデンス（タイプごと）</h2>
      {[...byType.entries()].map(([type, evs]) => (
        <section key={type} className="evidence-type">
          <h3>{typeLabel[type] ?? type}</h3>
          {evs.map((ev, i) => (
            <div key={i} className="evidence">
              <div className="evidence-line">
                <PresentIcon present={ev.present} />
                <span>{stageLabel[ev.stage] ?? ev.stage}</span>
                <LayerChips layers={ev.layers} names={c.layers} />
              </div>
              {ev.detail && <div className="evidence-detail">{ev.detail}</div>}
              {ev.found.map((f) => (
                <FoundView key={`${f.file}:${f.line}`} f={f} params={props.params} />
              ))}
            </div>
          ))}
        </section>
      ))}

      <ClaimHistory claim={c} level="h2" />
    </div>
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
        <strong>{f.name}</strong> {fileLink}
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

export function FilePage(props: { path: string; params?: URLSearchParams }) {
  const d = useLoad(() => api.file(props.path), props.path);
  return (
    <div>
      <p className="note">
        <Link to={reviewURL(props.params)}>ストーリーのレビューへ戻る</Link>
      </p>
      <h1>{props.path}</h1>
      {d.error != null && <ErrorBox error={d.error} title="ファイルを読めません" />}
      {d.data && <CodeBlock source={d.data.text} lang={langOf(props.path)} />}
    </div>
  );
}

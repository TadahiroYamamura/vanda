import { useState } from "react";
// 本文の中の生の HTML は描かない（react-markdown の既定。rehype-raw などを足さない）。Issue の本文に悪意のある HTML があっても、画面で動かないようにするため
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { api, type ClaimContent, type Context, type Entry, type FeatureContent, type FileContent, type Review, type TermContent } from "../api";
import { ErrorBox, FlagBadge, StateBadge, shortCommit, useLoad } from "../components/common";
import { ClaimHistory } from "../components/ClaimHistory";
import { CodeBlock } from "../components/Code";
import { LineDiff, Parts } from "../components/DiffView";
import { langOf } from "../highlight";
import { TlaCode } from "../components/TlaCode";
import { changeLabel, changeMark, fileTypeLabel, kindLabel, stageLabel, typeLabel } from "../labels";
import { Link, navigate, reviewQuery } from "../route";

// まとまりの件数がこれを超えたら、開いたときにたたんでおく（RV-10）
const FOLD_OVER = 10;

export function entryKey(e: Entry) {
  return e.kind === "file" ? `path:${e.path}` : `${e.kind}:${e.id}`;
}

function entryName(e: Entry) {
  return e.kind === "file" ? (e.path ?? "") : (e.id ?? "");
}

export function ReviewPage(props: { ctx: Context; params: URLSearchParams; onAgreed: () => void }) {
  const { ctx, params } = props;
  const view = params.get("view") === "story" ? "story" : "branch";
  const base = params.get("base") ?? (ctx.defaultBase || null);
  const storyParam = params.get("story");
  const sel = params.get("sel");

  const set = (changes: Record<string, string | null>, replace = false) => {
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(changes)) {
      if (v === null || v === "") p.delete(k);
      else p.set(k, v);
    }
    navigate(`/?${p.toString()}`, replace);
  };

  const canLoad = view === "branch" ? !!base : !!storyParam;
  const review = useLoad<Review>(canLoad ? () => api.review(view, base, storyParam) : null, `${view}|${base}|${storyParam}`);
  const story = review.data?.story ?? storyParam;
  const detected = view === "branch" && !storyParam ? (review.data?.story ?? null) : null;
  // 選択肢の中は短い名前にし、何と何を比べているかの説明は、触れたときに出す（欄に収まらないため）
  const branchHelp = `ブランチ（${base ?? "ベースブランチ"} と分かれた点からの、このブランチの変更。コミットしていない変更も含む）`;
  const storyHelp = story ? `ストーリー（${story} のコミットすべての変更。ほかのブランチや合流済みのものも含む）` : "ストーリー（そのストーリーのコミットすべての変更。ほかのブランチや合流済みのものも含む）";
  const viewHelp = view === "branch" ? branchHelp : storyHelp;

  const all = review.data ? [...review.data.detached, ...review.data.entries] : [];
  const selected = all.find((e) => entryKey(e) === sel) ?? null;

  return (
    <div className="review">
      <section className="controls" aria-label="比べるもの">
        <label>
          レビュー単位
          <select value={view} onChange={(e) => set({ view: e.target.value, sel: null })} title={viewHelp}>
            <option value="branch" title={branchHelp}>
              ブランチ
            </option>
            <option value="story" title={storyHelp}>
              ストーリー
            </option>
          </select>
        </label>
        {view === "branch" && (
          <label>
            ベースブランチ
            <select value={base ?? ""} onChange={(e) => set({ base: e.target.value, sel: null })}>
              {!base && <option value="">（選んでください）</option>}
              {base && !ctx.bases.includes(base) && <option value={base}>{base}</option>}
              {ctx.bases.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </label>
        )}
        <label>
          ストーリー
          <select value={storyParam ?? ""} onChange={(e) => set({ story: e.target.value, sel: null })}>
            <option value="" title={view === "branch" && detected ? "このブランチのコミットのメッセージから見つけたもの" : undefined}>
              {view === "branch" ? (detected ? `${detected}（このブランチのコミットから）` : "（見つかりません）") : "（選んでください）"}
            </option>
            {storyParam && !ctx.stories.some((s) => s.id === storyParam) && <option value={storyParam}>{storyParam}</option>}
            {ctx.stories.map((s) => (
              <option key={s.id} value={s.id}>
                {s.id}
                {s.inBranch ? "（このブランチ）" : ""}
              </option>
            ))}
          </select>
        </label>
        <p className="comparing" aria-live="polite">
          {view === "branch"
            ? base
              ? `レビュー単位はブランチです（${base} と分かれた点${review.data?.mergeBase ? ` ${shortCommit(review.data.mergeBase)}` : ""} からの、このブランチの変更。コミットしていない変更も含む）`
              : "ベースブランチを選んでください"
            : storyParam
              ? `レビュー単位はストーリーです（${storyParam} のコミットすべての変更。ほかのブランチや合流済みのものも含む）`
              : "ストーリーを選んでください"}
        </p>
      </section>

      <div className="columns">
        <aside className="story-panel">{story ? <StoryPanel id={story} configured={ctx.storyStore !== ""} /> : <p className="note">ストーリーが決まっていません。ストーリーを選んでください。</p>}</aside>

        <section className="list" aria-label="変わったものの一覧">
          {review.loading && <p className="note">読み込んでいます…</p>}
          {review.error != null && <ErrorBox error={review.error} title="一覧を作れません" />}
          {review.data && <EntryList review={review.data} ctx={ctx} selected={sel} onSelect={(k) => set({ sel: k }, true)} />}
        </section>

        <section className="detail" aria-label="1 件の中身">
          {selected && review.data ? (
            <EntryDetail
              key={entryKey(selected)}
              entry={selected}
              total={review.data.total}
              story={story}
              base={base}
              ctx={ctx}
              query={reviewQuery(params)}
              onAgreed={() => {
                review.reload();
                props.onAgreed();
              }}
            />
          ) : (
            <p className="note">一覧から 1 件を選んでください。</p>
          )}
        </section>
      </div>
    </div>
  );
}

function StoryPanel(props: { id: string; configured: boolean }) {
  const s = useLoad(props.configured ? () => api.story(props.id) : null, props.id);
  return (
    <div>
      <h2>ストーリー {props.id}</h2>
      {!props.configured && (
        <p className="note" title="ストーリーの置き場所が設定されていないため、題と本文を表示しません。.vanda/config.toml の [story] に書くと表示します。">
          題と本文なし（置き場所が未設定）
        </p>
      )}
      {s.loading && <p className="note">読み込んでいます…</p>}
      {s.error != null && <ErrorBox error={s.error} title="ストーリーの置き場所から読めません" />}
      {s.data && (
        <>
          <p className="story-title">{s.data.title}</p>
          {s.data.url && (
            <p>
              <a href={s.data.url} target="_blank" rel="noopener noreferrer">
                置き場所で開く
              </a>
            </p>
          )}
          <details open>
            <summary>本文</summary>
            <div className="story-body">
              <Markdown
                remarkPlugins={[remarkGfm]}
                components={{
                  a: ({ href, children }) => (
                    <a href={href} target="_blank" rel="noopener noreferrer">
                      {children}
                    </a>
                  ),
                }}
              >
                {s.data.body}
              </Markdown>
            </div>
          </details>
          <details open>
            <summary>添付ファイル {s.data.attachments.length} 件</summary>
            <ul>
              {s.data.attachments.map((a) => (
                <li key={a.url}>
                  <a href={a.url} target="_blank" rel="noopener noreferrer">
                    {a.name}
                  </a>
                </li>
              ))}
            </ul>
          </details>
        </>
      )}
    </div>
  );
}

function groupLabel(group: string, ctx: Context) {
  if (group === "terms-features") return "用語と機能";
  if (group === "files") return "エビデンスの本体";
  const id = group.replace(/^layer:/, "");
  const layer = ctx.layers.find((l) => l.id === id);
  return `約束（レイヤー ${layer ? layer.name : id || "なし"}）`;
}

function EntryList(props: { review: Review; ctx: Context; selected: string | null; onSelect: (key: string) => void }) {
  const { review } = props;
  const groups: { name: string; entries: Entry[] }[] = [];
  for (const e of review.entries) {
    const last = groups[groups.length - 1];
    if (last && last.name === e.group) last.entries.push(e);
    else groups.push({ name: e.group, entries: [e] });
  }
  const count = (c: string) => review.entries.filter((e) => e.change === c).length;
  return (
    <div>
      <h2>
        変わったもの <span className="total">全 {review.total} 件</span>
      </h2>
      <p className="note">
        追加 {count("added")}・変更 {count("modified")}・削除 {count("deleted")}・名前の変更 {count("renamed")}
      </p>
      {review.detached.length > 0 && (
        <div className="detached">
          <h3>合意が外れたもの（{review.detached.length} 件。比べる元から変わっていないため、総数に数えません）</h3>
          <ul className="entries">
            {review.detached.map((e) => (
              <EntryRow key={entryKey(e)} e={e} selected={props.selected === entryKey(e)} onSelect={props.onSelect} />
            ))}
          </ul>
        </div>
      )}
      {review.total === 0 && <p className="note">比べる元と今のあいだで、変わったものはありません。</p>}
      {groups.map((g) => (
        <Group key={g.name} label={groupLabel(g.name, props.ctx)} entries={g.entries} selected={props.selected} onSelect={props.onSelect} />
      ))}
    </div>
  );
}

function Group(props: { label: string; entries: Entry[]; selected: string | null; onSelect: (key: string) => void }) {
  const hasSelected = props.entries.some((e) => entryKey(e) === props.selected);
  const [open, setOpen] = useState(props.entries.length <= FOLD_OVER || hasSelected);
  return (
    <div className="group">
      <h3>
        <button type="button" className="fold" aria-expanded={open} onClick={() => setOpen(!open)}>
          {open ? "▾" : "▸"} {props.label} {props.entries.length} 件{!open && `（たたんだ ${props.entries.length} 件）`}
        </button>
      </h3>
      {open && (
        <ul className="entries">
          {props.entries.map((e) => (
            <EntryRow key={entryKey(e)} e={e} selected={props.selected === entryKey(e)} onSelect={props.onSelect} />
          ))}
        </ul>
      )}
    </div>
  );
}

function EntryRow(props: { e: Entry; selected: boolean; onSelect: (key: string) => void }) {
  const { e } = props;
  return (
    <li className={props.selected ? "entry selected" : "entry"}>
      <button type="button" className="entry-button" onClick={() => props.onSelect(entryKey(e))} aria-current={props.selected}>
        <span className="no">{e.no ?? "—"}</span>
        <span className={`change change-${e.change}`} title={changeLabel[e.change]}>
          {changeMark[e.change]}
        </span>
        <span className="kind">{e.kind === "file" && e.fileType ? fileTypeLabel[e.fileType] : kindLabel[e.kind]}</span>
        <span className="name">{entryName(e)}</span>
      </button>
      <StateBadge state={e.state} />
      {e.flags.map((f) => (
        <FlagBadge key={f} flag={f} />
      ))}
      {e.detachedAgreement && <span className="badge detached-badge">合意が外れた</span>}
    </li>
  );
}

function EntryDetail(props: { entry: Entry; total: number; story: string | null; base: string | null; ctx: Context; query: string; onAgreed: () => void }) {
  const { entry: e } = props;
  const [agreeing, setAgreeing] = useState(false);
  const [agreeError, setAgreeError] = useState<unknown>(null);
  const [done, setDone] = useState<string | null>(null);
  // 合意・取り消しのたびに増やし、由来と合意の履歴（RV-16）を読み直す
  const [written, setWritten] = useState(0);

  const revoke = async () => {
    if (!props.story) return;
    setAgreeing(true);
    setAgreeError(null);
    try {
      const r = await api.revoke(e, props.story, props.base);
      setDone(
        r.stillAgreed
          ? `合意を取り消しました（コミット ${shortCommit(r.commit)}）。ほかの合意が残っているため、合意済みのままです。`
          : `合意を取り消しました（コミット ${shortCommit(r.commit)}）。`,
      );
      setWritten((n) => n + 1);
      props.onAgreed();
    } catch (err) {
      setAgreeError(err);
    } finally {
      setAgreeing(false);
    }
  };

  const agree = async () => {
    if (!props.story) return;
    setAgreeing(true);
    setAgreeError(null);
    try {
      const r = await api.agree(e, props.story);
      setDone(`合意しました（コミット ${shortCommit(r.commit)}）`);
      setWritten((n) => n + 1);
      props.onAgreed();
    } catch (err) {
      setAgreeError(err);
    } finally {
      setAgreeing(false);
    }
  };

  const cannot = !props.story
    ? "ストーリーを選ぶと、合意できます。"
    : !props.ctx.user
      ? "git の user.name と user.email を設定すると、合意できます。"
      : null;

  return (
    <div>
      <p className="note">
        {e.no !== null ? `${e.no} / ${props.total}` : "総数に数えません"} · {e.kind === "file" && e.fileType ? fileTypeLabel[e.fileType] : kindLabel[e.kind]} · {changeLabel[e.change]}
      </p>
      <h2 className="entry-title">
        {entryName(e)} <StateBadge state={e.state} />
        {e.flags.map((f) => (
          <FlagBadge key={f} flag={f} />
        ))}
      </h2>
      {e.oldPath && <p className="note">前の場所: {e.oldPath}</p>}
      {e.detachedAgreement && (
        <p className="warn">
          ストーリー {props.story} の合意が外れています。
          {e.detachedAgreement.by.length > 0 && `（合意の後に変えたコミットのストーリー: ${e.detachedAgreement.by.join("・")}）`}
        </p>
      )}
      {e.otherCommits.length > 0 && (
        <div className="warn">
          ほかのコミットが、途中で同じものを変えています:
          <ul>
            {e.otherCommits.map((c) => (
              <li key={c.commit}>
                {shortCommit(c.commit)} {c.subject} {c.stories.length > 0 ? `（${c.stories.join("・")}）` : "（ストーリーの ID なし）"}
              </li>
            ))}
          </ul>
        </div>
      )}
      {e.kind === "claim" && e.id && e.change !== "deleted" && (
        <p>
          <Link to={`/claims/${encodeURIComponent(e.id)}${props.query ? `?${props.query}` : ""}`}>約束の詳細を開く</Link>
        </p>
      )}

      <div className="agree">
        {e.state === "proposed" && (
          <>
            <button type="button" className="primary" disabled={agreeing || cannot !== null} onClick={agree}>
              {agreeing ? "合意しています…" : props.story ? `ストーリー ${props.story} で合意する` : "合意する"}
            </button>
            {cannot && <span className="note">{cannot}</span>}
          </>
        )}
        {e.state === "agreed" && <span className="note">今の中身に合意済みです。</span>}
        {e.state === "agreed" && e.revoke?.possible && (
          <>
            <button type="button" className="revoke" disabled={agreeing} onClick={revoke}>
              {agreeing ? "取り消しています…" : "合意を取り消す"}
            </button>
            {e.revoke.others.length > 0 && (
              <span className="note">取り消しても、ほかの人（{e.revoke.others.map((u) => u.name).join("・")}）の合意が残るため、合意済みのままです。</span>
            )}
          </>
        )}
        {e.state === "agreed" && e.revoke && !e.revoke.possible && e.revoke.reason && <span className="note revoke-reason">{e.revoke.reason}</span>}
        {e.state === null && <span className="note">消したものには合意できません。</span>}
        {done && <span className="note">{done}</span>}
        {agreeError != null && <ErrorBox error={agreeError} title="合意できません" />}
      </div>

      <Compare entry={e} />
      {e.kind === "claim" && e.id && e.change !== "deleted" && <ClaimSections key={written} id={e.id} query={props.query} />}
    </div>
  );
}

// Row は、表の 1 行。field は、差分（diff.fields）の項目の名前
type Row = [string, string, string?];

function rows(kind: Entry["kind"], c: Entry["before"]): Row[] | null {
  if (!c) return null;
  const opt = (s: string | null) => s ?? "（なし）";
  switch (kind) {
    case "claim": {
      const x = c as ClaimContent;
      return [
        ["文", x.statement, "statement"],
        ["条件", opt(x.condition), "condition"],
        ["理由", opt(x.rationale), "rationale"],
        ["対象", x.subjects.join("・")],
        ["レイヤー", x.layers.join("・")],
        ["エビデンスの予定", x.evidence.map((ev) => `${stageLabel[ev.stage] ?? ev.stage} / ${typeLabel[ev.type] ?? ev.type} / ${ev.layers.join("・")}${ev.detail ? ` — ${ev.detail}` : ""}`).join("\n")],
        ["推測", x.guesses.length > 0 ? x.guesses.join("\n") : "（なし）"],
      ];
    }
    case "term": {
      const x = c as TermContent;
      return [
        ["名前", x.name, "name"],
        ["定義", x.definition, "definition"],
        ["〜の一部", opt(x.partOf)],
      ];
    }
    case "feature": {
      const x = c as FeatureContent;
      const b = (v: boolean | null) => (v === null ? "（書いていません）" : v ? "はい" : "いいえ");
      return [
        ["名前", x.name, "name"],
        ["定義", x.definition, "definition"],
        ["コア機能", b(x.core)],
        ["形式手法を使う", b(x.formal)],
        ["形式手法を使わない理由", opt(x.formalWaiver), "formalWaiver"],
      ];
    }
    case "file":
      return [["中身", (c as FileContent).text]];
  }
}

function Compare(props: { entry: Entry }) {
  const { entry: e } = props;
  if (e.diff && "lines" in e.diff) {
    return (
      <div className="compare-file">
        <h3>変更前と変更後の差分</h3>
        <LineDiff lines={e.diff.lines} lang={langOf(e.path ?? "")} />
      </div>
    );
  }
  const fields = e.diff && "fields" in e.diff ? e.diff.fields : null;
  const before = rows(e.kind, e.before);
  const after = rows(e.kind, e.after);
  const isTla = e.fileType === "oracle";
  const cell = (r: Row | undefined, side: "before" | "after") => {
    if (!r) return <td className="empty">—</td>;
    if (e.kind === "file") return <td>{isTla ? <TlaCode source={r[1]} /> : <CodeBlock source={r[1]} lang={langOf(e.path ?? "")} />}</td>;
    const parts = r[2] && fields ? fields[r[2]] : undefined;
    return <td className="text">{parts ? <Parts parts={parts} side={side} /> : r[1]}</td>;
  };
  const labels = (after ?? before ?? []).map((r) => r[0]);
  return (
    <table className="compare">
      <thead>
        <tr>
          <th></th>
          {before && <th>変更前</th>}
          {after && <th>{before ? "変更後" : "今"}</th>}
        </tr>
      </thead>
      <tbody>
        {labels.map((label, i) => {
          const b = before?.[i];
          const a = after?.[i];
          return (
            <tr key={label} className={before && after && b?.[1] !== a?.[1] ? "differs" : undefined}>
              <th scope="row">{label}</th>
              {before && cell(b, "before")}
              {after && cell(a, "after")}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// ClaimSections は、約束のエビデンスの予定と実際の表（RV-08。合格したかは段 3 で足す）と、由来と合意の履歴（RV-16）。
export function ClaimSections(props: { id: string; query?: string }) {
  const d = useLoad(() => api.claim(props.id), props.id);
  return (
    <div>
      <h3>エビデンスの予定と実際</h3>
      {d.error != null && <ErrorBox error={d.error} title="エビデンスを探せません" />}
      {d.data && (
        <table className="plan">
          <thead>
            <tr>
              <th>ステージ</th>
              <th>タイプ</th>
              <th>レイヤー</th>
              <th>予定の中身</th>
              <th>実際のエビデンス</th>
              <th>揃っているか</th>
            </tr>
          </thead>
          <tbody>
            {d.data.evidence.map((ev, i) => (
              <tr key={i}>
                <td>{stageLabel[ev.stage] ?? ev.stage}</td>
                <td>{typeLabel[ev.type] ?? ev.type}</td>
                <td>{ev.layers.join("・")}</td>
                <td>{ev.detail ?? ""}</td>
                <td>
                  {ev.found.map((f) => (
                    <div key={`${f.file}:${f.line}`}>
                      {f.name}{" "}
                      <Link className="note" to={`/files?${props.query ? `${props.query}&` : ""}path=${encodeURIComponent(f.file)}`}>
                        {f.file}:{f.line}
                      </Link>
                    </div>
                  ))}
                </td>
                <td>{ev.present === null ? "アノテーションでは確かめません" : ev.present ? "揃っています" : "ありません"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {d.data && <ClaimHistory claim={d.data} level="h3" />}
    </div>
  );
}

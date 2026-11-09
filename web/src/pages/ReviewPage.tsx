import { useState } from "react";
// 本文の中の生の HTML は描かない（react-markdown の既定。rehype-raw などを足さない）。Issue の本文に悪意のある HTML があっても、画面で動かないようにするため
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { api, type ClaimContent, type Content, type Piece, type Refs, type User, type EvidenceContent, type EvidenceMark, type Context, type Entry, type FeatureContent, type FileContent, type Kind, type Review, type TermContent } from "../api";
import { AgreementIcon, ErrorBox, FlagBadge, ListIcon, Loading, PresentIcon, StateBadge, shortCommit, useLoad } from "../components/common";
import { ClaimHistory } from "../components/ClaimHistory";
import { CodeBlock } from "../components/Code";
import { LineDiff, Parts } from "../components/DiffView";
import { LayerChips, LayerChipsDiff } from "../components/Chips";
import { EvidenceGroups } from "../components/EvidenceGroups";
import { EvidenceList } from "../components/EvidenceList";
import { langOf } from "../highlight";
import { TlaCode } from "../components/TlaCode";
import { Prose, RefsProvider, proseText } from "../components/Prose";
import { changeLabel, fileTypeLabel, kindLabel, stageLabel, typeLabel } from "../labels";
import { BACK, Link, navigate, reviewQuery } from "../route";

// まとまりの件数がこれを超えたら、開いたときにたたんでおく（RV-10）
const FOLD_OVER = 10;

export function entryKey(e: Entry) {
  return e.kind === "file" ? `path:${e.path}` : `${e.kind}:${e.id}`;
}

function entryName(e: Entry) {
  return e.kind === "file" ? (e.path ?? "") : (e.id ?? "");
}

export interface Filter {
  text: string;
  notAgreed: boolean;
}

// 文字の絞り込みで見る所。消したものは今の中身がないので、前の中身で見る。文は、参照を見せる文字に置き換えた後の文にマッチさせる（reference-filter）
function searchable(e: Entry): string[] {
  if (e.kind === "file") return [e.path ?? ""];
  const c = e.after ?? e.before;
  const out = [e.id ?? ""];
  if (c && "statement" in c) out.push(proseText(c.statement));
  if (c && "definition" in c) out.push(c.name, proseText(c.definition));
  return out;
}

// 承認済でないものは、提案中と、承認が失効したもの。承認が失効したものは、ほかのストーリーの承認で承認済のこともあるので、状態とは別に見る。
// 消したものは承認できない（状態がない）ので、どちらにもマッチしない
export function matchesFilter(e: Entry, f: Filter): boolean {
  if (f.notAgreed && e.state !== "proposed" && !e.detachedAgreement) return false;
  if (f.text === "") return true;
  const t = f.text.toLowerCase();
  return searchable(e).some((s) => s.toLowerCase().includes(t));
}

export function ReviewPage(props: { ctx: Context; params: URLSearchParams; onAgreed: () => void }) {
  const { ctx, params } = props;
  const view = params.get("view") === "story" ? "story" : "branch";
  const base = params.get("base") ?? (ctx.defaultBase || null);
  const storyParam = params.get("story");
  const sel = params.get("sel");
  const filter: Filter = { text: params.get("filter") ?? "", notAgreed: params.get("unagreed") === "1" };

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
  // ほかの画面へ引き継ぐクエリ。URL に書いていない既定のもの（レビュー単位、設定のベースブランチ、ブランチのコミットから決まったストーリー）も
  // いつも書く。移った先（約束の詳細・ファイルの画面）が、同じ一覧を引いて承認の部品を出すため（RV-18）
  const carried = (() => {
    const q = new URLSearchParams(params);
    const before = new URLSearchParams(params);
    before.delete(BACK);
    q.set(BACK, reviewQuery(before));
    q.set("view", view);
    if (base) q.set("base", base);
    if (story) q.set("story", story);
    return reviewQuery(q);
  })();
  // 選択肢の中は短い名前にし、何と何を比べているかの説明は、マウスを乗せたときに出す（欄に収まらないため）
  const branchHelp = `ブランチ（${base ?? "ベースブランチ"} とのマージベースから HEAD のコミットまでの、このブランチの変更）`;
  const storyHelp = story ? `ストーリー（HEAD の履歴にある、${story} のコミットすべての変更。マージしたほかのブランチのものも含む）` : "ストーリー（HEAD の履歴にある、そのストーリーのコミットすべての変更。マージしたほかのブランチのものも含む）";
  const viewHelp = view === "branch" ? branchHelp : storyHelp;

  // 一覧を初めて読み込んでいる間。ブランチのコミットから決まるストーリーも、まだ分からない
  const pending = canLoad && review.data === null && review.error == null;
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
        <StoryChoice view={view} storyParam={storyParam} review={review.data} pending={pending} ctx={ctx} onChange={(v) => set({ story: v, sel: null })} />
        <p className="comparing" aria-live="polite">
          {view === "branch"
            ? base
              ? `レビュー単位はブランチです（${base} とのマージベース${review.data?.mergeBase ? ` ${shortCommit(review.data.mergeBase)}` : ""} から HEAD のコミットまでの、このブランチの変更）`
              : "ベースブランチを選んでください"
            : storyParam
              ? `レビュー単位はストーリーです（HEAD の履歴にある、${storyParam} のコミットすべての変更。マージしたほかのブランチのものも含む）`
              : "ストーリーを選んでください"}
        </p>
      </section>

      <div className="columns">
        <aside className="story-panel">
          {story ? (
            <StoryPanel id={story} configured={ctx.storyStore !== ""} />
          ) : pending ? (
            <StoryPlaceholder id={null} configured={ctx.storyStore !== ""} />
          ) : (
            <p className="note">ストーリーが決まっていません。ストーリーを選んでください。</p>
          )}
        </aside>

        <section className="list" aria-label="変わったものの一覧">
          {review.error != null && <ErrorBox error={review.error} title="一覧を作れません" />}
          {review.data ? (
            <EntryList
              review={review.data}
              ctx={ctx}
              selected={sel}
              onSelect={(k) => set({ sel: k }, true)}
              filter={filter}
              // 打つたびに履歴を積むと、ブラウザーの「戻る」が 1 文字ずつ戻るため、置き換える
              onFilterText={(v) => set({ filter: v }, true)}
              onFilterNotAgreed={(v) => set({ unagreed: v ? "1" : null }, true)}
              reloading={review.loading}
            />
          ) : (
            pending && (
              <ListPlaceholder
                filter={filter}
                onFilterText={(v) => set({ filter: v }, true)}
                onFilterNotAgreed={(v) => set({ unagreed: v ? "1" : null }, true)}
              />
            )
          )}
        </section>

        <section className="detail" aria-label="1 件の中身">
          {selected && review.data ? (
            <RefsProvider refs={review.data.refs} query={carried}>
            <EntryDetail
              key={entryKey(selected)}
              entry={selected}
              total={review.data.total}
              story={story}
              base={base}
              ctx={ctx}
              query={carried}
              onAgreed={() => {
                review.reload();
                props.onAgreed();
              }}
            />
            </RefsProvider>
          ) : sel && pending ? (
            <SelectedPreview key={sel} sel={sel} query={carried} />
          ) : (
            <p className="note">一覧から 1 件を選んでください。</p>
          )}
        </section>
      </div>
    </div>
  );
}

// StoryChoice は、承認の記録に書くストーリーを示す・選ぶ所（RV-13）。1 つに決まったか・選ばせるものは、API（branchStories・storyFrom）の判定をそのまま使う。
// レビュー単位がブランチで、ブランチのコミットから 1 つに決まったときは、選ばせずに示すだけにする
function StoryChoice(props: { view: string; storyParam: string | null; review: Review | null; pending: boolean; ctx: Context; onChange: (v: string) => void }) {
  const { storyParam, review, ctx } = props;
  // ブランチのコミットから決まるかは一覧の答えで分かるので、それまでは選ばせない（決まっているのに「選んでください」と出さないため）
  if (props.pending && !storyParam) {
    return (
      <p className="story-fixed">
        ストーリー: <span className="skeleton skeleton-inline" aria-hidden="true" />
      </p>
    );
  }
  if (review?.storyChoice === "fixed" && review.story) {
    return (
      <p className="story-fixed">
        ストーリー: <strong>{review.story}</strong>
      </p>
    );
  }
  const choices = review?.storyChoice === "branch" ? [...(review.branchStories ?? [])] : ctx.stories.map((s) => s.id);
  if (storyParam && !choices.includes(storyParam)) choices.unshift(storyParam);
  return (
    <label>
      ストーリー
      <select value={storyParam ?? ""} onChange={(e) => props.onChange(e.target.value)}>
        {!storyParam && (
          <option value="" disabled>
            （選んでください）
          </option>
        )}
        {choices.map((id) => (
          <option key={id} value={id}>
            {id}
          </option>
        ))}
      </select>
    </label>
  );
}

function StoryPanel(props: { id: string; configured: boolean }) {
  const s = useLoad(props.configured ? () => api.story(props.id) : null, props.id);
  if (props.configured && s.data === null && s.error == null) return <StoryPlaceholder id={props.id} configured />;
  return (
    <div>
      <h2>ストーリー {props.id}</h2>
      {!props.configured && (
        <p className="note" title="ストーリーストアが設定されていないため、題と本文を表示しません。.vanda/config.toml の [story] に書くと表示します。">
          題と本文なし（ストーリーストアが未設定）
        </p>
      )}
      {s.error != null && <ErrorBox error={s.error} title="ストーリーストアから読めません" />}
      {s.data && (
        <>
          <p className="story-title user-text">{s.data.title}</p>
          {s.data.url && (
            <p>
              <a href={s.data.url} target="_blank" rel="noopener noreferrer">
                ストーリーストアで開く
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

// StoryPlaceholder は、ストーリーの題と本文を読み終えるまでのプレースホルダー。本文の欄を、高さの上限まで先に取っておく。
// 本文は数十行あることが多く、読み終えてから欄を伸ばすと、下の一覧と 1 件の中身が一度に押し下げられるため
function StoryPlaceholder(props: { id: string | null; configured: boolean }) {
  if (!props.configured) {
    return (
      <div>
        <h2>ストーリー</h2>
        <Loading />
      </div>
    );
  }
  return (
    <div>
      <h2>ストーリー{props.id ? ` ${props.id}` : ""}</h2>
      <p className="story-title" aria-hidden="true">
        <span className="skeleton skeleton-title" />
      </p>
      <p aria-hidden="true">
        <span className="skeleton skeleton-link" />
      </p>
      <div className="story-placeholder-section">
        <span className="skeleton skeleton-summary" aria-hidden="true" />
        <div className="story-body story-body-placeholder">
          <Loading />
        </div>
      </div>
      <div className="story-placeholder-section" aria-hidden="true">
        <span className="skeleton skeleton-summary" />
      </div>
    </div>
  );
}

function groupLabel(group: string, ctx: Context) {
  if (group === "terms") return "用語";
  if (group === "features") return "機能";
  if (group === "files") return "エビデンスの本体";
  const id = group.replace(/^layer:/, "");
  const layer = ctx.layers.find((l) => l.id === id);
  return `約束（レイヤー ${layer ? layer.name : id || "なし"}）`;
}

function EntryList(props: {
  review: Review;
  ctx: Context;
  selected: string | null;
  onSelect: (key: string) => void;
  filter: Filter;
  onFilterText: (v: string) => void;
  onFilterNotAgreed: (v: boolean) => void;
  reloading: boolean;
}) {
  const { review, filter } = props;
  const filtering = filter.text !== "" || filter.notAgreed;
  const entries = review.entries.filter((e) => matchesFilter(e, filter));
  const detached = review.detached.filter((e) => matchesFilter(e, filter));
  const groups: { name: string; entries: Entry[] }[] = [];
  for (const e of entries) {
    const last = groups[groups.length - 1];
    if (last && last.name === e.group) last.entries.push(e);
    else groups.push({ name: e.group, entries: [e] });
  }
  const count = (c: string) => review.entries.filter((e) => e.change === c).length;
  return (
    <div className={props.reloading ? "entry-list reloading" : "entry-list"}>
      <ListHead
        total={`全 ${review.total} 件${filtering ? `・絞り込んだ後 ${entries.length} 件` : ""}`}
        loading={props.reloading}
        filter={filter}
        onFilterText={props.onFilterText}
        onFilterNotAgreed={props.onFilterNotAgreed}
      />
      <p className="note">
        追加 {count("added")}・変更 {count("modified")}・削除 {count("deleted")}
      </p>
      {detached.length > 0 && (
        <div className="detached">
          <h3>承認が失効したもの（{detached.length} 件。比べる元から変わっていないため、総数に数えません）</h3>
          <ul className="entries">
            {detached.map((e) => (
              <EntryRow key={entryKey(e)} e={e} selected={props.selected === entryKey(e)} onSelect={props.onSelect} />
            ))}
          </ul>
        </div>
      )}
      {review.total === 0 && <p className="note">比べる元と今のあいだで、変わったものはありません。</p>}
      {filtering && (review.total > 0 || review.detached.length > 0) && entries.length === 0 && detached.length === 0 && (
        <p className="note">絞り込みの条件にマッチするものはありません。</p>
      )}
      {groups.map((g) => (
        <Group key={g.name} label={groupLabel(g.name, props.ctx)} entries={g.entries} selected={props.selected} onSelect={props.onSelect} />
      ))}
    </div>
  );
}

// ListHead は、一覧の見出しと絞り込みの欄。読み込み中のマークは、見出しの行に重ねて出す。
// 承認の後の読み直しで、行を差し込んで一覧を 1 行ぶん動かさないため
function ListHead(props: { total: string; loading: boolean; filter: Filter; onFilterText: (v: string) => void; onFilterNotAgreed: (v: boolean) => void }) {
  return (
    <>
      <h2>
        変わったもの <span className="total">{props.total}</span>
      </h2>
      {props.loading && <Loading className="loading-over" />}
      <div className="filter" role="search" aria-label="一覧の絞り込み">
        <label>
          絞り込み
          <input type="search" value={props.filter.text} onChange={(e) => props.onFilterText(e.target.value)} placeholder="ID・名前・文・定義・パス" />
        </label>
        <label>
          <input type="checkbox" checked={props.filter.notAgreed} onChange={(e) => props.onFilterNotAgreed(e.target.checked)} />
          承認済でないものだけ
        </label>
      </div>
    </>
  );
}

// 読み込み中に並べるスケルトン（灰色の帯）の数。読み込んだ後の一覧（多くは欄の高さの上限まで伸びる）と、欄の大きさをそろえるため
const PLACEHOLDER_ROWS = 16;

// ListPlaceholder は、一覧を初めて読み込む間のプレースホルダー。読み込んだ後と同じ見出し・絞り込みの欄と、件の行の大きさのスケルトンを先に置く
function ListPlaceholder(props: { filter: Filter; onFilterText: (v: string) => void; onFilterNotAgreed: (v: boolean) => void }) {
  return (
    <div className="entry-list">
      <ListHead total="" loading filter={props.filter} onFilterText={props.onFilterText} onFilterNotAgreed={props.onFilterNotAgreed} />
      <p className="note" aria-hidden="true">
        <span className="skeleton skeleton-count" />
      </p>
      <div aria-hidden="true">
        <div className="skeleton-group">
          <span className="skeleton skeleton-fold" />
        </div>
        {Array.from({ length: PLACEHOLDER_ROWS }, (_, i) => (
          <div key={i} className="skeleton-entry">
            <span className="skeleton skeleton-name" />
            <span className="skeleton skeleton-kind" />
          </div>
        ))}
      </div>
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
    // 1 件の枠（番号・名前・種類・アイコン）のどこをクリックしても選ぶ。キーボードとスクリーンリーダーのために、名前の所は <button> のまま残す
    <li className={props.selected ? "entry selected" : "entry"} onClick={() => props.onSelect(entryKey(e))}>
      <button
        type="button"
        className="entry-button"
        onClick={(ev) => {
          // 枠にも同じ動きがあるので、枠へ伝えず、二度選ばないようにする
          ev.stopPropagation();
          props.onSelect(entryKey(e));
        }}
        aria-current={props.selected}
      >
        <span className="no">{e.no ?? "—"}</span>
        <span className="name">{entryName(e)}</span>
      </button>
      <div className="entry-line2">
        <span className="kind">{e.kind === "file" && e.fileType ? fileTypeLabel[e.fileType] : kindLabel[e.kind]}</span>
        {/* 状態のアイコンは 1 つだけ。失効した承認は、提案中より先に気づくべきなので、失効したほうを出す */}
        {e.detachedAgreement ? <ListIcon name="detached" /> : e.state && <ListIcon name={e.state} />}
        {e.flags.length > 0 && <i className="entry-sep" aria-hidden="true" />}
        {e.flags.map((f) => (
          <ListIcon key={f} name={f} />
        ))}
      </div>
    </li>
  );
}

// previewOmit は、一覧を待つ間の 1 件の中身で出さない項目。詳細の API が返さないか、一覧と違う形で返す
// （機能のコア機能・形式手法を使うは、書いていないときも真偽にして返す）ので、一覧を読み終えてから出す
const previewOmit: Partial<Record<Kind, string[]>> = {
  claim: ["TLA+ で確かめない理由"],
  feature: ["コア機能", "形式手法を使う", "形式手法を使わない理由"],
};

// currentOf は、選んだ 1 件のキー（entryKey の形）から、その今の中身を詳細の API で読む。キーの形に合わなければ null
function currentOf(sel: string): { kind: Kind; name: string; load: () => Promise<{ content: Content; refs: Refs }> } | null {
  const at = sel.indexOf(":");
  if (at < 0) return null;
  const head = sel.slice(0, at);
  const name = sel.slice(at + 1);
  switch (head) {
    case "claim":
      return {
        kind: "claim",
        name,
        load: async () => {
          const c = await api.claim(name);
          const content: ClaimContent = {
            statement: c.statement,
            rationale: c.rationale,
            condition: c.condition,
            subjects: c.subjects.map((x) => x.id),
            layers: c.layers.map((l) => l.id),
            evidence: c.evidence,
            flags: c.flags,
            guesses: c.guesses,
            modelCheckWaiver: null,
          };
          return { content, refs: c.refs };
        },
      };
    case "term":
      return {
        kind: "term",
        name,
        load: async () => {
          const t = await api.term(name);
          const content: TermContent = { name: t.name, definition: t.definition, partOf: t.partOf?.id ?? null };
          return { content, refs: t.refs };
        },
      };
    case "feature":
      return {
        kind: "feature",
        name,
        load: async () => {
          const f = await api.feature(name);
          const content: FeatureContent = { name: f.name, definition: f.definition, core: null, formal: null, formalWaiver: null };
          return { content, refs: f.refs };
        },
      };
    case "path":
      return {
        kind: "file",
        name,
        load: async () => {
          const content: FileContent = { text: (await api.file(name)).text };
          return { content, refs: {} };
        },
      };
  }
  return null;
}

// SelectedPreview は、一覧を読み込む間に、URL で選ばれている 1 件の今の中身を先に出す（review-detail-shown-before-list）。
// 読めなかったとき（今はない 1 件など）は誤りを出さずに一覧を待つ。一覧の側が、読めない理由か、その 1 件の中身を出すため
function SelectedPreview(props: { sel: string; query: string }) {
  const target = currentOf(props.sel);
  const d = useLoad(target ? target.load : null, props.sel);
  if (!target) return <Loading />;
  const omit = previewOmit[target.kind] ?? [];
  return (
    <div>
      <p className="note">{kindLabel[target.kind]}</p>
      <h2 className="entry-title">{target.name}</h2>
      {/* 承認のボタンの欄は一覧を待つ。読み込み中のマークをその場所に置き、一覧が届いてボタンが出ても中身の表を動かさない */}
      <div className="agree">
        <Loading />
      </div>
      {d.data ? (
        <RefsProvider refs={d.data.refs} query={props.query}>
          {target.kind === "file" ? (
            <CodeBlock source={(d.data.content as FileContent).text} lang={langOf(target.name)} />
          ) : (
            <ItemCompare
              entry={{ kind: target.kind, diff: null, before: null, after: d.data.content }}
              before={null}
              after={(rows(target.kind, d.data.content) ?? []).filter((r) => !omit.includes(r[0]))}
            />
          )}
        </RefsProvider>
      ) : (
        <div aria-hidden="true">
          {[0, 1, 2].map((i) => (
            <span key={i} className="skeleton skeleton-line" />
          ))}
        </div>
      )}
    </div>
  );
}

function EntryDetail(props: { entry: Entry; total: number; story: string | null; base: string | null; ctx: Context; query: string; onAgreed: () => void }) {
  const { entry: e } = props;
  // 承認・取り消しのたびに増やし、変更履歴と承認の履歴（RV-16）を読み直す
  const [written, setWritten] = useState(0);

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
      {e.detachedAgreement && (
        <p className="warn">
          ストーリー {props.story} の承認が失効しています。
          {e.detachedAgreement.by.length > 0 && `（承認の後に変えたコミットのストーリー: ${e.detachedAgreement.by.join("・")}）`}
        </p>
      )}
      {e.otherCommits.length > 0 && (
        <div className="warn">
          ほかのコミットが、途中で同じものを変えています:
          <ul>
            {e.otherCommits.map((c) => (
              <li key={c.commit}>
                {shortCommit(c.commit)} <span className="user-text">{c.subject}</span> {c.stories.length > 0 ? `（${c.stories.join("・")}）` : "（ストーリーの ID なし）"}
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
      {e.kind === "file" && e.path && e.change !== "deleted" && (
        <p>
          <Link to={`/files?${props.query ? `${props.query}&` : ""}path=${encodeURIComponent(e.path)}`}>ファイルの画面を開く</Link>
        </p>
      )}

      <AgreeControls
        entry={e}
        story={props.story}
        base={props.base}
        user={props.ctx.user}
        onDone={() => {
          setWritten((n) => n + 1);
          props.onAgreed();
        }}
      />

      <Compare entry={e} />
      {e.kind === "claim" && e.id && e.change !== "deleted" && <ClaimSections key={written} id={e.id} query={props.query} />}
    </div>
  );
}

// AgreeControls は、1 件の承認の状態と、「承認する」「承認を取り消す」のボタン、承認できない・取り消せない理由（RV-06・RV-17）。
// 承認済か・取り消せるかは API（/api/review の state・revoke）の判定をそのまま使う。レビューの画面と、そこから移った約束の詳細・ファイルの画面（RV-18）で使う
export function AgreeControls(props: { entry: Entry; story: string | null; base: string | null; user: User | null; onDone: () => void }) {
  const { entry: e } = props;
  const [agreeing, setAgreeing] = useState(false);
  const [agreeError, setAgreeError] = useState<unknown>(null);
  const [done, setDone] = useState<string | null>(null);

  const run = async (act: () => Promise<string>) => {
    if (!props.story) return;
    setAgreeing(true);
    setAgreeError(null);
    try {
      setDone(await act());
      props.onDone();
    } catch (err) {
      setAgreeError(err);
    } finally {
      setAgreeing(false);
    }
  };
  const deleted = e.change === "deleted";
  const agree = () =>
    run(async () => `${deleted ? "消したことを承認しました" : "承認しました"}（コミット ${shortCommit((await api.agree(e, props.story as string)).commit)}）`);
  const revoke = () =>
    run(async () => {
      const r = await api.revoke(e, props.story as string, props.base);
      return r.stillAgreed
        ? `承認を取り消しました（コミット ${shortCommit(r.commit)}）。ほかの承認が残っているため、承認済みのままです。`
        : `承認を取り消しました（コミット ${shortCommit(r.commit)}）。`;
    });

  // 承認できるかは API（entries[].agree）の判定を使う。承認済でも、このストーリーの承認がなければ承認し直せる（失効した承認を解くため）
  const possible = e.agree?.possible ?? false;
  const agreeButton = (
    <button type="button" className="primary" disabled={agreeing || !possible} onClick={agree}>
      {agreeing ? "承認しています…" : `${props.story ? `ストーリー ${props.story} で` : ""}${deleted ? "消したことを承認する" : "承認する"}`}
    </button>
  );

  return (
    <div className="agree">
      {e.state === "proposed" && (
        <>
          {agreeButton}
          {!possible && e.agree?.reason && <span className="note">{e.agree.reason}</span>}
        </>
      )}
      {e.state === "agreed" && <span className="note">{deleted ? "消したことを承認済みです。" : "今の中身を承認済みです。"}</span>}
      {e.state === "agreed" && possible && agreeButton}
      {e.state === "agreed" && e.revoke?.possible && (
        <>
          <button type="button" className="revoke" disabled={agreeing} onClick={revoke}>
            {agreeing ? "取り消しています…" : "承認を取り消す"}
          </button>
          {e.revoke.others.length > 0 && (
            <span className="note">取り消しても、ほかの人（{e.revoke.others.map((u) => u.name).join("・")}）の承認が残るため、承認済みのままです。</span>
          )}
        </>
      )}
      {e.state === "agreed" && e.revoke && !e.revoke.possible && e.revoke.reason && <span className="note revoke-reason">{e.revoke.reason}</span>}
      {done && <span className="note">{done}</span>}
      {agreeError != null && <ErrorBox error={agreeError} title="承認できません" />}
    </div>
  );
}

// Row は、表の 1 行。値は文字か、自然言語で書く項目のセグメントの配列。field は、差分（diff.fields）の項目の名前
type Row = [string, string | Piece[], string?];

// guessLines は、推測の並びを、1 行に 1 つずつのセグメントの配列にする（変更前と変更後の比べと、出し方を、ほかの項目とそろえるため）
function guessLines(gs: Piece[][]): Piece[] {
  return gs.flatMap((g, i) => (i > 0 ? [{ text: "\n" }, ...g] : g));
}

const show = (v: string | Piece[]) => (typeof v === "string" ? v : <Prose pieces={v} />);
const same = (a: string | Piece[], b: string | Piece[]) => JSON.stringify(a) === JSON.stringify(b);

function rows(kind: Entry["kind"], c: Entry["before"]): Row[] | null {
  if (!c) return null;
  const opt = (s: Piece[] | string | null) => s ?? "（なし）";
  switch (kind) {
    case "claim": {
      const x = c as ClaimContent;
      return [
        ["文", x.statement, "statement"],
        ["条件", opt(x.condition), "condition"],
        ["理由", opt(x.rationale), "rationale"],
        ["対象", x.subjects.join("・")],
        ["レイヤー", x.layers.join("・")],
        ["エビデンスの予定", x.evidence.map((ev) => `${stageLabel[ev.stage] ?? ev.stage} / ${typeLabel[ev.type] ?? ev.type} / ${ev.layers.join("・")}${ev.detail ? ` — ${proseText(ev.detail)}` : ""}`).join("\n")],
        ["TLA+ で確かめない理由", opt(x.modelCheckWaiver), "modelCheckWaiver"],
        ["推測", x.guesses.length > 0 ? guessLines(x.guesses) : "（なし）"],
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
  const before = rows(e.kind, e.before);
  const after = rows(e.kind, e.after);
  if (e.kind === "file") {
    // 行の差分がないファイル（追加・削除したもの）は、ある側の中身をそのまま出す。両側があれば、変更前・変更後の順に出す
    const show = (r: Row) => {
      const text = typeof r[1] === "string" ? r[1] : "";
      return e.fileType === "oracle" ? <TlaCode source={text} /> : <CodeBlock source={text} lang={langOf(e.path ?? "")} />;
    };
    const b = before?.[0];
    const a = after?.[0];
    if (b && a) {
      return (
        <div className="compare-file">
          <h3>変更前</h3>
          {show(b)}
          <h3>変更後</h3>
          {show(a)}
        </div>
      );
    }
    const r = a ?? b;
    return r ? show(r) : null;
  }
  return <ItemCompare entry={e} before={before} after={after} />;
}

// ItemCompare は、約束・用語・機能の変更前と変更後を、項目ごとに 1 列で出す（左右に分けない）。
// 変わっていない項目は 1 回だけ、変わった文の項目は API の文字の単位の差分（diff.fields）で、消した文字を del、足した文字を ins で出す。
// 文字の差分のない項目（対象・レイヤー・推測・真偽）が変わったときは、変更前の値を del、変更後の値を ins で並べる。
// 追加・削除した件は、ある側の中身だけをマークなしで出す（追加・削除は件の見出しに出している）
function ItemCompare(props: { entry: Pick<Entry, "kind" | "diff" | "before" | "after">; before: Row[] | null; after: Row[] | null }) {
  const { entry: e, before, after } = props;
  const fields = e.diff && "fields" in e.diff ? e.diff.fields : null;
  const evidenceMarks = e.diff && "fields" in e.diff ? e.diff.evidence : undefined;
  const labels = (after ?? before ?? []).map((r) => r[0]);
  const value = (i: number) => {
    const b = before?.[i];
    const a = after?.[i];
    const label = labels[i];
    if (e.kind === "claim" && label === "エビデンスの予定") {
      if (before && after && evidenceMarks) return <UnifiedEvidence before={(e.before as ClaimContent).evidence} after={(e.after as ClaimContent).evidence} marks={evidenceMarks} />;
      return <EvidenceList items={((after ? e.after : e.before) as ClaimContent).evidence} />;
    }
    if (e.kind === "claim" && label === "レイヤー") {
      const bl = before ? (e.before as ClaimContent).layers : null;
      const al = after ? (e.after as ClaimContent).layers : null;
      if (bl && al) return <LayerChipsDiff before={bl} after={al} side="both" />;
      return <LayerChips layers={(al ?? bl) as string[]} />;
    }
    const one = a ?? b;
    if (!b || !a) return one ? show(one[1]) : undefined;
    const parts = a[2] && fields ? fields[a[2]] : undefined;
    if (parts) return <Parts parts={parts} side="both" />;
    if (same(b[1], a[1])) return show(a[1]);
    return (
      <>
        <del className="diff-part">{show(b[1])}</del>
        <ins className="diff-part">{show(a[1])}</ins>
      </>
    );
  };
  return (
    <table className="compare compare-unified">
      <tbody>
        {labels.map((label, i) => (
          <tr key={label}>
            <th scope="row">{label}</th>
            <td className="text user-text">{value(i)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// UnifiedEvidence は、エビデンスの予定の差分を 1 列で出す。変更後の並びで出し（変わったものは項目の中で差分）、消したものを後ろに並べる
function UnifiedEvidence(props: { before: EvidenceContent[]; after: EvidenceContent[]; marks: { before: EvidenceMark[]; after: EvidenceMark[] } }) {
  const items: EvidenceContent[] = [...props.after];
  const marks: EvidenceMark[] = [...props.marks.after];
  props.before.forEach((ev, i) => {
    if (props.marks.before[i]?.op === "removed") {
      items.push(ev);
      marks.push(props.marks.before[i]);
    }
  });
  return <EvidenceList items={items} marks={marks} side="both" others={props.before} />;
}

// ClaimSections は、約束のエビデンスの予定と実際の表（RV-08。合格したかは段 3 で足す）と、変更履歴と承認の履歴（RV-16）。
export function ClaimSections(props: { id: string; query?: string }) {
  const d = useLoad(() => api.claim(props.id), props.id);
  const h = useLoad(() => api.claimHistory(props.id), props.id);
  return (
    <div>
      <h3>エビデンスの予定と実際</h3>
      {d.loading && <Loading />}
      {d.error != null && <ErrorBox error={d.error} title="エビデンスを探せません" />}
      {d.data && (
        <RefsProvider refs={d.data.refs}>
        <EvidenceGroups
          items={d.data.evidence}
          level="h4"
          render={(ev) => (
            <>
              <div className="evidence-line">
                <PresentIcon present={ev.present} />
                <LayerChips layers={ev.layers} />
              </div>
              {ev.detail && (
                <div className="evidence-detail">
                  <Prose pieces={ev.detail} />
                </div>
              )}
              {ev.found.length > 0 && (
                <ul className="evidence-found">
                  {ev.found.map((f) => (
                    <li key={`${f.file}:${f.line}`}>
                      {f.name}{" "}
                      <Link className="note" to={`/files?${props.query ? `${props.query}&` : ""}path=${encodeURIComponent(f.file)}`}>
                        {f.file}:{f.line}
                      </Link>{" "}
                      <AgreementIcon state={f.state} />
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        />
        </RefsProvider>
      )}
      <ClaimHistory id={props.id} history={h} level="h3" />
    </div>
  );
}

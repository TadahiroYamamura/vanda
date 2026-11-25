// ストーリーのレビューの画面は、項目・ファイルの詳細の画面へ移ると外れる。戻ったときに移る前のままにするもの
// （たたんだまとまりと、スクロールの位置）を、外れない App に持たせる。
// sessionStorage ではなくメモリに置くのは、保つ範囲が同じタブの中で足り、読み込み直した後のことは決まっていないため
import { useLayoutEffect, useRef, type RefObject } from "react";

interface BoxScroll {
  // 画面のルートから、子の何番目かをたどる道筋。スクロールするエリアを名前で並べず、どのエリアも同じ扱いにするため
  path: number[];
  left: number;
  top: number;
}

interface SavedScroll {
  key: string;
  x: number;
  y: number;
  boxes: BoxScroll[];
}

export interface ReviewMemory {
  folded: Set<string>;
  scroll: SavedScroll | null;
}

export function newReviewMemory(): ReviewMemory {
  return { folded: new Set(), scroll: null };
}

// 戻し続ける時間の上限。ストーリーの本文や 1 件の中身は一覧より後に届き、届くたびに高さが変わるので、1 回では戻しきれない
const RESTORE_LIMIT_MS = 10000;

function pathOf(root: Element, el: Element): number[] | null {
  const path: number[] = [];
  let cur: Element = el;
  while (cur !== root) {
    const parent: Element | null = cur.parentElement;
    if (!parent) return null;
    path.unshift(Array.prototype.indexOf.call(parent.children, cur));
    cur = parent;
  }
  return path;
}

function resolve(root: Element, path: number[]): Element | null {
  let cur: Element | undefined = root;
  for (const i of path) {
    cur = cur.children[i];
    if (!cur) return null;
  }
  return cur;
}

// useKeptScroll は、画面が外れるときのスクロールの位置（ページ全体と、画面の中のスクロールするエリア）を memory に残し、
// 同じ key（同じ一覧）で画面が戻ったときに、その位置へ戻す。ready は、一覧を読み込み終えたか。
// 読み込み終える前に戻すと、まだ低い画面の高さで位置が切り詰められるので、ready になるまで待ち、
// その後も中身が届いて画面が変わるたびに戻し直す。利用者が自分で操作・スクロールしたら、そこでやめる
export function useKeptScroll(memory: ReviewMemory, key: string, ready: boolean): RefObject<HTMLDivElement | null> {
  const root = useRef<HTMLDivElement>(null);
  const latest = useRef({ key, ready });
  latest.current = { key, ready };
  const restore = useRef<() => void>(() => {});

  useLayoutEffect(() => {
    const el = root.current;
    if (!el) return;
    let pending = memory.scroll && memory.scroll.key === latest.current.key ? memory.scroll : null;
    // ページ全体の位置は、外れるときに読まず、スクロールのたびと、画面を移る合図（popstate）のときに控える。
    // ブラウザの「戻る」「進む」では、合図の後、画面が外れる前にブラウザが位置を動かすことがあるため。
    // スクロールのたびだけでは、スクロールの直後に移ったとき、最後の位置を控える前に画面が外れる
    let page = { x: window.scrollX, y: window.scrollY };
    let deadline = 0;
    // 自分で戻した結果の位置。その位置と違うスクロールは、利用者のものとみなす
    const applied = new Map<EventTarget, { left: number; top: number }>();

    const stop = () => {
      pending = null;
      page = { x: window.scrollX, y: window.scrollY };
    };
    const apply = () => {
      if (!pending || !latest.current.ready) return;
      if (deadline === 0) deadline = Date.now() + RESTORE_LIMIT_MS;
      else if (Date.now() > deadline) {
        stop();
        return;
      }
      if (window.scrollX !== pending.x || window.scrollY !== pending.y) window.scrollTo(pending.x, pending.y);
      applied.set(document, { left: window.scrollX, top: window.scrollY });
      for (const b of pending.boxes) {
        const box = resolve(el, b.path);
        if (!box) continue;
        box.scrollLeft = b.left;
        box.scrollTop = b.top;
        applied.set(box, { left: box.scrollLeft, top: box.scrollTop });
      }
    };
    restore.current = apply;

    const onScroll = (e: Event) => {
      if (pending) {
        // 戻し始める前のスクロールは、移った先の画面より低くなって切り詰められたもの。利用者のものではないので、控えない
        if (deadline === 0) return;
        const t = e.target;
        const a = t ? applied.get(t) : undefined;
        const now = t === document ? { left: window.scrollX, top: window.scrollY } : t instanceof Element ? { left: t.scrollLeft, top: t.scrollTop } : null;
        if (a && now && Math.abs(a.left - now.left) <= 1 && Math.abs(a.top - now.top) <= 1 && Date.now() <= deadline) return;
        stop();
      }
      page = { x: window.scrollX, y: window.scrollY };
    };
    const onLeave = () => {
      if (!pending) page = { x: window.scrollX, y: window.scrollY };
    };
    const onInput = () => {
      if (pending) stop();
    };
    const inputs = ["wheel", "pointerdown", "keydown", "touchstart"];
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("popstate", onLeave, true);
    for (const n of inputs) window.addEventListener(n, onInput, true);
    const mutation = new MutationObserver(apply);
    mutation.observe(el, { childList: true, subtree: true });
    // 画像やフォントが届いて高さだけが変わるときは、DOM が変わらないので、大きさでも見る
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(apply);
    resize?.observe(el);

    return () => {
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("popstate", onLeave, true);
      for (const n of inputs) window.removeEventListener(n, onInput, true);
      mutation.disconnect();
      resize?.disconnect();
      restore.current = () => {};
      // 戻しきる前に外れたときは、戻す先の位置をそのまま残す（途中の位置で上書きしない）
      if (pending) return;
      const boxes: BoxScroll[] = [];
      for (const box of el.querySelectorAll("*")) {
        if (box.scrollTop === 0 && box.scrollLeft === 0) continue;
        const path = pathOf(el, box);
        if (path) boxes.push({ path, left: box.scrollLeft, top: box.scrollTop });
      }
      memory.scroll = { key: latest.current.key, x: page.x, y: page.y, boxes };
    };
  }, [memory]);

  useLayoutEffect(() => {
    if (ready) restore.current();
  }, [ready]);

  return root;
}

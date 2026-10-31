// 文法に沿った色付け（見せ方だけで、判定には使わない）。使う言語の文法だけを、要るときに読み込む。
//
// TLA+ の文法（grammars/tlaplus/）は、VS Code の TLA+ の拡張 alygin/vscode-tlaplus 1.5.4（今は tlaplus/vscode-tlaplus）の
// languages/ にあるもの。MIT（grammars/tlaplus/LICENSE.txt）。nixpkgs の vscode-extensions.alygin.vscode-tlaplus から取り出した。
// シナリオ（txtar）は文法の定義がないので、grammars/txtar.json に小さく書いた。
import type { HighlighterCore, LanguageRegistration, ThemedToken } from "shiki/core";

export type Lang = "go" | "sql" | "tlaplus" | "txtar";

export interface Token {
  content: string;
  style: Record<string, string>;
}

const loaders: Record<Lang, () => Promise<LanguageRegistration[]>> = {
  go: async () => (await import("shiki/langs/go.mjs")).default,
  sql: async () => (await import("shiki/langs/sql.mjs")).default,
  tlaplus: async () => {
    const [tla, pluscal] = await Promise.all([import("./grammars/tlaplus/tlaplus-grammar.json"), import("./grammars/tlaplus/pluscal-grammar.json")]);
    return [
      { ...(pluscal.default as unknown as LanguageRegistration), name: "pluscal" },
      { ...(tla.default as unknown as LanguageRegistration), name: "tlaplus", embeddedLangs: ["pluscal"] },
    ];
  },
  txtar: async () => [{ ...((await import("./grammars/txtar.json")).default as unknown as LanguageRegistration), name: "txtar" }],
};

let core: Promise<HighlighterCore> | null = null;
const loaded = new Map<Lang, Promise<void>>();

// 正規表現のエンジンは Oniguruma（WebAssembly、約 600KB）にしている。TextMate の文法は Oniguruma の正規表現で書かれていて、
// VS Code と同じ色付けになるため。Oniguruma の本体（kkos/oniguruma）は 2025-04 にアーカイブされたが、Shiki が使う
// WebAssembly の版（microsoft/vscode-oniguruma）は VS Code が使い続けていて、手入れされている。問題が出たら、まず Shiki の
// JavaScript の正規表現のエンジン（shiki/engine/javascript）に替え、Go・SQL・TLA+・txtar の色付けが崩れないかを確かめる。
// 崩れるなら Prism を考える（TLA+ の文法は自分で書くことになる）。
function highlighter(): Promise<HighlighterCore> {
  if (!core) {
    core = (async () => {
      const [{ createHighlighterCore }, { createOnigurumaEngine }, light, dark] = await Promise.all([
        import("shiki/core"),
        import("shiki/engine/oniguruma"),
        import("shiki/themes/github-light.mjs"),
        import("shiki/themes/github-dark.mjs"),
      ]);
      return createHighlighterCore({ themes: [light.default, dark.default], langs: [], engine: createOnigurumaEngine(import("shiki/wasm")) });
    })();
  }
  return core;
}

async function ready(lang: Lang): Promise<HighlighterCore> {
  const h = await highlighter();
  let p = loaded.get(lang);
  if (!p) {
    p = loaders[lang]().then((regs) => h.loadLanguage(...regs));
    loaded.set(lang, p);
  }
  await p;
  return h;
}

// tokenize は、text を行ごとの色付きのかけらにする。行は、text を "\n" で分けたものと同じ数・同じ中身。
export async function tokenize(text: string, lang: Lang): Promise<Token[][]> {
  const h = await ready(lang);
  const res = h.codeToTokens(text, { lang, themes: { light: "github-light", dark: "github-dark" }, defaultColor: false });
  return res.tokens.map((line: ThemedToken[]) => line.map((t) => ({ content: t.content, style: (t.htmlStyle ?? {}) as Record<string, string> })));
}

export function langOf(path: string): Lang | null {
  if (path.endsWith(".tla")) return "tlaplus";
  if (path.endsWith(".sql")) return "sql";
  if (path.endsWith(".go")) return "go";
  if (path.endsWith(".txtar")) return "txtar";
  return null;
}

import { defineConfig, type Plugin } from "vitest/config";
import react from "@vitejs/plugin-react";

// fontsourceWoff2Only は、@fontsource の CSS から woff の読み込みを外し、woff2 だけを画面に入れる。
// 今のブラウザはどれも woff2 を読めるので、woff は使われないまま Go の実行ファイルを大きくするだけのため。
function fontsourceWoff2Only(): Plugin {
  return {
    name: "fontsource-woff2-only",
    enforce: "pre",
    transform(code, id) {
      if (!id.includes("/@fontsource/") || !id.endsWith(".css")) return null;
      return code.replace(/,\s*url\([^)]*\.woff\)\s*format\(['"]woff['"]\)/g, "");
    },
  };
}

export default defineConfig({
  plugins: [fontsourceWoff2Only(), react()],
  build: {
    // dist/ そのものは Go の go:embed の置き場所で、中の .keep だけをコミットする（画面を作る前でも Go をビルドできるように）。
    // 作った画面は dist/app/ に出し、emptyOutDir で消すのもそこだけにする
    outDir: "dist/app",
    emptyOutDir: true,
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});

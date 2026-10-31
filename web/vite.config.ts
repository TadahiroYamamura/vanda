import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
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

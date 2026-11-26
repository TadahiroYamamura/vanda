import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "e2e",
  use: {
    browserName: "chromium",
    // 落ちたテストだけ、操作ごとの画面と通信の記録（test-results/ の trace.zip）とスクリーンショットを残す。
    // ときどきしか落ちないテストは、落ちた時の記録がないと原因を調べられない
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});

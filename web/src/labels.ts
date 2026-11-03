// 画面に出す言葉（約束リストの用語の言葉）。
import type { Change, FileType, Flag, Kind, State } from "./api";

export const kindLabel: Record<Kind, string> = { claim: "約束", term: "用語", feature: "機能", file: "ファイル" };

export const fileTypeLabel: Record<FileType, string> = {
  scenario: "シナリオ",
  oracle: "オラクルの定義",
  guardrail: "ガードレールの定義",
};

export const changeLabel: Record<Change, string> = {
  added: "追加",
  modified: "変更",
  deleted: "削除",
  unchanged: "変更なし",
};

export const stateLabel: Record<State, string> = { proposed: "提案中", agreed: "合意済" };

export const detachedLabel = "合意が外れた";

export const flagLabel: Record<Flag, string> = {
  "ai-proposal": "AI 案",
  guess: "推測",
  "changed-since-agreement": "差分あり",
};

export const flagHelp: Record<Flag, string> = {
  "ai-proposal": "要求にも質疑にもなく、AI が自分で決めたもの",
  guess: "AI が確かめずに推し量って書いた所があるもの",
  "changed-since-agreement": "前の合意から中身が変わったもの",
};

export const stageLabel: Record<string, string> = {
  plan: "計画",
  implementation: "実装",
  release: "リリース",
  operation: "運用",
};

export const typeLabel: Record<string, string> = {
  "model-check": "モデル検査",
  proof: "証明",
  "cross-check": "クロスチェック",
  "random-check": "ランダムチェック",
  "scenario-test": "シナリオテスト",
  "unit-test": "単体テスト",
  "interface-test": "インターフェースのテスト",
  "concurrency-test": "同時実行のテスト",
  guardrail: "ガードレール",
  sensor: "センサー",
  "human-review": "人のレビュー",
};

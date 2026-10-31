# vanda

エビデンス・ファースト開発の道具一式。何を作るかは `docs/spec.md`、言葉は `docs/terms.md` にある。

## 言葉

- `docs/terms.md` の言葉を使う。コードを読み解く中で自分が名付けた言葉を、名前・コメント・テストの名前・コミットのメッセージに持ち込まない
- 新しい言葉が要るときは、使う前に監督に聞く

## 文言（CLI の出力と画面）

- CLI の出力と画面の文言は、です・ます調にする。docs/ や計画・規則の文体（だ・である調の言い切り）を持ち込まない
- 状態の報告は「〜しました」「〜しています」にする（例: `vanda を起動しました: http://127.0.0.1:44927/（Ctrl+C で停止します）`）
- 誤りは「<何が><どうなっているか>。<どうすればよいか>してください。」にする（例: `.vanda/promises.toml:21: 約束 task-add-saves-todo の今の中身に、まだ合意していません。中身を確かめてから合意してください。`）。主語の分からない使役（「〜させる」）や、命令とも説明ともつかない言い切りにしない
- 画面の見出し・項目の名前は名詞にする（例: 「変わったもの」「ベースブランチ」）。ボタンは「〜する」にする（例: 「合意する」）
- 言葉は docs/terms.md のものを使う

## 作り

- Go の 1 つの実行ファイル `vanda`（`cmd/vanda`）が、CLI・GUI のサーバー・MCP のサーバーを兼ねる。GUI は React で作り、できた画面のファイルを Go に埋め込む（段 2 から）
- 開発の道具は nix の flake で揃える
- `make check` で、ビルド、テスト、TLA+ のモデルの検査をすべて動かす。CI（GitHub Actions）は Linux と macOS の両方で `make check` を動かす
- `.vanda/` の外の取り決め（アノテーションの書き方、CI からの CLI の呼び方、アダプタとの受け渡しの形、添付の種類）は、v1.0.0 からは変えない。今のうちに形をよく考え、決めたら `docs/` に書く

## コード

- `panic`、comma-ok なしの型の取り出しのように、エラーを値として返さずに止まる書き方をしない。エラーを返し、呼び出し元で扱う
- エラーのメッセージには、どのファイルの何行目の、何がいけないか、どう直せばよいかを書く
- 新しい依存を足す前に、監督に聞く。了承済みの依存は次のとおり

| 依存 | 使う所 |
|---|---|
| `github.com/BurntSushi/toml` | TOML の読み込み |
| `github.com/spf13/cobra` | CLI |
| `github.com/rogpeppe/go-internal`（`testscript`） | シナリオテスト |
| `pgregory.net/rapid` | ランダムチェック |
| `github.com/sergi/go-diff` | 一覧の変更前と変更後の差分（行の単位と文字の単位） |
| `react`、`react-dom` | 画面（`web/`） |
| `shiki` | コードの色付け（Go・SQL は Shiki の文法、TLA+ は `web/src/grammars/tlaplus/`（vscode-tlaplus の MIT の文法）、シナリオは `web/src/grammars/txtar.json`） |
| `@fontsource/noto-sans-jp`、`@fontsource/jetbrains-mono` | 画面のフォント（同梱。400・500・700 と 400・500 の woff2 だけ。OFL は `web/public/assets/`） |
| `react-markdown`、`remark-gfm` | ストーリーの本文を Markdown として描く（表などは `remark-gfm`）。生の HTML は描かない |
| `@types/react`、`@types/react-dom` | 画面を TypeScript で書くための React の型 |
| `typescript`、`vite`、`@vitejs/plugin-react` | 画面を作る |
| `vitest`、`@testing-library/react`、`@testing-library/dom`、`@testing-library/user-event`、`jsdom` | 画面の部品のテスト（`@testing-library/dom` は `@testing-library/react` が求めるもの） |
| `msw` | 画面の部品のテストで API を差し替える |
| `@playwright/test` | E2E のテスト（Chrome を動かす）。版は flake の nixpkgs の `playwright-driver` と同じにする |

- コメントは、なぜこの形なのか（制約、選ばなかった案、直感に反する点）だけを書く。何をしているかは、コードで分かるようにする

## テスト

- テストの名前は日本語で、何を確かめているかを文で書く（例: `合意の後にフラグだけを立てると合意の審査で止まる`）
- 期待値は、仕様（`docs/spec.md`）や手で作った例から用意する。実装やそれが使うライブラリの出力から作らない
- 有限で小さい領域（エビデンスのタイプ、ステージ、アノテーションの種類など）は、全部を回す
- 判定のテストを書いたら、判定をわざと壊して落ちることを確かめる

## 分担

- 開発者のエージェントは実装を、テスト担当のエージェントはテストを書く。互いのファイルを書き換えない。テスト担当は、実装を読んで期待値を作らない
- 計画にない判断は、作る前に監督に聞く。部品ごとに、監督に報告する

## Git

- コミットのメッセージは日本語で、`(type) subject` の形にする。type は `feat`・`fix`・`test`・`docs`・`refactor`・`chore`。subject は「〜を追加する」「〜を修正する」の形
- 目的の違う変更は、コミットを分ける
- プッシュはしない（監督が、PO の指示を受けて行う）。`git push --force`、`git reset --hard`、履歴の書き換えをしない

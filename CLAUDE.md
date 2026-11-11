# vanda

エビデンス・ファースト開発の道具一式。作ったものの用語・機能・振る舞いは約束リスト（`.vanda/promises.toml`）に、まだ作っていない機能は `docs/spec.md` にある。

## 言葉

- 約束リスト（`.vanda/promises.toml`）の用語（`[[term]]`）の言葉を使う。コードを読み解く中で自分が名付けた言葉を、名前・コメント・テストの名前・コミットのメッセージに持ち込まない
- 新しい言葉が要るときは、使う前に監督に聞く
- 英語を直訳した言い方にせず、ふだん使う言い方（カタカナ語を含む）にする（例: 「問いかけ」ではなく「質問」、「応答」ではなく「レスポンス」、「押す」ではなく「クリック」）

## 文言（CLI の出力と画面）

- CLI の出力と画面の文言は、です・ます調にする。docs/ や計画・規則の文体（だ・である調の言い切り）を持ち込まない
- 状態の報告は「〜しました」「〜しています」にする（例: `vanda を起動しました: http://127.0.0.1:44927/（Ctrl+C で停止します）`）
- 誤りは「<何が><どうなっているか>。<どうすればよいか>してください。」にする（例: `.vanda/promises.toml:21: 約束 task-add-saves-todo の今の中身に、まだ承認していません（提案中）。中身を確かめてから承認し、承認の記録に書いてください。`）。主語の分からない使役（「〜させる」）や、命令とも説明ともつかない言い切りにしない
- 画面の見出し・項目の名前は名詞にする（例: 「変わったもの」「ベースブランチ」）。ボタンは「〜する」にする（例: 「承認する」）
- 言葉は約束リスト（`.vanda/promises.toml`）の用語のものを使う

## 作り

- Go の 1 つの実行ファイル `vanda`（`cmd/vanda`）が、CLI・vanda サーバー（画面と API）・MCP のサーバーを兼ねる。GUI は React で作り、できた画面のファイルを Go に埋め込む（段 2 から）
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
| `github.com/charmbracelet/huh`（v2 からは `charm.land/huh/v2`。キーの割り当てに使う `charm.land/bubbles/v2` と、それが使う charmbracelet の部品） | `vanda init` の質問（選ぶ・複数選ぶ・入力・確認） |
| `github.com/creack/pty` | `vanda init` の質問のテストで擬似端末を作る（テストだけで使う） |
| `github.com/sergi/go-diff` | 一覧の変更前と変更後の差分（行の単位と文字の単位） |
| `react`、`react-dom` | 画面（`web/`） |
| `shiki` | コードの色付け（Go・SQL は Shiki の文法、TLA+ は `web/src/grammars/tlaplus/`（vscode-tlaplus の MIT の文法）、シナリオは `web/src/grammars/txtar.json`） |
| `@fontsource/noto-sans-jp`、`@fontsource/jetbrains-mono` | 画面のフォント（同梱。400・500・700 と 400・500 の woff2 だけ。OFL は `web/public/assets/`） |
| `@fontsource/noto-sans-math` | TLA+ の数学の記号を描くフォント（同梱。400 の woff2 だけ。OFL は `web/public/assets/`） |
| `react-markdown`、`remark-gfm` | ストーリーの本文を Markdown として描く（表などは `remark-gfm`）。生の HTML は描かない |
| `@types/react`、`@types/react-dom` | 画面を TypeScript で書くための React の型 |
| `typescript`、`vite`、`@vitejs/plugin-react` | 画面を作る |
| `vitest`、`@testing-library/react`、`@testing-library/dom`、`@testing-library/user-event`、`jsdom` | 画面の部品のテスト（`@testing-library/dom` は `@testing-library/react` が求めるもの） |
| `msw` | 画面の部品のテストで API を差し替える |
| `@playwright/test` | E2E のテスト（Chrome を動かす）。バージョンは flake の nixpkgs の `playwright-driver` と同じにする |
| `expect`（nix の flake の packages） | `vanda init` のシナリオテスト（問いを待って答える流れを、端末で動かす） |

- コメントは、なぜこの形なのか（制約、選ばなかった案、直感に反する点）だけを書く。何をしているかは、コードで分かるようにする

## テスト

- テストの名前は日本語で、何を確かめているかを文で書く（例: `承認の後にフラグだけを立てると承認のチェックで止まる`）
- 期待値は、約束リスト（`.vanda/promises.toml`）・`docs/formats.md` や手で作った例から用意する。実装やそれが使うライブラリの出力から作らない
- 有限で小さい領域（エビデンスのタイプ、ステージ、アノテーションの種類など）は、全部を回す
- 判定のテストを書いたら、判定をわざと壊して落ちることを確かめる

## 失敗しやすいこと（`docs/pitfalls.jsonl`）

- 間違えても気づきにくく、被害の大きい構造を、1 行に 1 つ書いてある
- 開発者は作る前に、テスト担当はテストを書く前に読み、`trigger` にマッチするものの `question` を確かめる
- 足す・直す・消すときの基準と手順は `.claude/rules/pitfalls.md`。開発者とテスト担当は直接書き換えず、報告に候補として書く
- いつも守る書き方の決まりは、pitfalls ではなく `.claude/rules/` に、当てはまるファイルの `paths` を付けて書く

## 分担

- 開発者のエージェントは実装を、テスト担当のエージェントはテストを書く。互いのファイルを書き換えない。テスト担当は、実装を読んで期待値を作らない
- 計画にない判断は、作る前に監督に聞く。部品ごとに、監督に報告する

## Git

- ブランチを作る・コミットする・プッシュする・取り込む・片付けるときは、スキル git-workflow に従う。いつも守る決まりは `.claude/rules/git.md`
- ストーリーの中のコミットには、メッセージの末尾に、そのストーリーの `Story: <ID>` のトレーラー（例: `Story: #4`）を書く。vanda はトレーラーからストーリーを読み、subject と本文の ID は数えない。subject にストーリーの ID は入れない。トレーラーは、Co-Authored-By などのほかのトレーラーと空行をはさまず、同じ段落に書く

# 段 1 の CLI の文言の直し（前と今の対応表）

CLAUDE.md の「文言（CLI の出力と画面）」に合わせて、段 1 の CLI（審査の誤り、設定・約束リスト・合意の記録・アノテーションの読み込みの誤り、doctor）の文言を、です・ます調に直す。テスト担当が期待値を直し、実装の直しと同じときに `main` に合わせる。

## 変えないもの

- 誤りの骨組み: `<パス>:<行>: <何がどうなっているか>。<どうすればよいか>` と、行のないときの `<パス>: <場所>: …`
- 審査の結果の行: `審査 <名前>（<範囲>）: 通った` と `審査 <名前>（<範囲>）: 失敗（<件数> 件）`
- 審査・項目・値の名前（`promise-list-shape`、`flags` など）と、言葉（docs/terms.md）
- `vanda version` の出力

## 決まり

| 何を | 前 | 今 |
|---|---|---|
| 誤りの終わり | 直し方の後に「。」を付けない（`…合意の記録に書く`） | 直し方の後にも「。」を付ける（`…合意の記録に書いてください。`）。`diag.Error` が付けるので、下の表の直し方の断片には「。」を書いていない。`fmt.Errorf` でそのまま出す誤り（`--base`・`--story`・審査の設定のファイルなど）は、断片に「。」まで書いてある |
| 直し方のない誤り | `<パス>: <何がどうなっているか>`（「。」なし） | `<パス>: <何がどうなっているか>。`（どの誤りも「。」で終わる） |
| 何がどうなっているか | だ・である調の言い切り（`…がない`、`…でない`、`…している`） | です・ます調（`…がありません`、`…ではありません`、`…しています`） |
| どうすればよいか | 言い切り（`…を書く`、`…にする`） | `…してください` |
| vanda の不具合 | `vanda の不具合なので、知らせる` | `vanda の不具合です。開発者に知らせてください` |

## 出力の例（前 → 今）

| 前 | 今 |
|---|---|
| `.vanda/promises.toml:21: 約束 a の今の中身に合意していない（提案中。指紋 sha256:…）。中身を確かめて合意し、合意の記録に書く` | `.vanda/promises.toml:21: 約束 a の今の中身に、まだ合意していません（提案中。指紋 sha256:…）。中身を確かめてから合意し、合意の記録に書いてください。` |
| `.vanda/config.toml:1: 動いている vanda（0.1.0）が、設定の版（9.9.9）より古い。vanda を 9.9.9 に上げる` | `.vanda/config.toml:1: 動いている vanda（0.1.0）が、設定の版（9.9.9）より古いです。vanda を 9.9.9 に上げてください。` |
| `設定を確かめた: .vanda/config.toml（リポジトリ …、版 0.1.0）` | `設定を確かめました: .vanda/config.toml（リポジトリ …、版 0.1.0）` |
| `審査の設定に scope = "story" の審査（agreement）があるので、ベースブランチが要る。--base <ブランチ> を渡す` | `審査の設定に scope = "story" の審査（agreement）があるため、ベースブランチが必要です。--base <ブランチ> を渡してください。` |
| `約束リストが読めないため行えない（誤りが 2 件ある）。審査 promise-list-shape で誤りを出して直す` | `約束リストを読めないため、この審査を行えません（誤りが 2 件あります）。審査 promise-list-shape で誤りを確かめて、直してください。` |

## 対応表（ソースの文字列の断片。`"…"+x+"…"` の `x` には、ID や場所が入る）

| 場所 | 前 | 今 |
|---|---|---|
| `internal/config/config.go` | `" が決まった形でない", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: model、db）にする"` | `" が決まった形ではありません", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: model、db）にしてください"` |
| `internal/config/config.go` | `" がほかのレイヤーと重なっている", "別の ID にする"` | `" がほかのレイヤーと重なっています", "別の ID にしてください"` |
| `internal/config/config.go` | `"adapter と command の両方を書いている", "中に持つアダプタを使うなら adapter だけを、外部のコマンドを使うなら command だけを書く"` | `"adapter と command の両方が書かれています", "vanda の中のアダプタを使うなら adapter だけを、外部のコマンドを使うなら command だけを書いてください"` |
| `internal/config/config.go` | `" は vanda の中にない", "adapter を "+strings.Join(BuiltinAdapters, "・")+" のどれかにするか、外部のコマンドを command = [\"…\"] で書く"` | `" は vanda の中にありません", "adapter を "+strings.Join(BuiltinAdapters, "・")+" のどれかにするか、外部のコマンドを command = [\"…\"] で書いてください"` |
| `internal/config/config.go` | `"command に実行するコマンドがない", ˋcommand = ["コマンド", "引数"] のように書くˋ` | `"command に、実行するコマンドがありません", ˋcommand = ["コマンド", "引数"] のように書いてくださいˋ` |
| `internal/config/config.go` | `" は vanda の中にない", ˋadapter = "file" にする（今はファイルを読むだけのアダプタしかない）ˋ` | `" は vanda の中にありません", ˋadapter = "file" にしてください（今は、ファイルを読むだけのアダプタしかありません）ˋ` |
| `internal/config/config.go` | `"vanda の中のストーリーの ID の既定の書き方が読めない（"+err.Error()+"）", "vanda の不具合なので、知らせる"` | `"vanda の中の、ストーリーの ID の既定の書き方を読めません（"+err.Error()+"）", "vanda の不具合です。開発者に知らせてください"` |
| `internal/config/config.go` | `"id_pattern が正規表現として読めない（"+err.Error()+"）", "Go の正規表現（RE2）の書き方で直す"` | `"id_pattern を正規表現として読めません（"+err.Error()+"）", "Go の正規表現（RE2）の書き方で直してください"` |
| `internal/config/config.go` | `"id_pattern が空の文字列に当たる", "ストーリーの ID 全体に当たる正規表現（例: #[0-9]+）にする"` | `"id_pattern が空の文字列に当たります", "ストーリーの ID 全体に当たる正規表現（例: #[0-9]+）にしてください"` |
| `internal/config/config.go` | `"ストーリーの ID の書き方を、ID 全体に当たる形にできない（"+err.Error()+"）", "id_pattern を直す"` | `"ストーリーの ID の書き方を、ID 全体に当たる形にできません（"+err.Error()+"）", "id_pattern を直してください"` |
| `internal/config/config.go` | `"[story] に adapter と command の両方を書いている", "中に持つアダプタを使うなら adapter だけを、外部のコマンドを使うなら command だけを書く"` | `"[story] に adapter と command の両方が書かれています", "vanda の中のアダプタを使うなら adapter だけを、外部のコマンドを使うなら command だけを書いてください"` |
| `internal/config/config.go` | `" は vanda の中にない", ˋadapter = "github" にするか、外部のコマンドを command = ["…"] で書くˋ` | `" は vanda の中にありません", ˋadapter = "github" にするか、外部のコマンドを command = ["…"] で書いてくださいˋ` |
| `internal/config/config.go` | `"[story] の command に実行するコマンドがない", ˋcommand = ["コマンド", "引数"] のように書くˋ` | `"[story] の command に、実行するコマンドがありません", ˋcommand = ["コマンド", "引数"] のように書いてくださいˋ` |
| `internal/config/config.go` | `ˋadapter = "github" なのに repository がないˋ, ˋrepository = "<owner>/<repo>" を書くˋ` | `ˋadapter = "github" なのに、repository がありませんˋ, ˋrepository = "<owner>/<repo>" を書いてくださいˋ` |
| `internal/config/config.go` | `" が <owner>/<repo> の形でない", ˋrepository = "TadahiroYamamura/vanda" のように書くˋ` | `" が <owner>/<repo> の形ではありません", ˋrepository = "TadahiroYamamura/vanda" のように書いてくださいˋ` |
| `internal/config/config.go` | `" が http:// か https:// で始まる URL でない", ˋapi_url = "https://api.github.com" のように書くˋ` | `" が、http:// か https:// で始まる URL ではありません", ˋapi_url = "https://api.github.com" のように書いてくださいˋ` |
| `internal/config/config.go` | `ˋrepository は adapter = "github" のときだけ書けるˋ, ˋadapter = "github" を書くか、repository を消すˋ` | `ˋrepository は、adapter = "github" のときだけ書けますˋ, ˋadapter = "github" を書くか、repository を消してくださいˋ` |
| `internal/config/config.go` | `ˋapi_url は adapter = "github" のときだけ書けるˋ, ˋadapter = "github" を書くか、api_url を消すˋ` | `ˋapi_url は、adapter = "github" のときだけ書けますˋ, ˋadapter = "github" を書くか、api_url を消してくださいˋ` |
| `internal/config/config.go` | `" が major.minor.patch の形でない", ˋversion = "ˋ+version.Current+ˋ" のように書くˋ` | `" が major.minor.patch の形ではありません", ˋversion = "ˋ+version.Current+ˋ" のように書いてくださいˋ` |
| `internal/config/config.go` | `"動いている vanda の版 "+quote(version.Current)+" が読めない", "vanda の不具合なので、知らせる"` | `"動いている vanda の版 "+quote(version.Current)+" を読めません", "vanda の不具合です。開発者に知らせてください"` |
| `internal/config/config.go` | `"）より古い", "vanda を "+v+" に上げる"` | `"）より古いです", "vanda を "+v+" に上げてください"` |
| `internal/config/config.go` | `"）より新しい", "vanda の移行のコマンドで .vanda/ を "+version.Current+" の形に書き換える"` | `"）より新しいです", "vanda の移行のコマンドで、.vanda/ を "+version.Current+" の形に書き換えてください"` |
| `internal/config/config.go` | `" にパターンがない", key+ˋ = ["…"] のように、1 つ以上書くˋ` | `" にパターンがありません", key+ˋ = ["…"] のように、1 つ以上書いてくださいˋ` |
| `internal/config/config.go` | `" の書き方が誤っている", "リポジトリの直下からの / 区切りの場所で書く（* と ? と [] と、何段のディレクトリにも当たる ** が使える）"` | `" の書き方が誤っています", "リポジトリの直下からの / 区切りの場所で書いてください（* と ? と [] と、何段のディレクトリにも当たる ** が使えます）"` |
| `internal/tomlread/tomlread.go` | `"ファイルを読めない（" + err.Error() + "）"` | `"ファイルを読めません（" + err.Error() + "）"` |
| `internal/tomlread/tomlread.go` | `"ファイルの場所と、読む権限を確かめる"` | `"ファイルの場所と、読む権限を確かめてください"` |
| `internal/tomlread/tomlread.go` | `"ファイルがない", "ファイルを作るか、設定（.vanda/config.toml）の場所を直す"` | `"ファイルがありません", "ファイルを作るか、設定（.vanda/config.toml）の場所を直してください"` |
| `internal/tomlread/tomlread.go` | `"TOML として読めない（" + pe.Message + "）", Fix: "TOML の書き方に合わせて直す"` | `"TOML として読めません（" + pe.Message + "）", Fix: "TOML の書き方に合わせて直してください"` |
| `internal/tomlread/tomlread.go` | `"TOML として読めない（" + err.Error() + "）", Fix: "TOML の書き方に合わせて直す"` | `"TOML として読めません（" + err.Error() + "）", Fix: "TOML の書き方に合わせて直してください"` |
| `internal/tomlread/tomlread.go` | `fmt.Sprintf("%s がない", t.keyLabel(key)), fmt.Sprintf("%s を書く", example)` | `fmt.Sprintf("%s がありません", t.keyLabel(key)), fmt.Sprintf("%s を書いてください", example)` |
| `internal/tomlread/tomlread.go` | `" が文字列でない", key+ˋ = "…" のように文字列で書くˋ` | `" が文字列ではありません", key+ˋ = "…" のように、文字列で書いてくださいˋ` |
| `internal/tomlread/tomlread.go` | `" が真偽でない", key+" = true か "+key+" = false と書く"` | `" が真偽ではありません", key+" = true か "+key+" = false と書いてください"` |
| `internal/tomlread/tomlread.go` | `" が日時でない", key+" = 2026-10-31T12:00:00+09:00 のように、時差の付いた日時で書く"` | `" が日時ではありません", key+" = 2026-10-31T12:00:00+09:00 のように、時差の付いた日時で書いてください"` |
| `internal/tomlread/tomlread.go` | `" に時差がない", key+" = 2026-10-31T12:00:00+09:00 のように、時差（+09:00 や Z）を付ける"` | `" に時差がありません", key+" = 2026-10-31T12:00:00+09:00 のように、時差（+09:00 や Z）を付けてください"` |
| `internal/tomlread/tomlread.go` | `" が文字列の配列でない", key+ˋ = ["…"] のように書くˋ` | `" が文字列の配列ではありません", key+ˋ = ["…"] のように書いてくださいˋ` |
| `internal/tomlread/tomlread.go` | `" に文字列でない値がある", key+ˋ = ["…"] のように、文字列だけを並べるˋ` | `" に、文字列でない値があります", key+ˋ = ["…"] のように、文字列だけを並べてくださいˋ` |
| `internal/tomlread/tomlread.go` | `" が表の配列でない", "[["+header(t.keyPath(key))+"]] の表で書く")` | `" が表の配列ではありません", "[["+header(t.keyPath(key))+"]] の表で書いてください")` |
| `internal/tomlread/tomlread.go` | `" が表の配列でない", "[["+header(t.keyPath(key))+"]] の表で書く")` | `" が表の配列ではありません", "[["+header(t.keyPath(key))+"]] の表で書いてください")` |
| `internal/tomlread/tomlread.go` | `" が表でない", "["+header(t.keyPath(key))+"] の表で書く"` | `" が表ではありません", "["+header(t.keyPath(key))+"] の表で書いてください"` |
| `internal/tomlread/tomlread.go` | `" は決まった項目でない", "消すか、名前を直す（書ける項目: "+allowed+"）"` | `" は決まった項目ではありません", "消すか、名前を直してください（書ける項目: "+allowed+"）"` |
| `internal/promises/promises.go` | `"約束リストのファイルがない", Fix: "ファイルを作るか、.vanda/config.toml の promises を直す"` | `"約束リストのファイルがありません", Fix: "ファイルを作るか、.vanda/config.toml の promises を直してください"` |
| `internal/promises/promises.go` | `"evidence が 1 つもない", "[[claim.evidence]] を 1 つ以上書く"` | `"evidence が 1 つもありません", "[[claim.evidence]] を 1 つ以上書いてください"` |
| `internal/promises/promises.go` | `key+" が空", key+" に"+what+" を 1 つ以上書く"` | `key+" が空です", key+" に"+what+" を 1 つ以上書いてください"` |
| `internal/agreements/agreements.go` | `"合意の記録のファイルがない", Fix: "ファイルを作るか、.vanda/config.toml の agreements を直す"` | `"合意の記録のファイルがありません", Fix: "ファイルを作るか、.vanda/config.toml の agreements を直してください"` |
| `internal/agreements/agreements.go` | `" は決まった値でない", "kind を "+strings.Join(Kinds, "・")+" のどれかにする"` | `" は決まった値ではありません", "kind を "+strings.Join(Kinds, "・")+" のどれかにしてください"` |
| `internal/agreements/agreements.go` | `" が指紋の形でない", "sha256: に続けて 16 進の小文字 64 文字で書く"` | `" が指紋の形ではありません", "sha256: に続けて、16 進の小文字 64 文字で書いてください"` |
| `internal/agreements/agreements.go` | `"対象がファイルの合意に path がない", ˋpath = "…" に、合意したときのファイルの場所を書くˋ` | `"対象がファイルの合意に、path がありません", ˋpath = "…" に、合意したときのファイルの場所を書いてくださいˋ` |
| `internal/agreements/agreements.go` | `"対象がファイルの合意の key が fingerprint と違う", "key にも、ファイルの中身の指紋を書く"` | `"対象がファイルの合意の key が、fingerprint と違います", "key にも、ファイルの中身の指紋を書いてください"` |
| `internal/agreements/agreements.go` | `" の合意に path がある", "path は対象がファイルのときだけ書く。消す"` | `" の合意に、path があります", "path は対象がファイルのときだけ書けます。消してください"` |
| `internal/agreements/fingerprint.go` | `"UTF-8 でない文字列がある"` | `"UTF-8 でない文字列があります"` |
| `internal/agreements/fingerprint.go` | `"指紋を作れない種類の値（%T）がある"` | `"指紋を作れない種類の値（%T）があります"` |
| `internal/annotation/annotation.go` | `"）の 2 つ以上のパターンに当たる", Fix: "どれか 1 つの [[annotation]] にだけ当たるように、パターンを直す"` | `"）の 2 つ以上のパターンに当たります", Fix: "どれか 1 つの [[annotation]] にだけ当たるように、パターンを直してください"` |
| `internal/annotation/annotation.go` | `"%s の %d 行目の [[annotation]] のパターンに当たるが、使うアダプタが決まっていない"` | `"%s の %d 行目の [[annotation]] のパターンに当たりますが、使うアダプタが決まっていません"` |
| `internal/annotation/annotation.go` | `Fix: "その [[annotation]] に adapter か command を書く"` | `Fix: "その [[annotation]] に、adapter か command を書いてください"` |
| `internal/annotation/annotation.go` | `"ファイルを読めない（" + err.Error() + "）", Fix: "ファイルを読む権限を確かめる"` | `"ファイルを読めません（" + err.Error() + "）", Fix: "ファイルを読む権限を確かめてください"` |
| `internal/annotation/annotation.go` | `" のアダプタのファイルには書けない", Fix: "書けるのは " + strings.Join(ks, "・") + "。種類を直すか、アノテーションを別のファイルに移す"` | `" のアダプタのファイルには書けません", Fix: "書けるのは " + strings.Join(ks, "・") + " です。種類を直すか、アノテーションを別のファイルに移してください"` |
| `internal/annotation/annotation.go` | `"1 行に @vanda: が 2 つ以上ある", Fix: "アノテーションを 1 行に 1 つずつ書く"` | `"1 行に @vanda: が 2 つ以上あります", Fix: "アノテーションを、1 行に 1 つずつ書いてください"` |
| `internal/annotation/annotation.go` | `" は決まった種類でない", Fix: "@vanda: の後を " + strings.Join(Kinds, "・") + " のどれかにする"` | `" は決まった種類ではありません", Fix: "@vanda: の後を、" + strings.Join(Kinds, "・") + " のどれかにしてください"` |
| `internal/annotation/annotation.go` | `"@vanda:internal に理由がない", Fix: "@vanda:internal <仕様の中だけで使う理由> のように書く"` | `"@vanda:internal に理由がありません", Fix: "@vanda:internal <仕様の中だけで使う理由> のように書いてください"` |
| `internal/annotation/annotation.go` | `" に ID がない", Fix: "@vanda:" + kind + " <ID> のように書く"` | `" に ID がありません", Fix: "@vanda:" + kind + " <ID> のように書いてください"` |
| `internal/annotation/annotation.go` | `" に ID が 2 つ以上ある", Fix: "1 つのアノテーションには ID を 1 つだけ書き、複数の ID は行を分けて書く"` | `" に ID が 2 つ以上あります", Fix: "1 つのアノテーションには ID を 1 つだけ書き、複数の ID は行を分けて書いてください"` |
| `internal/annotation/adapters.go` | `"Go として読めない（" + list[0].Msg + "）", Fix: "Go の書き方に合わせて直す"` | `"Go として読めません（" + list[0].Msg + "）", Fix: "Go の書き方に合わせて直してください"` |
| `internal/annotation/adapters.go` | `"Go として読めない（" + err.Error() + "）", Fix: "Go の書き方に合わせて直す"` | `"Go として読めません（" + err.Error() + "）", Fix: "Go の書き方に合わせて直してください"` |
| `internal/annotation/adapters.go` | `"アノテーションが func の宣言の直前のコメントにない"` | `"アノテーションが、func の宣言の直前のコメントにありません"` |
| `internal/annotation/adapters.go` | `"付けたい func の宣言のすぐ上のコメント（空行をはさまない）に移す"` | `"付けたい func の宣言のすぐ上のコメント（空行をはさまない）に移してください"` |
| `internal/annotation/adapters.go` | `"アノテーションが、最初のファイルの区切り（%d 行目）より後にある", first), Fix: "vanda の不具合なので、知らせる"` | `"アノテーションが、最初のファイルの区切り（%d 行目）より後にあります", first), Fix: "vanda の不具合です。開発者に知らせてください"` |
| `internal/annotation/adapters.go` | `"アノテーションの後に定義がない", Fix: "アノテーションを、付けたい定義のすぐ上に書く"` | `"アノテーションの後に、定義がありません", Fix: "アノテーションを、付けたい定義のすぐ上に書いてください"` |
| `internal/annotation/adapters.go` | `"アノテーションの次の行（%d 行目）が、Name ==・Name(…) ==・Name[…] == の定義でない", next), Fix: "アノテーションを、付けたい定義のすぐ上に書く"` | `"アノテーションの次の行（%d 行目）が、Name ==・Name(…) ==・Name[…] == の定義ではありません", next), Fix: "アノテーションを、付けたい定義のすぐ上に書いてください"` |
| `internal/annotation/adapters.go` | `"アノテーションの後に、コメントでない行がない", Fix: "アノテーションを、付けたい定義のすぐ上に書く"` | `"アノテーションの後に、コメントでない行がありません", Fix: "アノテーションを、付けたい定義のすぐ上に書いてください"` |
| `internal/annotation/adapters.go` | `"アダプタに渡す JSON を作れない（" + err.Error() + "）", Fix: "vanda の不具合なので、知らせる"` | `"アダプタに渡す JSON を作れません（" + err.Error() + "）", Fix: "vanda の不具合です。開発者に知らせてください"` |
| `internal/annotation/adapters.go` | `"）が失敗した（" + msg + "）", Fix: "アダプタのエラーに従って直す"` | `"）が失敗しました（" + msg + "）", Fix: "アダプタが出したエラーに従って、直してください"` |
| `internal/annotation/adapters.go` | `"）の出力が、決まった形の JSON でない（" + err.Error() + "）"` | `"）の出力が、決まった形の JSON ではありません（" + err.Error() + "）"` |
| `internal/annotation/adapters.go` | `"アダプタ（%s）が、アノテーションのない %d 行目の名前を返した"` | `"アダプタ（%s）が、アノテーションのない %d 行目の名前を返しました"` |
| `internal/annotation/adapters.go` | `"）が空の名前を返した", Fix: "名前を返すように、アダプタを直す"` | `"）が空の名前を返しました", Fix: "名前を返すように、アダプタを直してください"` |
| `internal/annotation/adapters.go` | `"）が同じ行の名前を 2 つ以上返した", Fix: "1 行に 1 つの名前を返すように、アダプタを直す"` | `"）が同じ行の名前を 2 つ以上返しました", Fix: "1 行に 1 つの名前を返すように、アダプタを直してください"` |
| `internal/annotation/adapters.go` | `"）が、このアノテーションの名前を返さなかった", Fix: "渡された行のすべてに名前を返すように、アダプタを直す"` | `"）が、このアノテーションの名前を返しませんでした", Fix: "渡された行のすべてに名前を返すように、アダプタを直してください"` |
| `internal/guardrail/guardrail.go` | `"ガードレールの定義のファイルがない", Fix: "ファイルを作るか、.vanda/config.toml の [[guardrail]] の paths を直す"` | `"ガードレールの定義のファイルがありません", Fix: "ファイルを作るか、.vanda/config.toml の [[guardrail]] の paths を直してください"` |
| `internal/check/links.go` | `"にない", Fix: "約束リストにある" + kindName[want] + "の ID にするか、約束リストに足す"` | `"にありません", Fix: "約束リストにある" + kindName[want] + "の ID にするか、約束リストに足してください"` |
| `internal/check/links.go` | `"の ID でない", Fix: "@vanda:" + a.Kind + " には" + kindName[want] + "の ID を書く"` | `"の ID ではありません", Fix: "@vanda:" + a.Kind + " には、" + kindName[want] + "の ID を書いてください"` |
| `internal/check/links.go` | `"（TypeOK で使っている）に @vanda:feature が付いている", Fix: "型には @vanda:term <用語の ID> か @vanda:internal <理由> を付け、@vanda:feature は操作の定義に付ける"` | `"（TypeOK で使っています）に、@vanda:feature が付いています", Fix: "型には @vanda:term <用語の ID> か @vanda:internal <理由> を付け、@vanda:feature は操作の定義に付けてください"` |
| `internal/check/links.go` | `"（TypeOK で使っている）に @vanda:term も @vanda:internal もない", Fix: "定義のすぐ上のコメントに @vanda:term <用語の ID> か @vanda:internal <理由> を書く"` | `"（TypeOK で使っています）に、@vanda:term も @vanda:internal もありません", Fix: "定義のすぐ上のコメントに、@vanda:term <用語の ID> か @vanda:internal <理由> を書いてください"` |
| `internal/check/others.go` | `"コミットの一覧を読めない（" + err.Error() + "）", Fix: "git の状態を確かめる"` | `"コミットの一覧を読めません（" + err.Error() + "）", Fix: "git の状態を確かめてください"` |
| `internal/check/others.go` | `" で変わったファイルを読めない（" + err.Error() + "）", Fix: "git の状態を確かめる"` | `" で変わったファイルを読めません（" + err.Error() + "）", Fix: "git の状態を確かめてください"` |
| `internal/check/others.go` | `" のメッセージを読めない（" + err.Error() + "）", Fix: "git の状態を確かめる"` | `" のメッセージを読めません（" + err.Error() + "）", Fix: "git の状態を確かめてください"` |
| `internal/check/others.go` | `"コミット %s「%s」が約束リスト（%s）を変えているのに、メッセージにストーリーの ID（%s の形）がない"` | `"コミット %s「%s」が約束リスト（%s）を変えていますが、メッセージにストーリーの ID（%s の形）がありません"` |
| `internal/check/others.go` | `Fix: "そのコミットのメッセージに、ストーリーの ID を書く"` | `Fix: "そのコミットのメッセージに、ストーリーの ID を書いてください"` |
| `internal/check/others.go` | `" の指紋を作れない（"+err.Error()+"）", "約束リストに書ける値（文字列・真偽・配列・表）だけにする"` | `" の指紋を作れません（"+err.Error()+"）", "約束リストに書ける値（文字列・真偽・配列・表）だけにしてください"` |
| `internal/check/others.go` | `" の今の中身に合意していない（提案中。指紋 "+fp+"）", "中身を確かめて合意し、合意の記録に書く"` | `" の今の中身に、まだ合意していません（提案中。指紋 "+fp+"）", "中身を確かめてから合意し、合意の記録に書いてください"` |
| `internal/check/others.go` | `" の合意が外れている（前に合意した後に中身が変わり、今の中身 "+fp+" に "+e.story+" の合意がない）", "今の中身を確かめ、ストーリー "+e.story+" で合意し直す"` | `" の合意が外れています（前に合意した後に中身が変わり、今の中身 "+fp+" に "+e.story+" の合意がありません）", "今の中身を確かめ、ストーリー "+e.story+" で合意し直してください"` |
| `internal/check/others.go` | `"）が残っている", "合意するときにフラグを外す（合意の後にフラグだけを立てたなら、中身を確かめ直して外す）"` | `"）が残っています", "合意するときに、フラグを外してください（合意の後にフラグだけを立てたなら、中身を確かめ直してから外してください）"` |
| `internal/check/others.go` | `" に推測（guesses）が残っている", "推測を確かめ、合意するときに guesses を外す"` | `" に推測（guesses）が残っています", "推測を確かめ、合意するときに guesses を外してください"` |
| `internal/check/others.go` | `"ファイルを読めない（" + err.Error() + "）", Fix: "ファイルを読む権限を確かめる"` | `"ファイルを読めません（" + err.Error() + "）", Fix: "ファイルを読む権限を確かめてください"` |
| `internal/check/others.go` | `"ファイルの今の中身に合意していない（提案中。指紋 " + fp + "）", Fix: "中身を確かめて合意し、合意の記録に書く"` | `"ファイルの今の中身に、まだ合意していません（提案中。指紋 " + fp + "）", Fix: "中身を確かめてから合意し、合意の記録に書いてください"` |
| `internal/check/others.go` | `" の合意が外れている（前に合意した後に中身が変わり、今の中身 " + fp + " に " + e.story + " の合意がない）", Fix: "今の中身を確かめ、ストーリー " + e.story + " で合意し直す"` | `" の合意が外れています（前に合意した後に中身が変わり、今の中身 " + fp + " に " + e.story + " の合意がありません）", Fix: "今の中身を確かめ、ストーリー " + e.story + " で合意し直してください"` |
| `internal/check/others.go` | `" を指している", "約束リストに戻すか、この合意の記録を消す"` | `" を指しています", "約束リストに戻すか、この合意の記録を消してください"` |
| `internal/check/others.go` | `"タイプ proof は段 1 では扱えない（対応するアダプタがない）", "別のタイプのエビデンスにする"` | `"タイプ proof は、まだ扱えません（対応するアダプタがありません）", "別のタイプのエビデンスにしてください"` |
| `internal/check/others.go` | `"約束 %s の %s のエビデンスがない（%s のアダプタのファイルに @vanda:%s %s がない）"` | `"約束 %s の %s のエビデンスがありません（%s のアダプタのファイルに、@vanda:%s %s がありません）"` |
| `internal/check/others.go` | `"エビデンスを作り、@vanda:%s %s を付ける"` | `"エビデンスを作り、@vanda:%s %s を付けてください"` |
| `internal/check/others.go` | `".vanda/ にあるのに、設定のどのパターンにも当たらず、読まれていない", Fix: ".vanda/config.toml の promises か agreements のパターンに当たるようにするか、ファイルを消す"` | `".vanda/ にありますが、設定のどのパターンにも当たらないため、読まれていません", Fix: ".vanda/config.toml の promises か agreements のパターンに当たるようにするか、ファイルを消してください"` |
| `internal/check/run.go` | `"リポジトリのファイルの一覧を作れない（%v）。ファイルを読む権限を確かめる"` | `"リポジトリのファイルの一覧を作れません（%v）。ファイルを読む権限を確かめてください。"` |
| `internal/check/run.go` | `ˋ審査の設定に scope = "story" の審査（%s）があるので、ベースブランチが要る。--base <ブランチ> を渡すˋ` | `ˋ審査の設定に scope = "story" の審査（%s）があるため、ベースブランチが必要です。--base <ブランチ> を渡してください。ˋ` |
| `internal/check/run.go` | `"ベースブランチ %s から分かれた点が分からない（%v）。--base に、今のブランチと履歴のつながったブランチを渡す"` | `"ベースブランチ %s から分かれた点が分かりません（%v）。--base に、今のブランチと履歴のつながったブランチを渡してください。"` |
| `internal/check/run.go` | `"--story %s が、ストーリーの ID の書き方（%s）に合わない。設定の id_pattern に合う ID を渡す"` | `"--story %s が、ストーリーの ID の書き方（%s）に合いません。設定の id_pattern に合う ID を渡してください。"` |
| `internal/check/run.go` | `"コミットのメッセージを読めない（%v）"` | `"コミットのメッセージを読めません（%v）。"` |
| `internal/check/run.go` | `"範囲 story の審査（%s）のストーリーが分からない（分かれた点より後のコミットのメッセージに、ストーリーの ID がない）。--story <ID> を渡す"` | `"範囲 story の審査（%s）のストーリーが分かりません（分かれた点より後のコミットのメッセージに、ストーリーの ID がありません）。--story <ID> を渡してください。"` |
| `internal/check/run.go` | `"範囲 story の審査（%s）のストーリーが決まらない（分かれた点より後のコミットのメッセージに、ストーリーの ID が %s とある）。--story <ID> で 1 つを渡す"` | `"範囲 story の審査（%s）のストーリーが 1 つに決まりません（分かれた点より後のコミットのメッセージに、ストーリーの ID が %s とあります）。--story <ID> で 1 つを渡してください。"` |
| `internal/check/run.go` | `"審査 " + s.Name + " はない", Fix: "vanda の不具合なので、知らせる"` | `"審査 " + s.Name + " はありません", Fix: "vanda の不具合です。開発者に知らせてください"` |
| `internal/check/run.go` | `"%sが読めないため行えない（誤りが %d 件ある）", what, n), Fix: "審査 " + owner + " で誤りを出して直す"` | `"%sを読めないため、この審査を行えません（誤りが %d 件あります）", what, n), Fix: "審査 " + owner + " で誤りを確かめて、直してください"` |
| `internal/check/shape.go` | `" が決まった形でない", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: task-id）にする"` | `" が決まった形ではありません", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: task-id）にしてください"` |
| `internal/check/shape.go` | `"ID %s が、%s の%s（%s）と重なっている", quote(it.id), place(prev), kindName[prev.kind], prev.id), "用語・機能・約束のどれとも重ならない ID にする"` | `"ID %s が、%s の%s（%s）と重なっています", quote(it.id), place(prev), kindName[prev.kind], prev.id), "用語・機能・約束のどれとも重ならない ID にしてください"` |
| `internal/check/shape.go` | `" が用語にない"+notKind(k), "part_of に、約束リストにある用語の ID を書く"` | `" が用語にありません"+notKind(k), "part_of に、約束リストにある用語の ID を書いてください"` |
| `internal/check/shape.go` | `"コア機能で formal = false なのに、形式手法を使わない理由（formal_waiver）がない", ˋformal_waiver = "…" に理由を書くか、formal = true にするˋ` | `"コア機能で formal = false なのに、形式手法を使わない理由（formal_waiver）がありません", ˋformal_waiver = "…" に理由を書くか、formal = true にしてくださいˋ` |
| `internal/check/shape.go` | `"formal_waiver を書けるのは core = true かつ formal = false のときだけ", "formal_waiver を消す"` | `"formal_waiver を書けるのは、core = true かつ formal = false のときだけです", "formal_waiver を消してください"` |
| `internal/check/shape.go` | `" が用語にも機能にもない"+notKind(k), "subjects に、約束リストにある用語か機能の ID を書く"` | `" が用語にも機能にもありません"+notKind(k), "subjects に、約束リストにある用語か機能の ID を書いてください"` |
| `internal/check/shape.go` | `" が設定にない", "layers を "+orNone(layerIDs)+" から選ぶか、.vanda/config.toml の [[layer]] に足す"` | `" が設定にありません", "layers を "+orNone(layerIDs)+" から選ぶか、.vanda/config.toml の [[layer]] に足してください"` |
| `internal/check/shape.go` | `" は決まった値でない", "flags に書けるのは ai-proposal だけ"` | `" は決まった値ではありません", "flags には ai-proposal だけを書いてください"` |
| `internal/check/shape.go` | `" は決まった値でない", "stage を "+strings.Join(Stages, "・")+" のどれかにする"` | `" は決まった値ではありません", "stage を "+strings.Join(Stages, "・")+" のどれかにしてください"` |
| `internal/check/shape.go` | `" は決まった値でない", "type を "+strings.Join(Types, "・")+" のどれかにする"` | `" は決まった値ではありません", "type を "+strings.Join(Types, "・")+" のどれかにしてください"` |
| `internal/check/shape.go` | `" が、約束の layers にない", "エビデンスの layers を、約束の layers（"+strings.Join(c.Layers, "・")+"）から選ぶ"` | `" が、約束の layers にありません", "エビデンスの layers を、約束の layers（"+strings.Join(c.Layers, "・")+"）から選んでください"` |
| `internal/check/shape.go` | `" を確かめるエビデンスがない", "layers に "+l+" を含む [[claim.evidence]] を足す"` | `" を確かめるエビデンスがありません", "layers に "+l+" を含む [[claim.evidence]] を足してください"` |
| `internal/check/shape.go` | `"part_of が循環している（"+strings.Join(path, " → ")+"）", "どれかの part_of を消すか、別の用語にする"` | `"part_of が循環しています（"+strings.Join(path, " → ")+"）", "どれかの part_of を消すか、別の用語にしてください"` |
| `internal/check/shape.go` | `"（設定にレイヤーがない）"` | `"（設定にレイヤーがありません）"` |
| `internal/check/spec.go` | `"審査が 1 つもない", "[[check]] に name と scope を書く"` | `"審査が 1 つもありません", "[[check]] に name と scope を書いてください"` |
| `internal/check/spec.go` | `" はない", "name を "+strings.Join(names, "・")+" のどれかにする"` | `" はありません", "name を "+strings.Join(names, "・")+" のどれかにしてください"` |
| `internal/check/spec.go` | `" は決まった値でない", ˋscope を "all" か "story" にするˋ` | `" は決まった値ではありません", ˋscope を "all" か "story" にしてくださいˋ` |
| `internal/check/spec.go` | `" で行えない", "scope を "+strings.Join(scopes, " か ")+" にする"` | `" では行えません", "scope を "+strings.Join(scopes, " か ")+" にしてください"` |
| `internal/repo/repo.go` | `"git のリポジトリの外で動かしている（" + err.Error() + "）", Fix: "vanda は、.vanda/ のある git のリポジトリの中で動かす"` | `"git のリポジトリの外で動かしています（" + err.Error() + "）", Fix: "vanda は、.vanda/ のある git のリポジトリの中で動かしてください"` |
| `cmd/vanda/main.go` | `"エビデンス・ファースト開発の道具一式"` | `"エビデンス・ファースト開発の道具一式です"` |
| `cmd/vanda/main.go` | `errors.New("審査が失敗した")` | `errors.New("審査が失敗しました")` |
| `cmd/vanda/main.go` | `Short: "審査を行う"` | `Short: "審査を行います"` |
| `cmd/vanda/main.go` | `"審査の設定のファイルを 1 つ渡す（例: vanda check .vanda/checks/push.toml）。既定の審査はない"` | `"審査の設定のファイルがありません。審査の設定のファイルを 1 つ渡してください（例: vanda check .vanda/checks/push.toml。既定の審査はありません）。"` |
| `cmd/vanda/main.go` | `": 審査の設定のファイルを読めない（" + err.Error() + "）。今のディレクトリからの場所を確かめる"` | `": 審査の設定のファイルを読めません（" + err.Error() + "）。今のディレクトリからの場所を確かめてください。"` |
| `cmd/vanda/main.go` | `ˋベースブランチ（scope = "story" の審査があるときに要る）ˋ` | `ˋベースブランチ（scope = "story" の審査があるときに必要です）ˋ` |
| `cmd/vanda/main.go` | `"ストーリーの ID（コミットのメッセージから決まらないときに渡す）"` | `"ストーリーの ID（コミットのメッセージから決まらないときに渡します）"` |
| `cmd/vanda/main.go` | `Short: "版を出す"` | `Short: "版を表示します"` |
| `cmd/vanda/main.go` | `Short: "設定と版を確かめる"` | `Short: "設定と版を確かめます"` |
| `cmd/vanda/main.go` | `"設定を確かめた: %s（リポジトリ %s、版 %s）%s\n"` | `"設定を確かめました: %s（リポジトリ %s、版 %s）%s\n"` |
| `cmd/vanda/main.go` | `"今のディレクトリが分からない（" + err.Error() + "）"` | `"今のディレクトリが分かりません（" + err.Error() + "）。"` |
| `internal/version/version.go` | `"版 %q が major.minor.patch の形でない", s)` | `"版 %q が major.minor.patch の形ではありません", s)` |
| `internal/version/version.go` | `"版 %q が major.minor.patch の形でない", s)` | `"版 %q が major.minor.patch の形ではありません", s)` |
| `internal/annotation/adapters.go` | `Fix: "渡された行の名前だけを返すように、アダプタを直す"` | `Fix: "渡された行の名前だけを返すように、アダプタを直してください"` |
| `internal/snapshot/snapshot.go` | `"リポジトリのファイルの一覧を作れない（%v）。ファイルを読む権限を確かめる"` | `"リポジトリのファイルの一覧を作れません（%v）。ファイルを読む権限を確かめてください。"` |
| `internal/snapshot/snapshot.go` | `"コミット %s のファイルの一覧を読めない（%v）"` | `"コミット %s のファイルの一覧を読めません（%v）。"` |
| `internal/snapshot/snapshot.go` | `"コミット %s の %s を読めない（%v）"` | `"コミット %s の %s を読めません（%v）。"` |
| `internal/snapshot/snapshot.go` | `"%s がない", path` | `"%s がありません", path` |

- 表のほかに、誤りの中に入る次の語も変わる: `（<種類>の ID）`（`notKind`）はそのまま。`（設定にレイヤーがない）` → `（設定にレイヤーがありません）`
- ˋ は、ソースのバッククォート（Go の生の文字列）を表す

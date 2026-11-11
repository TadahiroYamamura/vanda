# Git と GitHub

このリポジトリで Git と GitHub を使う時に、いつも守る決まり。ブランチの作り方・コミット・プッシュ・片付けの手順は、スキル git-workflow に従う。グローバルの Git の決まり（コミットやプッシュは指示があった時だけ、など）より、こちらとスキル git-workflow を優先する。

- `main` への操作（マージ、コミット、プッシュ、書き換え、削除）は、どんな場合もユーザーの承認を得てから行う
- コミットの作者を `git -c user.name=…`・`git -c user.email=…` や環境の変数で指定しない。リポジトリのローカルの設定（なければグローバルの設定）をそのまま使う
- `gh` は、`git config github.user` の GitHub のユーザーのトークンで使う（`GH_TOKEN=$(gh auth token --user "$(git config github.user)") gh …`）。`github.user` がない時や、トークンを取れない時は、`gh` の既定のアカウントで続けず、止めてユーザーに聞く
- 作業ブランチ（`story-<番号>/…`）では、`push --force`・`rebase`・`commit --amend` などの履歴の書き換えをしてよい

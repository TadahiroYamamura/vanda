package main_test

import (
	"os"
	"path/filepath"
	"strings"
)

// storyMessage は、コミットのメッセージを作る。story が空でなければ、末尾に Story: のトレーラーを書く
// （vanda はストーリーの ID をトレーラーからだけ読み、subject と本文の ID は数えない。docs/formats.md の「コミットのストーリーの ID」）。
func storyMessage(subject, story string) string {
	if story == "" {
		return subject
	}
	return subject + "\n\nStory: " + story
}

// commitAll は、作業ツリーのすべて（git が追っていないファイルも）を 1 つのコミットにする。
// 同じ大きさのファイルを同じ時刻のうちに書き換えると、git add -A が stat の一致から変わっていないとみなし、
// 書き換えをコミットに入れないことがある。索引から一度外して、毎回すべてを読み直させる
func (g *rvGit) commitAll(subject, story string) {
	g.run("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
	g.run("add", "-A")
	g.run("commit", "-q", "--allow-empty", "-m", storyMessage(subject, story))
}

// commitTracked は、git が追っているファイルの変更だけを 1 つのコミットにする（git commit -a に当たる。追っていないファイルは残す）。
// 読み直させる理由は commitAll と同じ
func (g *rvGit) commitTracked(subject, story string) {
	var keep []string
	for _, f := range strings.Split(g.run("ls-files", "-z"), "\x00") {
		if f == "" {
			continue
		}
		// 作業ツリーで消したものは、索引から外したままにして、消したことをコミットする
		if _, err := os.Lstat(filepath.Join(g.root, filepath.FromSlash(f))); err == nil {
			keep = append(keep, f)
		}
	}
	g.run("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
	if len(keep) > 0 {
		g.run(append([]string{"add", "-f", "--"}, keep...)...)
	}
	g.run("commit", "-q", "--allow-empty", "-m", storyMessage(subject, story))
}

package main_test

import (
	"os"
	"path/filepath"
	"testing"
)

// RV-01・RV-13 の、手で作った例。操作の列と、期待する一覧を、手で書く。
// テストの中の数え方（rvBranchView・rvStoryView）が、この例と合うことを確かめる。
// API ができたら、同じ例を vanda の一覧と比べる。

type rvExample struct {
	name   string
	build  func(r *rvRepo)
	branch map[string][]rvEntry // ベースブランチ → レビュー単位がブランチのときの一覧
	story  map[string][]rvEntry // ストーリーの ID → レビュー単位がストーリーのときの一覧
}

func pickFirst(int) int { return 0 }

// pickCycle は、毎回違う選び方をする。書き方だけを変えて書き直すのに使う。
func pickCycle() rvPick {
	k := 0
	return func(n int) int {
		k++
		return k % n
	}
}

var rvExamples = []rvExample{
	{
		name: "追加・変更・削除のそれぞれが1度ずつ出て名前の変更は削除と追加の2件になり変わっていないものもコミットしていない書きかけの変更も出ない",
		build: func(r *rvRepo) {
			r.addItem("claim", "c-keep", 0)
			r.addItem("claim", "c-change", 1)
			r.addItem("term", "t-del", 0)
			r.addItem("feature", "f-change", 2)
			r.addItem("term", "t-keep", 1)
			r.addItem("feature", "f-keep", 0)
			r.addFileAt(rvScenario, "e2e/keep.txtar")
			ren := r.addFileAt(rvScenario, "e2e/old.txtar")
			r.addFileAt(rvGuardrail, "db/schema.sql")
			m := r.addFileAt(rvTLA, "spec/Model.tla")
			del := r.addFileAt(rvGuardrail, "db/old.sql")
			r.addFileAt(rvTLA, "spec/Keep.tla")
			r.commit("#1")

			r.newBranch("story-24")
			r.changeItem("c-change", "condition")
			r.deleteItem("t-del")
			r.changeItem("f-change", "definition")
			r.renameFileTo(ren, "e2e/sub/new-name.txtar")
			r.addFileAt(rvNoise, "notes/memo.md")
			r.commit("#24")
			// ここから下はコミットしない（作業ツリーの書きかけの変更と、まだ git が追っていないファイル）。一覧には出ない
			r.changeFile(m)
			r.addItem("claim", "c-new", 2)
			r.addFileAt(rvScenario, "e2e/new.txtar")
			r.deleteFile(del)
		},
		branch: map[string][]rvEntry{"main": {
			{Kind: "claim", Key: "c-change", Change: "modified"},
			{Kind: "term", Key: "t-del", Change: "deleted"},
			{Kind: "feature", Key: "f-change", Change: "modified"},
			{Kind: "file", Key: "e2e/old.txtar", Change: "deleted"},
			{Kind: "file", Key: "e2e/sub/new-name.txtar", Change: "added"},
		}},
	},
	{
		name: "約束の文が変わらずシナリオのファイルだけが変わったものも出る",
		build: func(r *rvRepo) {
			r.addItem("claim", "c-a", 0)
			s := r.addFileAt(rvScenario, "e2e/a.txtar")
			r.commit("#1")
			r.newBranch("story-24")
			r.changeFile(s)
			r.commit("#24")
		},
		branch: map[string][]rvEntry{"main": {
			{Kind: "file", Key: "e2e/a.txtar", Change: "modified"},
		}},
		story: map[string][]rvEntry{"#24": {
			{Kind: "file", Key: "e2e/a.txtar", Change: "modified"},
		}},
	},
	{
		name: "項目の順や空白や引用符やファイルの分け方だけを変えたものは出ない",
		build: func(r *rvRepo) {
			r.addItem("claim", "c-a", 0)
			r.addItem("claim", "c-b", 0)
			r.addItem("term", "t-a", 1)
			r.addItem("feature", "f-a", 2)
			r.commit("#1")
			before := readPromises(r)
			r.newBranch("story-24")
			r.pick = pickCycle()
			r.reformat()
			r.moveItem("c-b", 2)
			r.moveItem("t-a", 0)
			r.commit("#24")
			if after := readPromises(r); after == before {
				r.t.Fatalf("書き方を変えたつもりが、約束リストのファイルが変わっていない")
			}
		},
		branch: map[string][]rvEntry{"main": nil},
		story:  map[string][]rvEntry{"#24": nil},
	},
	{
		name: "レビュー単位がブランチのときはベースブランチと分かれた点との差分でベースブランチを変えると一覧も変わる",
		build: func(r *rvRepo) {
			r.addItem("claim", "c-a", 0)
			r.addItem("claim", "c-b", 0)
			r.commit("#1")
			r.newBranch("release")
			r.changeItem("c-a", "statement")
			r.commit("#2")
			r.newBranch("story-24")
			r.changeItem("c-b", "rationale")
			r.commit("#24")
			// 分かれた後に main で足したものは、main と比べても出ない（main の先端との差分ではない）
			r.switchTo("main")
			r.addItem("claim", "c-main", 1)
			r.commit("#3")
			r.switchTo("story-24")
		},
		branch: map[string][]rvEntry{
			"release": {
				{Kind: "claim", Key: "c-b", Change: "modified"},
			},
			"main": {
				{Kind: "claim", Key: "c-a", Change: "modified"},
				{Kind: "claim", Key: "c-b", Change: "modified"},
			},
		},
	},
	{
		name: "レビュー単位がストーリーのときは途中で足して後で消したものを出さずそのストーリーのIDのないコミットが途中で変えたものを示す",
		build: func(r *rvRepo) {
			r.addItem("claim", "c-a", 0)
			r.addItem("claim", "c-b", 0)
			s1 := r.addFileAt(rvScenario, "e2e/s1.txtar")
			s2 := r.addFileAt(rvScenario, "e2e/s2.txtar")
			r.commit("#1")
			r.newBranch("story-24")
			r.changeItem("c-a", "statement")
			r.addItem("claim", "c-tmp", 1)
			r.commit("#24")
			r.changeItem("c-a", "condition")
			r.commit("#25")
			r.deleteItem("c-tmp")
			r.renameFileTo(s2, "e2e/s2-renamed.txtar")
			r.addItem("claim", "c-new", 2)
			r.commit("#24")
			r.changeFile(s1)
			r.commit("#25")
			r.changeItem("c-new", "statement")
			r.commit("")
		},
		branch: map[string][]rvEntry{"main": {
			{Kind: "claim", Key: "c-a", Change: "modified"},
			{Kind: "claim", Key: "c-new", Change: "added"},
			{Kind: "file", Key: "e2e/s2.txtar", Change: "deleted"},
			{Kind: "file", Key: "e2e/s2-renamed.txtar", Change: "added"},
			{Kind: "file", Key: "e2e/s1.txtar", Change: "modified"},
		}},
		story: map[string][]rvEntry{
			"#24": {
				{Kind: "claim", Key: "c-a", Change: "modified", Others: "#25"},
				{Kind: "claim", Key: "c-new", Change: "added", Others: "-"},
				{Kind: "file", Key: "e2e/s2.txtar", Change: "deleted"},
				{Kind: "file", Key: "e2e/s2-renamed.txtar", Change: "added"},
			},
			"#25": {
				{Kind: "claim", Key: "c-a", Change: "modified"},
				{Kind: "file", Key: "e2e/s1.txtar", Change: "modified"},
			},
		},
	},
	{
		name: "場所と中身を両方変えたファイルとTLAの名前の変更は削除と追加の2件になり同じ場所を使い直したものは変更になる",
		build: func(r *rvRepo) {
			both := r.addFileAt(rvScenario, "e2e/both.txtar")
			tla := r.addFileAt(rvTLA, "spec/Old.tla")
			reuse := r.addFileAt(rvGuardrail, "db/reuse.sql")
			r.commit("#1")
			r.newBranch("story-24")
			r.renameFileTo(both, "e2e/both-new.txtar")
			r.changeFile(both)
			r.renameFileTo(tla, "spec/New.tla")
			r.deleteFile(reuse)
			r.commit("#24")
			r.addFileAt(rvGuardrail, "db/reuse.sql")
			r.commit("#24")
		},
		branch: map[string][]rvEntry{"main": {
			{Kind: "file", Key: "e2e/both.txtar", Change: "deleted"},
			{Kind: "file", Key: "e2e/both-new.txtar", Change: "added"},
			{Kind: "file", Key: "spec/Old.tla", Change: "deleted"},
			{Kind: "file", Key: "spec/New.tla", Change: "added"},
			{Kind: "file", Key: "db/reuse.sql", Change: "modified"},
		}},
		story: map[string][]rvEntry{"#24": {
			{Kind: "file", Key: "e2e/both.txtar", Change: "deleted"},
			{Kind: "file", Key: "e2e/both-new.txtar", Change: "added"},
			{Kind: "file", Key: "spec/Old.tla", Change: "deleted"},
			{Kind: "file", Key: "spec/New.tla", Change: "added"},
			{Kind: "file", Key: "db/reuse.sql", Change: "modified"},
		}},
	},
	{
		name: "レビュー単位がストーリーのときはIDのないコミットが名前を変えたファイルをストーリーがさらに名前を変えたものをその間の場所の削除と新しい場所の追加として示す",
		build: func(r *rvRepo) {
			g := r.addFileAt(rvGuardrail, "db/a.sql")
			r.commit("#1")
			r.renameFileTo(g, "db/b.sql")
			r.commit("")
			r.renameFileTo(g, "db/c.sql")
			r.commit("#25")
		},
		branch: map[string][]rvEntry{"main": nil},
		story: map[string][]rvEntry{
			"#25": {{Kind: "file", Key: "db/b.sql", Change: "deleted"}, {Kind: "file", Key: "db/c.sql", Change: "added"}},
			// #1 が足した db/a.sql は今ないので出ない。db/c.sql を最初に変えたのは #25
			"#1": {{Kind: "term", Key: "base", Change: "added"}},
		},
	},
	{
		name: "ストーリーが2つのコミットで2度名前を変えたファイルは最初の場所の削除と最後の場所の追加の2件になり途中の場所は出ない",
		build: func(r *rvRepo) {
			g := r.addFileAt(rvGuardrail, "db/a.sql")
			r.commit("#1")
			r.newBranch("story-25")
			r.renameFileTo(g, "db/b.sql")
			r.commit("#25")
			r.renameFileTo(g, "db/c.sql")
			r.commit("#25")
		},
		branch: map[string][]rvEntry{"main": {{Kind: "file", Key: "db/a.sql", Change: "deleted"}, {Kind: "file", Key: "db/c.sql", Change: "added"}}},
		story:  map[string][]rvEntry{"#25": {{Kind: "file", Key: "db/a.sql", Change: "deleted"}, {Kind: "file", Key: "db/c.sql", Change: "added"}}},
	},
}

func readPromises(r *rvRepo) string {
	var all string
	for f := 0; f < rvPromiseFiles; f++ {
		data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rvPromisePath(f))))
		if err != nil {
			r.t.Fatalf("%v", err)
		}
		all += string(data)
	}
	return all
}

func rvSet(es []rvEntry) map[rvEntry]bool {
	m := map[rvEntry]bool{}
	for _, e := range es {
		m[e] = true
	}
	return m
}

// RV-01・RV-13
func TestRV01の期待値の数え方は手で作った例と合う(t *testing.T) {
	t.Parallel()
	for _, ex := range rvExamples {
		t.Run(ex.name, func(t *testing.T) {
			t.Parallel()
			r := newRVRepo(t, t.TempDir(), "0.1.0", "#1", pickFirst)
			ex.build(r)
			for base, want := range ex.branch {
				if d := rvDiffEntries(rvSet(want), r.branchView(base)); d != "" {
					t.Errorf("ベースブランチ %s の、レビュー単位がブランチのときの一覧が、手で作った例と合わない\n%s", base, d)
				}
			}
			for story, want := range ex.story {
				if d := rvDiffEntries(rvSet(want), r.storyView(story)); d != "" {
					t.Errorf("ストーリー %s の、レビュー単位がストーリーのときの一覧が、手で作った例と合わない\n%s", story, d)
				}
			}
		})
	}
}

package main_test

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// rvRepo は、テストの中の状態（rvState）とコミットの履歴を持ち、操作のたびに一時的な git のリポジトリへ書き出す。
// 一覧の期待値は、ここで持つ状態から rvBranchView・rvStoryView で作る。
type rvRepo struct {
	t       fataler
	g       *rvGit
	root    string
	pick    rvPick
	commits []rvCommit
	heads   map[string]int // ブランチの名前 → commits の添え字
	branch  string
	state   rvState // 作業ツリーの今の状態
	dirty   bool    // 最後のコミットの後に、作業ツリーを書き換えたか
	seq     int     // 新しい中身と場所を作るための番号
	used    map[string]bool
}

// newRVRepo は、root に git のリポジトリを作り、main ブランチに、約束の対象に使う用語だけの最初のコミット（ストーリー firstStory）を置く。
func newRVRepo(t fataler, root, version, firstStory string, pick rvPick) *rvRepo {
	r := &rvRepo{t: t, g: newRVGit(t, root), root: root, pick: pick, heads: map[string]int{}, branch: "main",
		state: newRVState(), used: map[string]bool{}}
	r.g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", rvConfig(version))
	r.state.items[rvBaseTerm] = rvItem{kind: "term", fields: map[string]string{"name": "もと", "definition": "約束の対象"}}
	r.flush()
	r.commit(firstStory)
	return r
}

func (r *rvRepo) next() int {
	r.seq++
	return r.seq
}

func (r *rvRepo) flush() {
	writeState(r.t, r.root, r.state, r.pick)
	r.dirty = true
}

// --- 約束リストの項目 ---

func (r *rvRepo) addItem(kind, id string, file int) {
	if _, ok := r.state.items[id]; ok {
		r.t.Fatalf("ID %s はもうある", id)
	}
	n := r.next()
	f := map[string]string{}
	switch kind {
	case "term", "feature":
		f["name"] = fmt.Sprintf("名前 %d", n)
		f["definition"] = fmt.Sprintf("定義 %d", n)
	case "claim":
		f["statement"] = fmt.Sprintf("文 %d", n)
		f["rationale"] = fmt.Sprintf("理由 %d", n)
		f["condition"] = fmt.Sprintf("条件 %d", n)
		f["evidence.detail"] = fmt.Sprintf("中身 %d", n)
	default:
		r.t.Fatalf("知らない種類 %q", kind)
	}
	r.state.items[id] = rvItem{kind: kind, fields: f, file: file}
	r.flush()
}

// rvItemFields は、種類ごとの、変えられる項目。
func rvItemFields(kind string) []string {
	if kind == "claim" {
		return []string{"statement", "rationale", "condition", "evidence.detail"}
	}
	return []string{"name", "definition"}
}

func (r *rvRepo) item(id string) rvItem {
	it, ok := r.state.items[id]
	if !ok || id == rvBaseTerm {
		r.t.Fatalf("ID %s は変えられる項目でない", id)
	}
	return it
}

func (r *rvRepo) changeItem(id, field string) {
	it := r.item(id).clone()
	if _, ok := it.fields[field]; !ok {
		r.t.Fatalf("%s に項目 %s はない", id, field)
	}
	it.fields[field] = fmt.Sprintf("%s 改 %d", strings.Fields(it.fields[field])[0], r.next())
	r.state.items[id] = it
	r.flush()
}

func (r *rvRepo) deleteItem(id string) {
	r.item(id)
	delete(r.state.items, id)
	r.flush()
}

// moveItem は、項目を別の約束リストのファイルへ移す（書き方だけの違い）。
func (r *rvRepo) moveItem(id string, file int) {
	it := r.item(id).clone()
	it.file = file
	r.state.items[id] = it
	r.flush()
}

// reformat は、中身を変えずに、約束リストを別の書き方で書き直す。
func (r *rvRepo) reformat() { r.flush() }

// --- ファイル ---

func rvContent(class, p string, n int) string {
	switch class {
	case rvScenario:
		return fmt.Sprintf("# 中身 %d\n-- a.txt --\n%d\n", n, n)
	case rvTLA:
		return fmt.Sprintf("---- MODULE %s ----\nX == %d\n====\n", strings.TrimSuffix(path.Base(p), ".tla"), n)
	case rvGuardrail:
		return fmt.Sprintf("CREATE TABLE t%d (id INTEGER);\n", n)
	default:
		return fmt.Sprintf("メモ %d\n", n)
	}
}

func (r *rvRepo) freshPath(class string, n int, sub bool) string {
	switch class {
	case rvScenario:
		if sub {
			return fmt.Sprintf("e2e/sub/s%d.txtar", n)
		}
		return fmt.Sprintf("e2e/s%d.txtar", n)
	case rvTLA:
		return fmt.Sprintf("spec/M%d.tla", n)
	case rvGuardrail:
		return fmt.Sprintf("db/g%d.sql", n)
	default:
		return fmt.Sprintf("notes/n%d.md", n)
	}
}

// addFile は、新しい場所にファイルを足し、テストの中でそのファイルを見分ける番号を返す。
func (r *rvRepo) addFile(class string) int {
	return r.addFileAt(class, r.freshPath(class, r.next(), false))
}

func (r *rvRepo) pathInUse(p string) bool {
	for _, f := range r.state.files {
		if f.path == p {
			return true
		}
	}
	return false
}

// addFileAt は、場所 p にファイルを足す。前に使って今は空いている場所を使い直してもよい。
func (r *rvRepo) addFileAt(class, p string) int {
	if r.pathInUse(p) {
		r.t.Fatalf("場所 %s には、もうファイルがある", p)
	}
	r.used[p] = true
	id := r.next()
	r.state.files[id] = rvFile{class: class, path: p, content: rvContent(class, p, r.next())}
	r.flush()
	return id
}

// freePaths は、前に使って今は空いている、class の場所を、名前の順に返す。
func (r *rvRepo) freePaths(class string) []string {
	var ps []string
	for p := range r.used {
		if !r.pathInUse(p) && r.freshPathClass(p) == class {
			ps = append(ps, p)
		}
	}
	sort.Strings(ps)
	return ps
}

func (r *rvRepo) freshPathClass(p string) string {
	switch {
	case strings.HasPrefix(p, "e2e/"):
		return rvScenario
	case strings.HasPrefix(p, "spec/"):
		return rvTLA
	case strings.HasPrefix(p, "db/"):
		return rvGuardrail
	default:
		return rvNoise
	}
}

func (r *rvRepo) file(n int) rvFile {
	f, ok := r.state.files[n]
	if !ok {
		r.t.Fatalf("ファイル %d はない", n)
	}
	return f
}

func (r *rvRepo) changeFile(n int) {
	f := r.file(n)
	f.content = rvContent(f.class, f.path, r.next())
	r.state.files[n] = f
	r.flush()
}

func (r *rvRepo) renameFile(n int, sub bool) string {
	return r.renameFileTo(n, r.freshPath(r.file(n).class, r.next(), sub))
}

// renameFileTo は、ファイルの場所だけを変える。TLA+ は、モジュールの名前をファイルの名前に合わせるので、中身も変わる。
func (r *rvRepo) renameFileTo(n int, p string) string {
	f := r.file(n)
	if r.pathInUse(p) {
		r.t.Fatalf("場所 %s には、もうファイルがある", p)
	}
	r.used[p] = true
	if f.class == rvTLA {
		f.content = strings.Replace(f.content, "MODULE "+strings.TrimSuffix(path.Base(f.path), ".tla"),
			"MODULE "+strings.TrimSuffix(path.Base(p), ".tla"), 1)
	}
	f.path = p
	r.state.files[n] = f
	r.flush()
	return p
}

func (r *rvRepo) deleteFile(n int) {
	r.file(n)
	delete(r.state.files, n)
	r.flush()
}

// --- コミットとブランチ ---

// commit は、作業ツリーのすべてをコミットする。story が空なら、メッセージにストーリーの ID を入れない。
func (r *rvRepo) commit(story string) {
	// メッセージを毎回違うものにして、git の履歴で見分けやすくする
	r.g.commitAll(fmt.Sprintf("中身を変える（%d）", r.next()), story)
	parent := -1
	if h, ok := r.heads[r.branch]; ok {
		parent = h
	}
	r.commits = append(r.commits, rvCommit{parent: parent, story: story, state: r.state.clone(), hash: r.g.run("rev-parse", "HEAD")})
	r.heads[r.branch] = len(r.commits) - 1
	r.dirty = false
}

// newBranch は、今のコミットから新しいブランチを作り、そこへ移る。書きかけの変更は持っていく。
func (r *rvRepo) newBranch(name string) {
	r.g.run("switch", "-q", "-c", name)
	r.heads[name] = r.heads[r.branch]
	r.branch = name
}

func (r *rvRepo) switchTo(name string) {
	if r.dirty {
		r.t.Fatalf("書きかけの変更があるので、%s へ移らない", name)
	}
	r.g.run("switch", "-q", name)
	r.branch = name
	r.state = r.commits[r.heads[name]].state.clone()
	verifyWorkTree(r.t, r.root, r.state)
}

func (r *rvRepo) ancestors(c int) []int {
	var xs []int
	for ; c >= 0; c = r.commits[c].parent {
		xs = append(xs, c)
	}
	return xs
}

// mergeBase は、ベースブランチ base と今のブランチが分かれた点を、テストの中の履歴から決め、git の答えと合うかも確かめる。
func (r *rvRepo) mergeBase(base string) rvCommit {
	inBase := map[int]bool{}
	for _, c := range r.ancestors(r.heads[base]) {
		inBase[c] = true
	}
	for _, c := range r.ancestors(r.heads[r.branch]) {
		if inBase[c] {
			if got := r.g.run("merge-base", base, "HEAD"); got != r.commits[c].hash {
				r.t.Fatalf("分かれた点が合わない: git は %s、テストの中の履歴は %s", got, r.commits[c].hash)
			}
			return r.commits[c]
		}
	}
	r.t.Fatalf("%s と %s に分かれた点がない", base, r.branch)
	return rvCommit{}
}

// head は、今のブランチの HEAD のコミット。
func (r *rvRepo) head() rvCommit { return r.commits[r.heads[r.branch]] }

// branchView は、ベースブランチ base での、レビュー単位がブランチのときの期待値。分かれた点の中身と HEAD のコミットの中身を比べ、
// 作業ツリーの書きかけの変更（コミットしていないもの）は含めない（約束 review-view-branch）。
func (r *rvRepo) branchView(base string) map[rvEntry]bool {
	mb := r.mergeBase(base)
	verifyCommit(r.g, mb.hash, mb.state)
	head := r.head()
	verifyCommit(r.g, head.hash, head.state)
	verifyWorkTree(r.t, r.root, r.state)
	return rvBranchView(mb.state, head.state)
}

// storyView は、ストーリー story の、レビュー単位がストーリーのときの期待値。
// 書きかけの変更をストーリーのものとみなすかを仕様から決められないので、作業ツリーに書きかけの変更がないときだけ作る。
func (r *rvRepo) storyView(story string) map[rvEntry]bool {
	if r.dirty {
		r.t.Fatalf("書きかけの変更があるときは、レビュー単位がストーリーのときの期待値を作らない")
	}
	anc := r.ancestors(r.heads[r.branch])
	chain := make([]rvCommit, len(anc))
	for i, c := range anc {
		chain[len(anc)-1-i] = r.commits[c]
	}
	verifyWorkTree(r.t, r.root, r.state)
	return rvStoryView(chain, r.state, story)
}

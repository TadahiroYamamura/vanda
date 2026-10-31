package main_test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// RV-01 の一覧の期待値を、vanda に頼らずテストの中で数える仕組み。
// 約束リストの項目とファイルの中身を、テストの中の状態（rvState）として持ち、それを git のリポジトリに書き出す。
// 期待する一覧は、書き出す前の状態どうしを比べて作る。リポジトリに書いた TOML やファイルを読み直して作らないのは、
// 読み込みや差分の取り方を実装と同じ作りにすると、実装と同じ誤りに気づけないため。
//
// 期待値を仕様から決められない組み合わせは、操作の側で起こさない（rvRepo の各操作の説明に書く）。
// - ファイルの名前を変えたうえで中身も変える: 「名前の変更」か「削除と追加」か「変更」かを、仕様から決められない。
//   ファイルごとに、名前だけを変えるものか、中身だけを変えるものかを、足したときに決める
// - TLA+ のファイルの名前の変更: モジュールの名前が変わり中身も変わる（docs/spec.md）ので、上と同じ理由で起こさない
// - 消したファイルや名前を変える前の場所を、別のファイルで使い直す: 場所で追うか中身で追うかで答えが変わる
// - 中身の同じファイルを 2 つ置く: ファイルは中身の指紋で見分けるので、どれの名前の変更かが決まらない
// - フラグ（flags・guesses）だけを変える: 指紋には含めないが、一覧に出すかを仕様から決められない

// rvEntry は、一覧の 1 件の期待値。
type rvEntry struct {
	Kind    string // claim・term・feature・file
	Key     string // 約束・用語・機能は ID。ファイルは今の場所（削除なら、前の場所）
	Change  string // added・changed・deleted・renamed
	OldPath string // 名前の変更の、前の場所
	Others  string // ストーリー全体の見方で、途中で同じものを変えたほかのストーリーの ID（名前の順に , でつなぐ）
}

func (e rvEntry) String() string {
	s := fmt.Sprintf("%s %s %s", e.Kind, e.Key, e.Change)
	if e.OldPath != "" {
		s += " ← " + e.OldPath
	}
	if e.Others != "" {
		s += "（ほかのストーリー: " + e.Others + "）"
	}
	return s
}

type rvItem struct {
	kind   string            // term・feature・claim
	fields map[string]string // id のほかの、変えられる項目。約束のエビデンスの detail は "evidence.detail"
	file   int               // どの約束リストのファイルに書くか（書き方だけの違いなので、一覧には効かない）
}

func (it rvItem) same(o rvItem) bool {
	if it.kind != o.kind || len(it.fields) != len(o.fields) {
		return false
	}
	for k, v := range it.fields {
		if o.fields[k] != v {
			return false
		}
	}
	return true
}

func (it rvItem) clone() rvItem {
	f := make(map[string]string, len(it.fields))
	for k, v := range it.fields {
		f[k] = v
	}
	return rvItem{kind: it.kind, fields: f, file: it.file}
}

// ファイルの種類。どれが合意の対象のファイルかは、rvConfig のパターンで決まる。
const (
	rvScenario  = "scenario"  // e2e/**/*.txtar（txtar のアダプタ）
	rvTLA       = "tla"       // spec/*.tla（tla のアダプタ）
	rvGuardrail = "guardrail" // db/*.sql（[[guardrail]] の paths）
	rvNoise     = "noise"     // notes/*.md。どのパターンにも当たらないので、一覧に出ない
)

type rvFile struct {
	class      string
	path       string
	content    string
	renameOnly bool // 名前だけを変えるファイルか（中身だけを変えるファイルか）
}

type rvState struct {
	items map[string]rvItem // 鍵は ID
	files map[int]rvFile    // 鍵は、テストの中でファイルを見分ける番号。名前を変えても同じ番号のまま
}

func newRVState() rvState {
	return rvState{items: map[string]rvItem{}, files: map[int]rvFile{}}
}

func (s rvState) clone() rvState {
	c := newRVState()
	for id, it := range s.items {
		c.items[id] = it.clone()
	}
	for n, f := range s.files {
		c.files[n] = f
	}
	return c
}

// 約束の対象に使う用語。どの操作も変えないので、一覧には出ない。
const rvBaseTerm = "base"

const rvPromiseFiles = 3

func rvConfig(version string) string {
	return fmt.Sprintf(`version = %q
promises = [".vanda/promises/*.toml"]

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["e2e/**/*.txtar"]
adapter = "txtar"

[[annotation]]
paths = ["spec/*.tla"]
adapter = "tla"

[[guardrail]]
adapter = "file"
paths = ["db/*.sql"]
`, version)
}

func rvFileKind(class string) bool { return class != rvNoise }

func rvClassifyItem(id string, before, after *rvItem) (rvEntry, bool) {
	switch {
	case before == nil && after == nil:
		return rvEntry{}, false
	case before == nil:
		return rvEntry{Kind: after.kind, Key: id, Change: "added"}, true
	case after == nil:
		return rvEntry{Kind: before.kind, Key: id, Change: "deleted"}, true
	case before.same(*after):
		return rvEntry{}, false
	default:
		return rvEntry{Kind: after.kind, Key: id, Change: "changed"}, true
	}
}

func rvClassifyFile(before, after *rvFile) (rvEntry, bool) {
	switch {
	case before == nil && after == nil:
		return rvEntry{}, false
	case before == nil:
		if !rvFileKind(after.class) {
			return rvEntry{}, false
		}
		return rvEntry{Kind: "file", Key: after.path, Change: "added"}, true
	case after == nil:
		if !rvFileKind(before.class) {
			return rvEntry{}, false
		}
		return rvEntry{Kind: "file", Key: before.path, Change: "deleted"}, true
	}
	if !rvFileKind(after.class) {
		return rvEntry{}, false
	}
	switch {
	case before.path == after.path && before.content == after.content:
		return rvEntry{}, false
	case before.path == after.path:
		return rvEntry{Kind: "file", Key: after.path, Change: "changed"}, true
	default:
		return rvEntry{Kind: "file", Key: after.path, Change: "renamed", OldPath: before.path}, true
	}
}

func itemPtr(s rvState, id string) *rvItem {
	if it, ok := s.items[id]; ok {
		return &it
	}
	return nil
}

func filePtr(s rvState, n int) *rvFile {
	if f, ok := s.files[n]; ok {
		return &f
	}
	return nil
}

// rvBranchView は、ブランチごとの見方の期待値。分かれた点の中身（base）と、作業ツリーの今の中身（cur）を比べる。
func rvBranchView(base, cur rvState) map[rvEntry]bool {
	want := map[rvEntry]bool{}
	for id := range unionKeys(base.items, cur.items) {
		if e, ok := rvClassifyItem(id, itemPtr(base, id), itemPtr(cur, id)); ok {
			want[e] = true
		}
	}
	for n := range unionKeys(base.files, cur.files) {
		if e, ok := rvClassifyFile(filePtr(base, n), filePtr(cur, n)); ok {
			want[e] = true
		}
	}
	return want
}

type rvCommit struct {
	parent int // ないときは -1
	story  string
	state  rvState
	hash   string
}

// rvStoryView は、ストーリー全体の見方の期待値。chain は、根のコミットから今のコミットまで（一本道の履歴）。
// ストーリーのコミットで変わったものごとに、そのストーリーで最初に変えたコミットの直前の中身と、今の中身（cur）を比べる。
// 最初に変えたコミットより後に、ほかのストーリーのコミットが同じものを変えていたら、その ID を Others に書く。
func rvStoryView(chain []rvCommit, cur rvState, story string) map[rvEntry]bool {
	type ref struct {
		item string
		file int
	}
	first := map[ref]int{}
	others := map[ref]map[string]bool{}
	prev := newRVState()
	for i, c := range chain {
		var touched []ref
		for id := range unionKeys(prev.items, c.state.items) {
			if _, ok := rvClassifyItem(id, itemPtr(prev, id), itemPtr(c.state, id)); ok {
				touched = append(touched, ref{item: id})
			}
		}
		for n := range unionKeys(prev.files, c.state.files) {
			b, a := filePtr(prev, n), filePtr(c.state, n)
			if (b != nil && rvFileKind(b.class)) || (a != nil && rvFileKind(a.class)) {
				if b == nil || a == nil || *b != *a {
					touched = append(touched, ref{file: n + 1})
				}
			}
		}
		for _, r := range touched {
			_, seen := first[r]
			switch {
			case c.story == story && !seen:
				first[r] = i
			case c.story != story && seen:
				if others[r] == nil {
					others[r] = map[string]bool{}
				}
				others[r][c.story] = true
			}
		}
		prev = c.state
	}
	want := map[rvEntry]bool{}
	for r, i := range first {
		before := newRVState()
		if i > 0 {
			before = chain[i-1].state
		}
		var e rvEntry
		var ok bool
		if r.file == 0 {
			e, ok = rvClassifyItem(r.item, itemPtr(before, r.item), itemPtr(cur, r.item))
		} else {
			e, ok = rvClassifyFile(filePtr(before, r.file-1), filePtr(cur, r.file-1))
		}
		if !ok {
			continue
		}
		var ids []string
		for s := range others[r] {
			ids = append(ids, s)
		}
		sort.Strings(ids)
		e.Others = strings.Join(ids, ",")
		want[e] = true
	}
	return want
}

func unionKeys[K comparable, V any](a, b map[K]V) map[K]bool {
	u := map[K]bool{}
	for k := range a {
		u[k] = true
	}
	for k := range b {
		u[k] = true
	}
	return u
}

func rvEntries(m map[rvEntry]bool) []string {
	var s []string
	for e := range m {
		s = append(s, e.String())
	}
	sort.Strings(s)
	return s
}

// rvDiffEntries は、2 つの一覧の違い（足りないものと余計なもの）を返す。同じなら空。
func rvDiffEntries(want, got map[rvEntry]bool) string {
	var b strings.Builder
	for _, e := range rvEntries(want) {
		found := false
		for g := range got {
			if g.String() == e {
				found = true
			}
		}
		if !found {
			fmt.Fprintf(&b, "  足りない: %s\n", e)
		}
	}
	for _, g := range rvEntries(got) {
		found := false
		for w := range want {
			if w.String() == g {
				found = true
			}
		}
		if !found {
			fmt.Fprintf(&b, "  余計: %s\n", g)
		}
	}
	return b.String()
}

// --- 書き出し ---

type fataler interface {
	Fatalf(format string, args ...any)
}

// rvPick は、書き方を選ぶ。n 通りのうちの 1 つ（0 から n-1）を返す。
type rvPick func(n int) int

func rvShuffle(xs []string, pick rvPick) {
	for i := len(xs) - 1; i > 0; i-- {
		j := pick(i + 1)
		xs[i], xs[j] = xs[j], xs[i]
	}
}

func rvValue(v string, pick rvPick) string {
	if pick(2) == 0 {
		return fmt.Sprintf("%q", v)
	}
	return "'" + v + "'"
}

func rvArray(vs []string, pick rvPick) string {
	var q []string
	for _, v := range vs {
		q = append(q, rvValue(v, pick))
	}
	if pick(2) == 0 {
		return "[" + strings.Join(q, ", ") + "]"
	}
	return "[ " + strings.Join(q, " , ") + " ]"
}

func rvKeyLine(key, value string, pick rvPick) string {
	sep := []string{" = ", "=", "   =  "}[pick(3)]
	return key + sep + value + "\n"
}

// rvRenderItem は、1 つの項目を TOML の表にする。項目の順、空白、引用符、エビデンスをインラインの表にするかを、pick で選ぶ。
func rvRenderItem(id string, it rvItem, pick rvPick) string {
	var b strings.Builder
	if pick(3) == 0 {
		b.WriteString("# メモ\n")
	}
	fmt.Fprintf(&b, "[[%s]]\n", it.kind)
	lines := map[string]string{"id": rvValue(id, pick)}
	var detail string
	for k, v := range it.fields {
		if k == "evidence.detail" {
			detail = v
			continue
		}
		lines[k] = rvValue(v, pick)
	}
	inlineEvidence := false
	if it.kind == "claim" {
		lines["subjects"] = rvArray([]string{rvBaseTerm}, pick)
		lines["layers"] = rvArray([]string{"model"}, pick)
		if pick(2) == 0 {
			inlineEvidence = true
			ev := []string{
				"stage = " + rvValue("test", pick),
				"type = " + rvValue("unit-test", pick),
				"layers = " + rvArray([]string{"model"}, pick),
				"detail = " + rvValue(detail, pick),
			}
			rvShuffle(ev, pick)
			lines["evidence"] = "[{ " + strings.Join(ev, ", ") + " }]"
		}
	}
	var keys []string
	for k := range lines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rvShuffle(keys, pick)
	for _, k := range keys {
		b.WriteString(rvKeyLine(k, lines[k], pick))
	}
	if it.kind == "claim" && !inlineEvidence {
		b.WriteString(strings.Repeat("\n", pick(2)))
		ev := []string{
			rvKeyLine("stage", rvValue("test", pick), pick),
			rvKeyLine("type", rvValue("unit-test", pick), pick),
			rvKeyLine("layers", rvArray([]string{"model"}, pick), pick),
			rvKeyLine("detail", rvValue(detail, pick), pick),
		}
		rvShuffle(ev, pick)
		b.WriteString("[[claim.evidence]]\n" + strings.Join(ev, ""))
	}
	return b.String()
}

func rvRenderPromises(items map[string]rvItem, pick rvPick) []string {
	out := make([]string, rvPromiseFiles)
	for f := range out {
		var ids []string
		for id, it := range items {
			if it.file == f {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		rvShuffle(ids, pick)
		var b strings.Builder
		for i, id := range ids {
			if i > 0 {
				b.WriteString(strings.Repeat("\n", 1+pick(3)))
			}
			b.WriteString(rvRenderItem(id, items[id], pick))
		}
		out[f] = b.String()
	}
	return out
}

func rvPromisePath(f int) string { return fmt.Sprintf(".vanda/promises/p%d.toml", f) }

// rvGit は、一時的なリポジトリで git を動かす。動かしているマシンの git の設定に左右されないようにする。
type rvGit struct {
	t    fataler
	root string
	env  []string
}

func newRVGit(t fataler, root string) *rvGit {
	return &rvGit{t: t, root: root, env: append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CEILING_DIRECTORIES="+filepath.Dir(root),
		"GIT_AUTHOR_NAME=テスト担当", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=テスト担当", "GIT_COMMITTER_EMAIL=tester@example.com")}
}

func (g *rvGit) run(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.root
	cmd.Env = g.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		g.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeState は、状態を作業ツリーに書き出す。約束リストは毎回、pick で選んだ書き方で書き直す。
func writeState(t fataler, root string, s rvState, pick rvPick) {
	for f, src := range rvRenderPromises(s.items, pick) {
		writeFile(t, root, rvPromisePath(f), src)
	}
	want := map[string]string{}
	for _, f := range s.files {
		want[f.path] = f.content
	}
	for path := range readTreeFiles(t, root) {
		if _, ok := want[path]; !ok {
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(path))); err != nil {
				t.Fatalf("%v", err)
			}
		}
	}
	for path, content := range want {
		writeFile(t, root, path, content)
	}
}

// readTreeFiles は、作業ツリーの、.git と .vanda の外のファイルを読む。
func readTreeFiles(t fataler, root string) map[string]string {
	got := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == ".vanda" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		got[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("作業ツリーを読めない: %v", err)
	}
	return got
}

// rvParsedItems は、約束リストの TOML を読み、rvItem の fields と同じ形にする。
// 書き出しが正しいか（テストの中の状態と、リポジトリの中身が合っているか）を確かめるためだけに使う。
func rvParsedItems(t fataler, srcs map[int]string) map[string]rvItem {
	got := map[string]rvItem{}
	for f, src := range srcs {
		var doc map[string][]map[string]any
		if _, err := toml.Decode(src, &doc); err != nil {
			t.Fatalf("%s を読めない: %v\n%s", rvPromisePath(f), err, src)
		}
		for kind, tables := range doc {
			for _, tab := range tables {
				it := rvItem{kind: kind, fields: map[string]string{}, file: f}
				var id string
				for k, v := range tab {
					switch k {
					case "id":
						id, _ = v.(string)
					case "subjects", "layers":
					case "evidence":
						// [[claim.evidence]] は []map[string]any に、インラインの表の配列は []any に読まれる
						var ev map[string]any
						switch evs := v.(type) {
						case []map[string]any:
							if len(evs) == 1 {
								ev = evs[0]
							}
						case []any:
							if len(evs) == 1 {
								ev, _ = evs[0].(map[string]any)
							}
						}
						if ev == nil {
							t.Fatalf("%s の evidence が 1 件の表の配列でない: %#v", rvPromisePath(f), v)
						}
						d, _ := ev["detail"].(string)
						it.fields["evidence.detail"] = d
					default:
						s, ok := v.(string)
						if !ok {
							t.Fatalf("%s の %s が文字列でない: %#v", rvPromisePath(f), k, v)
						}
						it.fields[k] = s
					}
				}
				if _, dup := got[id]; dup {
					t.Fatalf("ID %s が 2 度書かれた", id)
				}
				got[id] = it
			}
		}
	}
	return got
}

// verifyWorkTree は、作業ツリーの中身が状態 s と合っているかを確かめる。
func verifyWorkTree(t fataler, root string, s rvState) {
	srcs := map[int]string{}
	for f := 0; f < rvPromiseFiles; f++ {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rvPromisePath(f))))
		if err != nil {
			t.Fatalf("%v", err)
		}
		srcs[f] = string(data)
	}
	verifyContent(t, "作業ツリー", rvParsedItems(t, srcs), readTreeFiles(t, root), s)
}

// verifyCommit は、コミット rev の中身が状態 s と合っているかを確かめる。
func verifyCommit(g *rvGit, rev string, s rvState) {
	srcs := map[int]string{}
	for f := 0; f < rvPromiseFiles; f++ {
		srcs[f] = g.run("show", rev+":"+rvPromisePath(f))
	}
	files := map[string]string{}
	for _, p := range strings.Split(g.run("ls-tree", "-r", "--name-only", rev), "\n") {
		if p == "" || strings.HasPrefix(p, ".vanda/") {
			continue
		}
		files[p] = g.run("show", rev+":"+p)
	}
	// git show の出力は、最後の改行を run が落とすので、比べる側も落とす
	trimmed := s.clone()
	for n, f := range trimmed.files {
		f.content = strings.TrimSpace(f.content)
		trimmed.files[n] = f
	}
	verifyContent(g.t, "コミット "+rev, rvParsedItems(g.t, srcs), files, trimmed)
}

func verifyContent(t fataler, where string, items map[string]rvItem, files map[string]string, s rvState) {
	var b strings.Builder
	for id := range unionKeys(items, s.items) {
		got, gok := items[id]
		want, wok := s.items[id]
		if gok != wok || (gok && !got.same(want)) {
			fmt.Fprintf(&b, "  項目 %s: 書いたもの %v、状態 %v\n", id, got, want)
		}
	}
	want := map[string]string{}
	for _, f := range s.files {
		want[f.path] = f.content
	}
	for p := range unionKeys(files, want) {
		if files[p] != want[p] {
			fmt.Fprintf(&b, "  ファイル %s: 書いたもの %q、状態 %q\n", p, files[p], want[p])
		}
	}
	if b.Len() > 0 {
		t.Fatalf("%s の中身が、テストの中の状態と合わない\n%s", where, b.String())
	}
}

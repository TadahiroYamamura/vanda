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
// 期待する一覧は、書き出す前の状態どうしを、docs/formats.md の GET /api/review の決まり（鍵ごとに比べる。ファイルは場所で
// 見分けるので、名前を変えたファイルは古い場所の削除と新しい場所の追加になる）で比べて作る。リポジトリに書いた TOML やファイルを読み直して作らないのは、
// 読み込みや差分の取り方を実装と同じ作りにすると、実装と同じ誤りに気づけないため。

// rvEntry は、一覧の 1 件の期待値。
type rvEntry struct {
	Kind   string // claim・term・feature・file
	Key    string // 約束・用語・機能は ID。ファイルは今の場所（削除なら、前の場所）
	Change string // added・modified・deleted
	// Others は、レビュー単位がストーリーのときに、そのストーリーで最初に変えた後に同じものを変えた、そのストーリーの ID のないコミット。
	// コミットごとに、メッセージにあるストーリーの ID（ないものは "-"）を書き、名前の順に , でつなぐ。
	Others string
}

func (e rvEntry) String() string {
	s := fmt.Sprintf("%s %s %s", e.Kind, e.Key, e.Change)
	if e.Others != "" {
		s += "（ほかのコミット: " + e.Others + "）"
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
	class   string
	path    string
	content string
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
types = ["scenario-test"]
[[annotation]]
paths = ["spec/*.tla"]
adapter = "tla"
types = ["model-check"]
[[guardrail]]
adapter = "file"
paths = ["db/*.sql"]
`, version)
}

func rvFileKind(class string) bool { return class != rvNoise }

// rvVal は、一覧の鍵の 1 つの中身。kind は claim・term・feature・file、body は比べる中身。
type rvVal struct {
	kind string
	body string
}

// rvKeyed は、状態を一覧の鍵ごとの中身にする。約束・用語・機能の鍵は "item:<ID>"、ファイルの鍵は "file:<場所>"。
// 約束・用語・機能の中身は、書き方によらない形（項目の名前の順に並べたもの）にする。
func rvKeyed(s rvState) map[string]rvVal {
	m := map[string]rvVal{}
	for id, it := range s.items {
		var ks []string
		for k := range it.fields {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		var b strings.Builder
		for _, k := range ks {
			fmt.Fprintf(&b, "%s=%q;", k, it.fields[k])
		}
		m["item:"+id] = rvVal{kind: it.kind, body: b.String()}
	}
	for _, f := range s.files {
		if rvFileKind(f.class) {
			m["file:"+f.path] = rvVal{kind: "file", body: f.content}
		}
	}
	return m
}

// rvDiff は、鍵ごとの比べる元（before）と今（after）から一覧を作る。keys に含む鍵だけを見る。
// others は鍵ごとのほかのコミット（rvEntry.Others の元。コミットを見分ける文字列 → 書く文字列）。
func rvDiff(keys map[string]bool, before, after map[string]rvVal, others map[string]map[string]string) map[rvEntry]bool {
	want := map[rvEntry]bool{}
	for k := range keys {
		b, bok := before[k]
		a, aok := after[k]
		switch {
		case bok && aok && b == a:
		case bok && aok:
			want[rvEntryOf(k, a.kind, "modified", others[k])] = true
		case bok:
			want[rvEntryOf(k, b.kind, "deleted", others[k])] = true
		case aok:
			want[rvEntryOf(k, a.kind, "added", others[k])] = true
		}
	}
	return want
}

func rvEntryOf(key, kind, change string, others map[string]string) rvEntry {
	_, k, _ := strings.Cut(key, ":")
	var labels []string
	for _, s := range others {
		labels = append(labels, s)
	}
	sort.Strings(labels)
	return rvEntry{Kind: kind, Key: k, Change: change, Others: strings.Join(labels, ",")}
}

// rvBranchView は、レビュー単位がブランチのときの期待値。分かれた点の中身（base）と、作業ツリーの今の中身（cur）を比べる。
func rvBranchView(base, cur rvState) map[rvEntry]bool {
	b, a := rvKeyed(base), rvKeyed(cur)
	return rvDiff(unionKeys(b, a), b, a, nil)
}

type rvCommit struct {
	parent int // ないときは -1
	story  string
	state  rvState
	hash   string
}

// rvStoryView は、レビュー単位がストーリーのときの期待値。chain は、根のコミットから今のコミットまで（一本道の履歴）。
// 鍵ごとに、そのストーリーの ID を持つコミットのうち、その鍵の中身を変えた最初のコミットの親の中身を比べる元にし、
// 作業ツリーの今の中身（cur）と比べる。最初に変えたコミットより後に、そのストーリーの ID のないコミットが同じ鍵を変えていたら、
// Others に書く。
func rvStoryView(chain []rvCommit, cur rvState, story string) map[rvEntry]bool {
	before := map[string]rvVal{}
	first := map[string]bool{}
	others := map[string]map[string]string{}
	prev := map[string]rvVal{}
	for _, c := range chain {
		now := rvKeyed(c.state)
		for k := range unionKeys(prev, now) {
			p, pok := prev[k]
			n, nok := now[k]
			if pok == nok && p == n {
				continue
			}
			switch {
			case c.story == story && !first[k]:
				first[k] = true
				if pok {
					before[k] = p
				}
			case c.story != story && first[k]:
				if others[k] == nil {
					others[k] = map[string]string{}
				}
				label := c.story
				if label == "" {
					label = "-"
				}
				others[k][c.hash] = label
			}
		}
		prev = now
	}
	return rvDiff(first, before, rvKeyed(cur), others)
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
				"stage = " + rvValue("implementation", pick),
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
			rvKeyLine("stage", rvValue("implementation", pick), pick),
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

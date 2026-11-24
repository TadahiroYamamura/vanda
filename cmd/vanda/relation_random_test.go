package main_test

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// 約束リストの形: refines と assumes の関係の決まり（docs/plans/promise-reorg-shape-impl.md の 2.2 と、docs/formats.md の「関係」）。
// 無作為に作った項目の組で、vanda check が誤りにする行が、テストの中で決まりの表から書いた判定（relExpected）と一致することを確かめる。
// ID は型ごとに別の形にして重ならないようにし、関係の決まりだけを見る（ID の重なりは id_overlap_random_test.go）。

type relRandItem struct {
	kind, id string
	// nil なら、その項目を書かない
	refines []string
	assumes []string
}

// relWantKind は、関係を書く側の型とキーごとの、指してよい先の型。表にない組は、その型に書けないキー
var relWantKind = map[string]string{
	"property.refines": "property",
	"feature.refines":  "property",
	"spec.refines":     "feature",
	"property.assumes": "truth",
	"term.assumes":     "truth",
	"spec.assumes":     "truth",
}

type relRandLines struct {
	head, refines, assumes int
}

func relRandArray(ids []string) string {
	q := make([]string, len(ids))
	for i, id := range ids {
		q[i] = strconv.Quote(id)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func relRandRender(items []relRandItem) (string, map[string]relRandLines) {
	var b strings.Builder
	lines := map[string]relRandLines{}
	n := 0
	write := func(s string) int {
		b.WriteString(s + "\n")
		n++
		return n
	}
	for _, it := range items {
		var l relRandLines
		l.head = write("[[" + it.kind + "]]")
		write("id = " + strconv.Quote(it.id))
		switch it.kind {
		case "property", "truth":
			write(`statement = "文"`)
		case "term", "feature":
			write(`name = "名前"`)
			write(`definition = "定義"`)
		case "spec":
			write(`statement = "文"`)
			write(`layers = ["model"]`)
		}
		if it.refines != nil {
			l.refines = write("refines = " + relRandArray(it.refines))
		}
		if it.assumes != nil {
			l.assumes = write("assumes = " + relRandArray(it.assumes))
		}
		if it.kind == "spec" {
			write("")
			write("[[spec.evidence]]")
			write(`stage = "implementation"`)
			write(`type = "unit-test"`)
			write(`layers = ["model"]`)
		}
		write("")
		lines[it.id] = l
	}
	return b.String(), lines
}

// relExpected は、誤りになる行と、その理由を返す。決まりは、計画の 2.1（機能と仕様の refines は必須で、空にできない）と 2.2 の表。
func relExpected(items []relRandItem, lines map[string]relRandLines) map[int][]string {
	kindOf := map[string]string{}
	for _, it := range items {
		kindOf[it.id] = it.kind
	}
	// 性質 → それが refines する性質（指す先が性質のものだけ）
	parents := map[string][]string{}
	hasChild := map[string]bool{}
	for _, it := range items {
		if it.kind != "property" {
			continue
		}
		for _, up := range it.refines {
			if kindOf[up] == "property" {
				parents[it.id] = append(parents[it.id], up)
				hasChild[up] = true
			}
		}
	}
	returns := func(start string) bool {
		seen := map[string]bool{}
		stack := append([]string{}, parents[start]...)
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if cur == start {
				return true
			}
			if seen[cur] {
				continue
			}
			seen[cur] = true
			stack = append(stack, parents[cur]...)
		}
		return false
	}

	want := map[int][]string{}
	add := func(line int, why string) { want[line] = append(want[line], why) }
	array := func(it relRandItem, key string, ids []string, line int) {
		target := relWantKind[it.kind+"."+key]
		count := map[string]int{}
		for _, id := range ids {
			count[id]++
		}
		for id, c := range count {
			if c > 1 {
				add(line, "重なり")
			}
			switch k, ok := kindOf[id]; {
			case !ok:
				add(line, "先がない")
			case k != target:
				add(line, "型が合わない")
			case it.kind == "feature" && key == "refines" && hasChild[id]:
				add(line, "下に性質を持つ性質に機能をつないだ")
			}
		}
	}
	for _, it := range items {
		l := lines[it.id]
		if it.kind == "feature" || it.kind == "spec" {
			switch {
			case it.refines == nil:
				add(l.head, "refines がない")
			case len(it.refines) == 0:
				add(l.refines, "refines が空")
			}
		}
		if it.refines != nil {
			array(it, "refines", it.refines, l.refines)
		}
		if it.assumes != nil {
			array(it, "assumes", it.assumes, l.assumes)
		}
		if it.kind == "property" && returns(it.id) {
			add(l.refines, "循環")
		}
	}
	return want
}

var relRandErrLine = regexp.MustCompile(`(?m)^\.vanda/promises\.toml:(\d+): `)

func relRandGen(rt *rapid.T) []relRandItem {
	counts := []struct {
		kind, prefix string
		max          int
	}{{"property", "p", 4}, {"truth", "tr", 2}, {"term", "t", 2}, {"feature", "f", 3}, {"spec", "s", 3}}
	byKind := map[string][]string{}
	var all []string
	for _, c := range counts {
		for i := 1; i <= rapid.IntRange(0, c.max).Draw(rt, c.kind+"の数"); i++ {
			id := fmt.Sprintf("%s%d", c.prefix, i)
			byKind[c.kind] = append(byKind[c.kind], id)
			all = append(all, id)
		}
	}
	// 約束リストにない ID
	all = append(all, "nope")
	// 指す先は、決まりに合う型から選ぶことを多くして、通る組も出るようにする
	pick := func(kind, key string) []string {
		right := byKind[relWantKind[kind+"."+key]]
		n := rapid.SampledFrom([]int{0, 1, 1, 1, 1, 1, 2, 2, 3}).Draw(rt, key+"の数")
		ids := []string{}
		for i := 0; i < n; i++ {
			if len(right) > 0 && rapid.IntRange(0, 9).Draw(rt, "決まりに合う型から選ぶか") < 9 {
				ids = append(ids, rapid.SampledFrom(right).Draw(rt, key+"の先"))
			} else {
				ids = append(ids, rapid.SampledFrom(all).Draw(rt, key+"の先"))
			}
		}
		return ids
	}
	var items []relRandItem
	for _, c := range counts {
		for _, id := range byKind[c.kind] {
			it := relRandItem{kind: c.kind, id: id}
			switch c.kind {
			case "property":
				if rapid.Bool().Draw(rt, "性質の refines を書くか") {
					it.refines = pick(c.kind, "refines")
				}
			case "feature", "spec":
				if rapid.IntRange(0, 19).Draw(rt, "refines を書くか") > 0 {
					it.refines = pick(c.kind, "refines")
				}
			}
			if c.kind != "feature" && c.kind != "truth" && rapid.IntRange(0, 2).Draw(rt, "assumes を書くか") == 0 {
				it.assumes = pick(c.kind, "assumes")
			}
			items = append(items, it)
		}
	}
	// 書く順で結果が変わらないことも見る
	return rapid.Permutation(items).Draw(rt, "書く順")
}

// @vanda:evidence mistakes-reported-with-fix
func Test無作為に作った項目の組でrefinesとassumesの誤りの行が決まりの表から書いた判定と一致する(t *testing.T) {
	// t.Parallel() を呼ばない。rapid のシードはプロセスに 1 つのフラグなので、並行に動くほかのテストがフラグを書き換えると、
	// 記録に出したシードと、実際に使ったシードが食い違う
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf("version = %q\n\n[[layer]]\nid = \"model\"\nname = \"モデル\"\ndescription = \"判定と状態の変化\"\n", version))
	writeFile(t, root, ".vanda/checks/shape.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n")

	seed := rvRandomSeed(t)
	t.Logf("シード: VANDA_RV01_SEED=%d", seed)
	setFlag(t, "rapid.seed", strconv.FormatUint(seed, 10))
	// 落ちたときに rapid が testdata/rapid に書くファイルを作らせない。再現には、記録に出したシードを使う
	setFlag(t, "rapid.nofailfile", "true")

	seen := map[string]int{}
	checks := 0
	defer func() {
		t.Logf("確かめた組: %d、理由ごとの回数: %v", checks, seen)
		if t.Failed() || checks < 100 {
			return
		}
		// 生成が偏って、ある決まりを確かめないまま通るのを防ぐ
		for _, why := range []string{"通る", "先がない", "型が合わない", "重なり", "refines がない", "refines が空", "下に性質を持つ性質に機能をつないだ", "循環"} {
			if seen[why] == 0 {
				t.Errorf("%q の組が 1 度も出なかった（シード %d）", why, seed)
			}
		}
	}()

	rapid.Check(t, func(rt *rapid.T) {
		items := relRandGen(rt)
		src, lines := relRandRender(items)
		want := relExpected(items, lines)
		writeFile(rt, root, ".vanda/promises.toml", src)
		// チェックは HEAD のコミットの中身だけを読む
		newRVGit(rt, root).commitAll("(test) チェックの前にコミットする", "")
		if committed := newRVGit(rt, root).run("show", "HEAD:.vanda/promises.toml"); strings.TrimSpace(committed) != strings.TrimSpace(src) {
			rt.Fatalf("書いた約束リストがコミットに入っていない")
		}
		cmd := exec.Command(bin, "check", ".vanda/checks/shape.toml")
		cmd.Dir = root
		cmd.Env = g.env
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exitErr *exec.ExitError
		if err != nil && !errors.As(err, &exitErr) {
			rt.Fatalf("vanda を動かせない: %v", err)
		}
		got := map[int]bool{}
		for _, m := range relRandErrLine.FindAllStringSubmatch(stderr.String(), -1) {
			n, _ := strconv.Atoi(m[1])
			got[n] = true
		}
		var gotLines, wantLines []int
		for n := range got {
			gotLines = append(gotLines, n)
		}
		for n := range want {
			wantLines = append(wantLines, n)
		}
		sort.Ints(gotLines)
		sort.Ints(wantLines)
		if (len(want) == 0) != (err == nil) || fmt.Sprint(gotLines) != fmt.Sprint(wantLines) {
			rt.Fatalf("誤りの行が合わない: vanda %v（終了 %v）、思ったもの %v（理由 %v）\n%s%s\n-- .vanda/promises.toml --\n%s",
				gotLines, err, wantLines, want, stdout.String(), stderr.String(), numbered(src))
		}
		checks++
		if len(want) == 0 {
			seen["通る"]++
		}
		for _, whys := range want {
			for _, why := range whys {
				seen[why]++
			}
		}
	})
}

func numbered(src string) string {
	var b strings.Builder
	for i, l := range strings.Split(strings.TrimRight(src, "\n"), "\n") {
		fmt.Fprintf(&b, "%3d  %s\n", i+1, l)
	}
	return b.String()
}

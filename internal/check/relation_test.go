package check

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 約束リストの新しい形（性質・前提・仕様と、refines・assumes の関係）のテスト。
// 期待値は、docs/plans/promise-reorg-shape-impl.md の 2.1・2.2 と、docs/formats.md の「約束リスト」「関係」から作った。
// 誤りの文は全体を比べず、誤りになるか、どの行を指すか、問題の ID や項目の名前を含むかを確かめる。

// 5 つの型。型の組は小さいので、どのテストも代表を選ばずにすべてを回す
var relKinds = []string{"property", "truth", "term", "feature", "spec"}

// relItem は、約束リストの 1 つの項目。refines・assumes は、nil なら書かない。
type relItem struct {
	kind, id string
	refines  []string
	assumes  []string
}

// relLines は、項目の ID ごとの、キーを書いた行（1 から）。同じ ID を 2 度書いた時は、後のものが残る。
type relLines map[string]map[string]int

func relArray(ids []string) string {
	q := make([]string, len(ids))
	for i, id := range ids {
		q[i] = strconv.Quote(id)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// relRender は、項目を書いた順に TOML にする。必須の項目は、どの型も、通る値で埋める。
func relRender(items []relItem) (string, relLines) {
	var b strings.Builder
	lines := relLines{}
	n := 1
	write := func(id, key, line string) {
		b.WriteString(line + "\n")
		if key != "" {
			if lines[id] == nil {
				lines[id] = map[string]int{}
			}
			lines[id][key] = n
		}
		n++
	}
	for _, it := range items {
		lines[it.id] = nil
		write(it.id, "head", "[["+it.kind+"]]")
		write(it.id, "id", "id = "+strconv.Quote(it.id))
		switch it.kind {
		case "property", "truth":
			write(it.id, "", `statement = "文"`)
		case "term", "feature":
			write(it.id, "", `name = "名前"`)
			write(it.id, "", `definition = "定義"`)
		case "spec":
			write(it.id, "", `statement = "文"`)
			write(it.id, "", `layers = ["model"]`)
		}
		if it.refines != nil {
			write(it.id, "refines", "refines = "+relArray(it.refines))
		}
		if it.assumes != nil {
			write(it.id, "assumes", "assumes = "+relArray(it.assumes))
		}
		if it.kind == "spec" {
			write(it.id, "", "")
			write(it.id, "", "[[spec.evidence]]")
			write(it.id, "", `stage = "implementation"`)
			write(it.id, "", `type = "unit-test"`)
			write(it.id, "", `layers = ["model"]`)
		}
		write(it.id, "", "")
	}
	return b.String(), lines
}

var relErrLine = regexp.MustCompile(`(?m)^\.vanda/promises\.toml:(\d+): (.*)$`)

// relErrors は、チェックの出力から、約束リストの行ごとの誤りの文を集める。
func relErrors(out string) map[int][]string {
	got := map[int][]string{}
	for _, m := range relErrLine.FindAllStringSubmatch(out, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		got[n] = append(got[n], m[2])
	}
	return got
}

func relLineSet(errs map[int][]string) []int {
	var ns []int
	for n := range errs {
		ns = append(ns, n)
	}
	sort.Ints(ns)
	return ns
}

// relWantOnly は、誤りが want の行だけを指し、その行の誤りのどれかが frags をすべて含むことを確かめる。
func relWantOnly(t *testing.T, pass bool, out string, want int, frags ...string) {
	t.Helper()
	errs := relErrors(out)
	if pass || fmt.Sprint(relLineSet(errs)) != fmt.Sprint([]int{want}) {
		t.Errorf("誤りが %d 行目だけを指していない（通った: %v、指した行: %v）\n%s", want, pass, relLineSet(errs), out)
		return
	}
	for _, msg := range errs[want] {
		ok := true
		for _, f := range frags {
			ok = ok && strings.Contains(msg, f)
		}
		if ok {
			return
		}
	}
	t.Errorf("%d 行目の誤りに %q を含むものがない\n%s", want, frags, out)
}

// relBase は、どの型の項目も 1 つずつある、通る約束リスト。
// 性質 tp は、下に性質を持たず、機能も refines していない。機能 tf が refines する性質は base-p にして、
// tp を refines する性質を足しても、tf が「下に性質を持つ性質につないだ機能」にならないようにする
func relBase() []relItem {
	return []relItem{
		{kind: "property", id: "base-p"},
		{kind: "property", id: "tp"},
		{kind: "truth", id: "tr"},
		{kind: "term", id: "tt"},
		{kind: "feature", id: "tf", refines: []string{"base-p"}},
		{kind: "spec", id: "ts", refines: []string{"tf"}},
	}
}

// relTargets は、指す先の型ごとの、relBase の中の ID。「ない」は、約束リストにない ID
var relTargets = []struct{ kind, id string }{
	{"property", "tp"}, {"truth", "tr"}, {"term", "tt"}, {"feature", "tf"}, {"spec", "ts"}, {"ない", "nothing"},
}

// relWriter は、型 kind の項目 w を、関係のほかは通る形で作る。
func relWriter(kind string) relItem {
	w := relItem{kind: kind, id: "w"}
	switch kind {
	case "feature":
		w.refines = []string{"base-p"}
	case "spec":
		w.refines = []string{"tf"}
	}
	return w
}

func TestPromiseListShapeの土台にする約束リストは通る(t *testing.T) {
	r := goodRepo(t)
	src, _ := relRender(relBase())
	if pass, out := shapeOf(t, r, src); !pass {
		t.Fatalf("どの型の項目も 1 つずつある約束リストが通らない（通らなければ、ほかのテストの誤りが関係のせいと言えない）\n%s", out)
	}
}

// 書ける組み合わせは、性質→性質、機能→性質、仕様→機能だけ（docs/plans/promise-reorg-layers.md の 3 章）
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeはrefinesの書く側の型と指す先の型のすべての組で許可された3つの組だけを通す(t *testing.T) {
	r := goodRepo(t)
	allowed := map[string]string{"property": "property", "feature": "property", "spec": "feature"}
	for _, wk := range []string{"property", "feature", "spec"} {
		for _, target := range relTargets {
			t.Run(wk+"から"+target.kind, func(t *testing.T) {
				w := relWriter(wk)
				w.refines = []string{target.id}
				src, lines := relRender(append(relBase(), w))
				pass, out := shapeOf(t, r, src)
				if allowed[wk] == target.kind {
					if !pass {
						t.Errorf("%s の refines が %s を指して、通らない\n%s", wk, target.kind, out)
					}
					return
				}
				relWantOnly(t, pass, out, lines["w"]["refines"], "refines", strconv.Quote(target.id))
			})
		}
	}
}

// assumes の相手は前提だけ。書けるのは、性質・仕様・用語（docs/plans/promise-reorg-layers.md の 3 章）
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeはassumesの書く側の型と指す先の型のすべての組で指す先が前提の時だけ通す(t *testing.T) {
	r := goodRepo(t)
	for _, wk := range []string{"property", "spec", "term"} {
		for _, target := range relTargets {
			t.Run(wk+"から"+target.kind, func(t *testing.T) {
				w := relWriter(wk)
				w.assumes = []string{target.id}
				src, lines := relRender(append(relBase(), w))
				pass, out := shapeOf(t, r, src)
				if target.kind == "truth" {
					if !pass {
						t.Errorf("%s の assumes が前提を指して、通らない\n%s", wk, out)
					}
					return
				}
				relWantOnly(t, pass, out, lines["w"]["assumes"], "assumes", strconv.Quote(target.id))
			})
		}
	}
}

// 機能と前提は、assumes を持たない（docs/formats.md の表）。指す先がどの型でも、決まっていない項目の誤りになる
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは機能と前提にassumesを書くとどの型を指しても誤りにする(t *testing.T) {
	r := goodRepo(t)
	for _, wk := range []string{"feature", "truth"} {
		for _, target := range relTargets {
			t.Run(wk+"から"+target.kind, func(t *testing.T) {
				w := relWriter(wk)
				w.assumes = []string{target.id}
				src, lines := relRender(append(relBase(), w))
				pass, out := shapeOf(t, r, src)
				relWantOnly(t, pass, out, lines["w"]["assumes"], "assumes")
			})
		}
	}
}

// 用語と前提は、refines を持たない（docs/formats.md の表）
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは用語と前提にrefinesを書くとどの型を指しても誤りにする(t *testing.T) {
	r := goodRepo(t)
	for _, wk := range []string{"term", "truth"} {
		for _, target := range relTargets {
			t.Run(wk+"から"+target.kind, func(t *testing.T) {
				w := relWriter(wk)
				w.refines = []string{target.id}
				src, lines := relRender(append(relBase(), w))
				pass, out := shapeOf(t, r, src)
				relWantOnly(t, pass, out, lines["w"]["refines"], "refines")
			})
		}
	}
}

// 性質を n 段につなぎ（p1 が一番上、p(i) は p(i-1) を refines する）、機能をそれぞれの段につなぐ。
// つなげるのは、下に性質を持たない一番下の性質だけ。段が何段でも同じ
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは下に性質を持つ性質に機能をつなぐと誤りにし下に性質を持たない性質にはつなげる(t *testing.T) {
	r := goodRepo(t)
	for depth := 1; depth <= 4; depth++ {
		for at := 1; at <= depth; at++ {
			t.Run(fmt.Sprintf("%d段の%d段目につなぐ", depth, at), func(t *testing.T) {
				var items []relItem
				for i := 1; i <= depth; i++ {
					p := relItem{kind: "property", id: fmt.Sprintf("p%d", i)}
					if i > 1 {
						p.refines = []string{fmt.Sprintf("p%d", i-1)}
					}
					items = append(items, p)
				}
				target := fmt.Sprintf("p%d", at)
				items = append(items,
					relItem{kind: "feature", id: "f", refines: []string{target}},
					relItem{kind: "spec", id: "s", refines: []string{"f"}})
				src, lines := relRender(items)
				pass, out := shapeOf(t, r, src)
				if at == depth {
					if !pass {
						t.Errorf("下に性質を持たない性質 %s に機能をつないで、通らない\n%s", target, out)
					}
					return
				}
				relWantOnly(t, pass, out, lines["f"]["refines"], "refines", strconv.Quote(target))
			})
		}
	}
}

// 2 つの性質につないだ機能は、片方でも下に性質を持てば誤り
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは2つの性質につないだ機能の片方の性質が下に性質を持てば誤りにする(t *testing.T) {
	r := goodRepo(t)
	src, lines := relRender([]relItem{
		{kind: "property", id: "upper"},
		{kind: "property", id: "lower", refines: []string{"upper"}},
		{kind: "property", id: "other"},
		{kind: "feature", id: "f", refines: []string{"other", "upper"}},
	})
	pass, out := shapeOf(t, r, src)
	relWantOnly(t, pass, out, lines["f"]["refines"], "refines", `"upper"`)
}

// 輪の上の性質は、refines をたどると自分に戻る。輪につながるだけの性質（tail）は、自分には戻らないので誤りにしない
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは性質のrefinesが循環すると輪の上の性質をすべて誤りにする(t *testing.T) {
	r := goodRepo(t)
	for size := 1; size <= 4; size++ {
		t.Run(fmt.Sprintf("%dつの輪", size), func(t *testing.T) {
			var items []relItem
			var want []int
			for i := 0; i < size; i++ {
				items = append(items, relItem{kind: "property", id: fmt.Sprintf("c%d", i), refines: []string{fmt.Sprintf("c%d", (i+1)%size)}})
			}
			items = append(items, relItem{kind: "property", id: "tail", refines: []string{"c0"}}, relItem{kind: "property", id: "free"})
			src, lines := relRender(items)
			for i := 0; i < size; i++ {
				want = append(want, lines[fmt.Sprintf("c%d", i)]["refines"])
			}
			sort.Ints(want)
			pass, out := shapeOf(t, r, src)
			errs := relErrors(out)
			if pass || fmt.Sprint(relLineSet(errs)) != fmt.Sprint(want) {
				t.Fatalf("誤りが、輪の上の性質の refines の行 %v だけを指していない（通った: %v、指した行: %v）\n%s", want, pass, relLineSet(errs), out)
			}
			for _, n := range want {
				if !strings.Contains(strings.Join(errs[n], "\n"), "refines") {
					t.Errorf("%d 行目の誤りが refines を示していない\n%s", n, out)
				}
			}
		})
	}
}

// 循環でないもの: 2 つの上の性質を refines する性質が 2 つあり、同じ性質に 2 通りの道でたどり着く
func TestPromiseListShapeは同じ性質に2通りの道でたどり着くだけのrefinesを循環としない(t *testing.T) {
	r := goodRepo(t)
	src, _ := relRender([]relItem{
		{kind: "property", id: "top"},
		{kind: "property", id: "left", refines: []string{"top"}},
		{kind: "property", id: "right", refines: []string{"top"}},
		{kind: "property", id: "bottom", refines: []string{"left", "right"}},
		{kind: "property", id: "bottom2", refines: []string{"right", "left"}},
	})
	if pass, out := shapeOf(t, r, src); !pass {
		t.Errorf("循環していない性質の refines が通らない\n%s", out)
	}
}

func TestPromiseListShapeは1つの性質が2つの性質を1つの機能が2つの性質を1つの仕様が2つの機能をrefinesできる(t *testing.T) {
	r := goodRepo(t)
	src, _ := relRender([]relItem{
		{kind: "property", id: "u1"},
		{kind: "property", id: "u2"},
		{kind: "property", id: "pa", refines: []string{"u1", "u2"}},
		{kind: "property", id: "pb", refines: []string{"u2"}},
		{kind: "truth", id: "tr1"},
		{kind: "truth", id: "tr2"},
		{kind: "feature", id: "fa", refines: []string{"pa", "pb"}},
		{kind: "feature", id: "fb", refines: []string{"pb"}},
		{kind: "spec", id: "s", refines: []string{"fa", "fb"}, assumes: []string{"tr1", "tr2"}},
	})
	if pass, out := shapeOf(t, r, src); !pass {
		t.Errorf("複数の上の項目への refines が通らない\n%s", out)
	}
}

// refines・assumes を書ける型とキーのすべてで、同じ配列に同じ ID を 2 度書く。隣り合う時も、離れている時も誤り
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは同じrefinesかassumesの配列に同じIDが2度あれば誤りにする(t *testing.T) {
	r := goodRepo(t)
	cases := []struct{ kind, key, target, other string }{
		{"property", "refines", "tp", "base-p"},
		{"feature", "refines", "tp", "base-p"},
		{"spec", "refines", "tf", "tf2"},
		{"property", "assumes", "tr", "tr2"},
		{"term", "assumes", "tr", "tr2"},
		{"spec", "assumes", "tr", "tr2"},
	}
	for _, c := range cases {
		for name, ids := range map[string][]string{"隣り合う": {c.target, c.target}, "離れている": {c.target, c.other, c.target}} {
			t.Run(c.kind+"の"+c.key+"で"+name, func(t *testing.T) {
				// w の後に置く機能 tf2 と前提 tr2 は、離れている時の、間の ID。base-p は、w が性質の時に下に性質を持つことになるので、機能はつながない
				items := []relItem{
					{kind: "property", id: "base-p"}, {kind: "property", id: "tp"}, {kind: "property", id: "leaf"},
					{kind: "truth", id: "tr"}, {kind: "truth", id: "tr2"},
					{kind: "feature", id: "tf", refines: []string{"leaf"}}, {kind: "feature", id: "tf2", refines: []string{"leaf"}},
				}
				w := relWriter(c.kind)
				switch c.kind {
				case "feature":
					w.refines = []string{"leaf"}
				}
				if c.key == "refines" {
					w.refines = ids
				} else {
					w.assumes = ids
				}
				src, lines := relRender(append(items, w))
				pass, out := shapeOf(t, r, src)
				relWantOnly(t, pass, out, lines["w"][c.key], c.key, strconv.Quote(c.target))
			})
		}
	}
}

// 同じ ID を、型の組のすべて（先に書く型 × 後に書く型）で 2 度書く。誤りは、後に書いたほうの id の行を指す（docs/formats.md）
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは性質と前提を含むどの型の組でもIDが同じなら後に書いたほうを誤りにする(t *testing.T) {
	r := goodRepo(t)
	for _, first := range relKinds {
		for _, second := range relKinds {
			t.Run(first+"の後に"+second, func(t *testing.T) {
				a, b := relWriter(first), relWriter(second)
				a.id, b.id = "same", "same"
				var b2 strings.Builder
				baseSrc, _ := relRender(append(relBase(), a))
				b2.WriteString(baseSrc)
				offset := strings.Count(baseSrc, "\n")
				tail, lines := relRender([]relItem{b})
				b2.WriteString(tail)
				pass, out := shapeOf(t, r, b2.String())
				relWantOnly(t, pass, out, offset+lines["same"]["id"], `"same"`)
			})
		}
	}
}

// 決まっていない表と項目は、その表の見出し・項目の行を指す誤りになる（docs/formats.md）。
// [[claim]] と subjects は、古い形。読み替えずに、決まっていない表・項目として誤りにする
// @vanda:evidence mistakes-reported-with-fix
func TestPromiseListShapeは古い形のclaimとsubjectsと性質と前提の決まっていない項目を誤りにする(t *testing.T) {
	r := goodRepo(t)
	const evidence = "stage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n"
	cases := []struct {
		name, from, to string
		line           int
		frag           string
	}{
		{"claimの表", "[[spec]]\nid = \"c\"", "[[claim]]\nid = \"c\"", 20, "claim"},
		{"仕様のsubjects", `statement = "s"`, "statement = \"s\"\nsubjects = [\"task\"]", 24, "subjects"},
		{"仕様のrefinesをsubjectsにする", `refines = ["add-task"]`, `subjects = ["add-task"]`, 22, "subjects"},
		{"性質のname", `statement = "登録したタスクは失われない"`, "statement = \"登録したタスクは失われない\"\nname = \"名前\"", 4, "name"},
		{"性質の決まっていない項目", `statement = "登録したタスクは失われない"`, "statement = \"登録したタスクは失われない\"\ncondition = \"c\"", 4, "condition"},
		{"性質のevidenceの表", "statement = \"登録したタスクは失われない\"\n", "statement = \"登録したタスクは失われない\"\n\n[[property.evidence]]\n" + evidence, 5, "evidence"},
		{"性質のインラインのevidence", `statement = "登録したタスクは失われない"`, "statement = \"登録したタスクは失われない\"\nevidence = [{ stage = \"implementation\", type = \"unit-test\", layers = [\"model\"] }]", 4, "evidence"},
		{"前提のname", `statement = "ディスクに書いたものは、読み戻せる"`, "statement = \"ディスクに書いたものは、読み戻せる\"\nname = \"名前\"", 8, "name"},
		{"前提のrationale", `statement = "ディスクに書いたものは、読み戻せる"`, "statement = \"ディスクに書いたものは、読み戻せる\"\nrationale = \"r\"", 8, "rationale"},
		{"前提のevidenceの表", "statement = \"ディスクに書いたものは、読み戻せる\"\n", "statement = \"ディスクに書いたものは、読み戻せる\"\n\n[[truth.evidence]]\n" + evidence, 9, "evidence"},
		{"用語のrefines", `definition = "やること"`, "definition = \"やること\"\nrefines = [\"no-lost-task\"]", 13, "refines"},
		{"機能のassumes", `definition = "タスクを足す"`, "definition = \"タスクを足す\"\nassumes = [\"disk-works\"]", 18, "assumes"},
		{"機能のsubjects", `definition = "タスクを足す"`, "definition = \"タスクを足す\"\nsubjects = [\"task\"]", 18, "subjects"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := strings.Replace(minimalPromises, c.from, c.to, 1)
			if src == minimalPromises {
				t.Fatalf("約束リストに %q がない", c.from)
			}
			pass, out := shapeOf(t, r, src)
			prefix := ".vanda/promises.toml:" + strconv.Itoa(c.line) + ": "
			if pass || !lineWith(out, prefix, c.frag) {
				t.Errorf("%s で始まり %q を含む誤りにならない\n%s", prefix, c.frag, out)
			}
		})
	}
	// 新しい形の約束リストの末尾に、古い形の仕様を足す。minimalPromises は 29 行なので、空行をはさんで 31 行目が表の見出し
	t.Run("新しい形の後に足したclaimの表", func(t *testing.T) {
		src := minimalPromises + "\n[[claim]]\nid = \"old\"\nsubjects = [\"task\"]\nstatement = \"s\"\nlayers = [\"model\"]\n"
		if pass, out := shapeOf(t, r, src); pass || !lineWith(out, ".vanda/promises.toml:31: ", "claim") {
			t.Errorf(".vanda/promises.toml:31: で始まり claim を含む誤りにならない\n%s", out)
		}
	})
}

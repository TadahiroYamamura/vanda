package main_test

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// RV-01 のランダムチェックの、操作の列を作って git のリポジトリに当て、期待する一覧を作るところ。
// 一覧を API から読んで比べるのは、API の形が docs/formats.md に書かれてから足す。

var (
	rvClaimIDs   = []string{"c-a", "c-b", "c-c", "c-d"}
	rvTermIDs    = []string{"t-a", "t-b"}
	rvFeatureIDs = []string{"f-a", "f-b"}
	rvStories    = []string{"#1", "#24", "#25", ""}
)

func rvIDs(kind string) []string {
	switch kind {
	case "claim":
		return rvClaimIDs
	case "term":
		return rvTermIDs
	default:
		return rvFeatureIDs
	}
}

// rvRandomSeed は、rapid の種を決める。既定では実行のたびに変え、VANDA_RV01_SEED でだけ決める。
// rapid の -rapid.seed を使わないのは、通ったときにも使った種を記録に出すため（rapid は落ちたときにしか出さない）。
func rvRandomSeed(t *testing.T) uint64 {
	t.Helper()
	if v := os.Getenv("VANDA_RV01_SEED"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || n == 0 {
			t.Fatalf("VANDA_RV01_SEED=%q は 0 でない整数でない", v)
		}
		return n
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("種を作れない: %v", err)
	}
	// rapid は 0 を「種を自分で選ぶ」の意味に使うので、0 を避ける
	return binary.LittleEndian.Uint64(b[:]) | 1
}

func setFlag(t *testing.T, name, value string) {
	t.Helper()
	old := flag.Lookup(name).Value.String()
	if err := flag.Set(name, value); err != nil {
		t.Fatalf("-%s=%s を設定できない: %v", name, value, err)
	}
	t.Cleanup(func() {
		if err := flag.Set(name, old); err != nil {
			t.Errorf("-%s を戻せない: %v", name, err)
		}
	})
}

// rvRandomOp は、今の状態でできる操作を 1 つ選んで当てる。
func rvRandomOp(rt *rapid.T, r *rvRepo) string {
	var ops []string
	var present, absent []string
	for _, kind := range []string{"claim", "term", "feature"} {
		for _, id := range rvIDs(kind) {
			if _, ok := r.state.items[id]; ok {
				present = append(present, id)
			} else {
				absent = append(absent, kind+" "+id)
			}
		}
	}
	var files []int
	for n := range r.state.files {
		files = append(files, n)
	}
	sort.Ints(files)
	if len(absent) > 0 {
		ops = append(ops, "addItem")
	}
	if len(present) > 0 {
		ops = append(ops, "changeItem", "deleteItem", "moveItem")
	}
	ops = append(ops, "addFile", "reformat")
	if len(files) > 0 {
		// 名前の変更は、分かれた点より前からあるファイルに、変更なしで当たらないと一覧に出ないので、ほかより多く選ぶ
		ops = append(ops, "changeFile", "deleteFile", "renameFile", "renameFile", "renameFile")
	}
	op := rapid.SampledFrom(ops).Draw(rt, "操作")
	switch op {
	case "addItem":
		ki := strings.Fields(rapid.SampledFrom(absent).Draw(rt, "足す項目"))
		f := rapid.IntRange(0, rvPromiseFiles-1).Draw(rt, "ファイル")
		r.addItem(ki[0], ki[1], f)
		return fmt.Sprintf("%s %s %s を p%d に", op, ki[0], ki[1], f)
	case "changeItem":
		id := rapid.SampledFrom(present).Draw(rt, "変える項目")
		field := rapid.SampledFrom(rvItemFields(r.state.items[id].kind)).Draw(rt, "変える所")
		r.changeItem(id, field)
		return fmt.Sprintf("%s %s の %s", op, id, field)
	case "deleteItem":
		id := rapid.SampledFrom(present).Draw(rt, "消す項目")
		r.deleteItem(id)
		return op + " " + id
	case "moveItem":
		id := rapid.SampledFrom(present).Draw(rt, "移す項目")
		f := rapid.IntRange(0, rvPromiseFiles-1).Draw(rt, "移す先")
		r.moveItem(id, f)
		return fmt.Sprintf("%s %s を p%d へ", op, id, f)
	case "addFile":
		class := rapid.SampledFrom([]string{rvScenario, rvTLA, rvGuardrail, rvNoise}).Draw(rt, "ファイルの種類")
		var n int
		if free := r.freePaths(class); len(free) > 0 && rapid.Bool().Draw(rt, "場所を使い直すか") {
			n = r.addFileAt(class, rapid.SampledFrom(free).Draw(rt, "使い直す場所"))
		} else {
			n = r.addFile(class)
		}
		return fmt.Sprintf("%s %s", op, r.state.files[n].path)
	case "changeFile":
		n := rapid.SampledFrom(files).Draw(rt, "中身を変えるファイル")
		r.changeFile(n)
		return op + " " + r.state.files[n].path
	case "renameFile":
		n := rapid.SampledFrom(files).Draw(rt, "名前を変えるファイル")
		from := r.state.files[n].path
		var to string
		if free := r.freePaths(r.state.files[n].class); len(free) > 0 && rapid.Bool().Draw(rt, "場所を使い直すか") {
			to = r.renameFileTo(n, rapid.SampledFrom(free).Draw(rt, "使い直す場所"))
		} else {
			to = r.renameFile(n, r.state.files[n].class == rvScenario && rapid.Bool().Draw(rt, "下のディレクトリへ"))
		}
		return fmt.Sprintf("%s %s → %s", op, from, to)
	case "deleteFile":
		n := rapid.SampledFrom(files).Draw(rt, "消すファイル")
		p := r.state.files[n].path
		r.deleteFile(n)
		return op + " " + p
	default:
		r.reformat()
		return op
	}
}

func rvRandomOps(rt *rapid.T, r *rvRepo, label string, max int, mayLeave bool, log *[]string) {
	n := rapid.IntRange(0, max).Draw(rt, label+"の操作の数")
	for i := 0; i < n; i++ {
		*log = append(*log, fmt.Sprintf("%s: %s", label, rvRandomOp(rt, r)))
		if !mayLeave || rapid.Bool().Draw(rt, "コミットするか") {
			story := rapid.SampledFrom(rvStories).Draw(rt, "ストーリー")
			r.commit(story)
			*log = append(*log, "  コミット "+story)
		}
	}
}

// RV-01・RV-13
func TestRV01の操作の列をリポジトリに当てると期待する一覧を数えられる(t *testing.T) {
	runRV01Random(t, nil)
}

// rvCompare は、作った一時的なリポジトリ（r）と期待する一覧を受け取り、vanda の一覧と比べる。
// 書きかけの変更があるときは、ブランチごとの見方だけで 1 度呼び（stories は nil）、コミットした後に両方の見方で呼ぶ。
type rvCompare func(rt *rapid.T, bin string, r *rvRepo, branch map[rvEntry]bool, stories map[string]map[rvEntry]bool)

// runRV01Random は、無作為な操作の列を当てたリポジトリを作り、期待する一覧を数える。compare があれば、vanda の一覧と比べる。
func runRV01Random(t *testing.T, compare rvCompare) {
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	seed := rvRandomSeed(t)
	t.Logf("種: VANDA_RV01_SEED=%d", seed)
	setFlag(t, "rapid.seed", strconv.FormatUint(seed, 10))
	checks := envInt(t, "VANDA_RV01_CHECKS", 100)
	setFlag(t, "rapid.checks", strconv.Itoa(checks))
	// 落ちたときに rapid が testdata/rapid に書くファイルを作らせない。再現には、記録に出した種を使う
	setFlag(t, "rapid.nofailfile", "true")
	tmp := t.TempDir()
	// 一覧の期待値に、どの変化が何回出たか。生成が偏って、確かめていない変化がないかを記録に出す
	seen := map[string]int{}
	defer func() {
		var keys []string
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, " %s=%d", k, seen[k])
		}
		t.Logf("期待値に出た回数:%s", b.String())
		if t.Failed() || checks < 100 {
			return
		}
		// 下限は、100 回を 5 通りの種で数えたいちばん少ない回数（種類と変化の組で 6、名前の変更で 25）の半分ほど。
		// 生成が偏って、ある変化を確かめないまま通るのを防ぐ
		for _, kind := range []string{"claim", "term", "feature", "file"} {
			for _, change := range []string{"added", "modified", "deleted"} {
				atLeast(t, seen, "ブランチごと/"+kind+"/"+change, 3)
			}
		}
		atLeast(t, seen, "ブランチごと/file/renamed", 10)
		for _, change := range []string{"added", "modified", "deleted", "renamed", "ほかのストーリー"} {
			atLeast(t, seen, "ストーリー全体/"+change, 10)
		}
	}()

	rapid.Check(t, func(rt *rapid.T) {
		root, err := os.MkdirTemp(tmp, "repo")
		if err != nil {
			rt.Fatalf("%v", err)
		}
		pick := func(n int) int { return rapid.IntRange(0, n-1).Draw(rt, "書き方") }
		r := newRVRepo(rt, root, version, "#1", pick)
		var log []string
		defer func() {
			if rt.Failed() {
				rt.Logf("操作の列:\n%s", strings.Join(log, "\n"))
			}
		}()

		// 変更・削除・名前の変更が一覧に出るには、分かれた点より前からあるものが要るので、はじめにいくつか置く
		for _, kind := range []string{"claim", "term", "feature"} {
			for _, id := range rvIDs(kind) {
				if rapid.Bool().Draw(rt, "はじめに置く項目か") {
					r.addItem(kind, id, rapid.IntRange(0, rvPromiseFiles-1).Draw(rt, "ファイル"))
					log = append(log, fmt.Sprintf("main: addItem %s %s", kind, id))
				}
			}
		}
		for i, n := 0, rapid.IntRange(1, 4).Draw(rt, "はじめのファイルの数"); i < n; i++ {
			class := rapid.SampledFrom([]string{rvScenario, rvTLA, rvGuardrail}).Draw(rt, "はじめのファイルの種類")
			log = append(log, "main: addFile "+r.state.files[r.addFile(class)].path)
		}
		r.commit("#1")
		rvRandomOps(rt, r, "main", 10, false, &log)
		r.newBranch("story")
		rvRandomOps(rt, r, "story", 6, false, &log)
		r.switchTo("main")
		rvRandomOps(rt, r, "分かれた後の main", 4, false, &log)
		r.switchTo("story")
		rvRandomOps(rt, r, "story", 8, true, &log)

		dirtyBranch := r.branchView("main")
		if compare != nil && r.dirty {
			compare(rt, bin, r, dirtyBranch, nil)
		}
		branch := dirtyBranch
		for e := range branch {
			seen["ブランチごと/"+e.Kind+"/"+e.Change]++
		}
		shape(rt, bin, root)

		if r.dirty {
			story := rapid.SampledFrom(rvStories).Draw(rt, "残りのストーリー")
			r.commit(story)
			log = append(log, "残りをコミット "+story)
		}
		stories := map[string]map[rvEntry]bool{}
		for _, s := range rvStories {
			if s == "" {
				continue
			}
			stories[s] = r.storyView(s)
			for e := range stories[s] {
				seen["ストーリー全体/"+e.Change]++
				if e.Others != "" {
					seen["ストーリー全体/ほかのストーリー"]++ // そのストーリーの ID のないコミットが途中で変えたもの
				}
			}
		}
		if again := r.branchView("main"); rvDiffEntries(branch, again) != "" {
			rt.Fatalf("コミットしただけで、ブランチごとの見方の期待値が変わった\n%s", rvDiffEntries(branch, again))
		}
		if compare != nil {
			compare(rt, bin, r, branch, stories)
		}
	})
}

func atLeast(t *testing.T, seen map[string]int, key string, min int) {
	t.Helper()
	if seen[key] < min {
		t.Errorf("期待値に %s が %d 回しか出ていない（下限 %d）。操作の列の作り方が偏っている", key, seen[key], min)
	}
}

// shape は、書き出した約束リストが、段 1 の約束リストの形の審査を通ることを確かめる。
// 通らない約束リストでは、一覧が出なくても vanda の誤りとは言えないため。
func shape(t fataler, bin, root string) {
	writeFile(t, root, ".vanda/checks/shape.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n")
	defer func() {
		if err := os.Remove(root + "/.vanda/checks/shape.toml"); err != nil {
			t.Fatalf("%v", err)
		}
	}()
	cmd := exec.Command(bin, "check", ".vanda/checks/shape.toml")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("書き出した約束リストが、約束リストの形の審査を通らない: %v\n%s", err, out)
	}
}

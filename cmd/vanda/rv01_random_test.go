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
	rvStories    = []string{"#1", "#24", "#25"}
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
	var changeable, renameable []int
	for n, f := range r.state.files {
		if f.renameOnly {
			renameable = append(renameable, n)
		} else {
			changeable = append(changeable, n)
		}
	}
	sort.Ints(changeable)
	sort.Ints(renameable)
	files := append(append([]int{}, changeable...), renameable...)
	sort.Ints(files)
	if len(absent) > 0 {
		ops = append(ops, "addItem")
	}
	if len(present) > 0 {
		ops = append(ops, "changeItem", "deleteItem", "moveItem")
	}
	ops = append(ops, "addFile", "reformat")
	if len(changeable) > 0 {
		ops = append(ops, "changeFile")
	}
	if len(renameable) > 0 {
		ops = append(ops, "renameFile")
	}
	if len(files) > 0 {
		ops = append(ops, "deleteFile")
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
		renameOnly := class != rvTLA && rapid.Bool().Draw(rt, "名前だけを変えるファイルか")
		n := r.addFile(class, renameOnly)
		return fmt.Sprintf("%s %s", op, r.state.files[n].path)
	case "changeFile":
		n := rapid.SampledFrom(changeable).Draw(rt, "中身を変えるファイル")
		r.changeFile(n)
		return op + " " + r.state.files[n].path
	case "renameFile":
		n := rapid.SampledFrom(renameable).Draw(rt, "名前を変えるファイル")
		from := r.state.files[n].path
		to := r.renameFile(n, r.state.files[n].class == rvScenario && rapid.Bool().Draw(rt, "下のディレクトリへ"))
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
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	seed := rvRandomSeed(t)
	t.Logf("種: VANDA_RV01_SEED=%d", seed)
	setFlag(t, "rapid.seed", strconv.FormatUint(seed, 10))
	setFlag(t, "rapid.checks", strconv.Itoa(envInt(t, "VANDA_RV01_CHECKS", 100)))
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

		rvRandomOps(rt, r, "main", 10, false, &log)
		r.newBranch("story")
		rvRandomOps(rt, r, "story", 6, false, &log)
		r.switchTo("main")
		rvRandomOps(rt, r, "分かれた後の main", 4, false, &log)
		r.switchTo("story")
		rvRandomOps(rt, r, "story", 8, true, &log)

		branch := r.branchView("main")
		for e := range branch {
			seen["ブランチごと/"+e.Kind+"/"+e.Change]++
		}
		shape(rt, bin, root)

		if r.dirty {
			story := rapid.SampledFrom(rvStories).Draw(rt, "残りのストーリー")
			r.commit(story)
			log = append(log, "残りをコミット "+story)
		}
		for _, s := range rvStories {
			for e := range r.storyView(s) {
				seen["ストーリー全体/"+e.Change]++
				if e.Others != "" {
					seen["ストーリー全体/ほかのストーリー"]++
				}
			}
		}
		if again := r.branchView("main"); rvDiffEntries(branch, again) != "" {
			rt.Fatalf("コミットしただけで、ブランチごとの見方の期待値が変わった\n%s", rvDiffEntries(branch, again))
		}
	})
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

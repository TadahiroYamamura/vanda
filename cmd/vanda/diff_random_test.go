package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// diffRandomLine は、行の候補。同じ行が何度も出るように少なくし、行の LCS が何通りにもなる入力を作る
var diffRandomLine = []string{"a", "b", "a b", "ab", "", "買い物を登録した", "買い物を足した"}

func drawDiffFile(rt *rapid.T, label string) string {
	rows := rapid.SliceOfN(rapid.SampledFrom(diffRandomLine), 0, 6).Draw(rt, label)
	s := strings.Join(rows, "\n")
	if len(rows) > 0 && rapid.Bool().Draw(rt, label+"の最後の改行") {
		s += "\n"
	}
	return s
}

// checkDiffLines は、docs/formats.md の entries[].diff.lines の決まりのうち、行の分け方によらずに言えるものを確かめる。
func checkDiffLines(rt *rapid.T, path, before, after string, lines []diffLine) {
	if b := rebuild(lines, "delete"); b != before {
		rt.Errorf("%s: equal と delete の行をつなげても変更前（%q）にならない: %q", path, before, b)
	}
	if a := rebuild(lines, "insert"); a != after {
		rt.Errorf("%s: equal と insert の行をつなげても変更後（%q）にならない: %q", path, after, a)
	}
	var ops []string
	for _, l := range lines {
		ops = append(ops, l.Op)
	}
	rw := rewritten(ops)
	nb, na := 0, 0
	for i, l := range lines {
		switch l.Op {
		case "equal", "delete", "insert":
		default:
			rt.Errorf("%s: %d 行目の op が %q", path, i+1, l.Op)
		}
		// 同じ所（equal にはさまれた所）では、消した行を足した行より先に並べる
		if l.Op == "delete" && i > 0 && lines[i-1].Op == "insert" {
			rt.Errorf("%s: %d 行目で、足した行の後に消した行がある: %v", path, i+1, ops)
		}
		if l.Op != "insert" {
			nb++
			if l.Before == nil || *l.Before != nb {
				rt.Errorf("%s: %d 行目（%s %q）の before が %v（%d のはず）", path, i+1, l.Op, l.Text, l.Before, nb)
			}
		} else if l.Before != nil {
			rt.Errorf("%s: %d 行目（insert）の before が null でない", path, i+1)
		}
		if l.Op != "delete" {
			na++
			if l.After == nil || *l.After != na {
				rt.Errorf("%s: %d 行目（%s %q）の after が %v（%d のはず）", path, i+1, l.Op, l.Text, l.After, na)
			}
		} else if l.After != nil {
			rt.Errorf("%s: %d 行目（delete）の after が null でない", path, i+1)
		}
		if strings.Contains(l.Text, "\n") {
			rt.Errorf("%s: %d 行目の text に改行がある: %q", path, i+1, l.Text)
		}
		if rw[i] != (l.Parts != nil) {
			rt.Errorf("%s: %d 行目（%s %q）: parts は書き換えた行だけに付く（書き換え %v、parts %v）", path, i+1, l.Op, l.Text, rw[i], l.Parts)
		}
		if l.Parts != nil {
			if joinParts(l.Parts, "equal", "delete", "insert") != l.Text {
				rt.Errorf("%s: %d 行目の parts をつなげても text にならない: %+v", path, i+1, l.Parts)
			}
			for _, p := range l.Parts {
				if p.Op != "equal" && p.Op != l.Op {
					rt.Errorf("%s: %d 行目（%s）の parts に %s がある", path, i+1, l.Op, p.Op)
				}
			}
		}
	}
}

// RV-05 のランダムチェック。手で作った例（diff_api_test.go）では、行の LCS が 1 通りに決まる例しか確かめていないので、
// 同じ行が何度も出るファイルを無作為に作り、行の分け方によらずに言える決まりを確かめる
// @vanda:evidence review-file-diff-lines
func Testファイルの差分は無作為な変更前と変更後でもすべての行をファイルの順に1度ずつ並べ書き換えた行だけを文字の単位で比べる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	seed := rvRandomSeed(t)
	t.Logf("シード: VANDA_RV01_SEED=%d", seed)
	setFlag(t, "rapid.seed", strconv.FormatUint(seed, 10))
	setFlag(t, "rapid.checks", strconv.Itoa(envInt(t, "VANDA_DIFF_CHECKS", 30)))
	setFlag(t, "rapid.nofailfile", "true")
	tmp := t.TempDir()
	// どの op の並びが出たか。生成が偏って、書き換えや改行の有無の違いを確かめないまま通るのを防ぐ
	seen := map[string]int{}
	rapid.Check(t, func(rt *rapid.T) {
		root, err := os.MkdirTemp(tmp, "repo")
		if err != nil {
			rt.Fatalf("%v", err)
		}
		g := newRVGit(rt, root)
		g.run("init", "-q", "-b", "main")
		writeFile(rt, root, ".vanda/config.toml", fmt.Sprintf(diffConfig, version))
		type pair struct{ before, after string }
		files := map[string]pair{}
		for i := range 4 {
			path := fmt.Sprintf("e2e/f%d.txtar", i)
			before := drawDiffFile(rt, path+" の変更前")
			after := drawDiffFile(rt, path+" の変更後")
			// 無関係な 2 つのファイルでは equal の行がほとんど出ないので、半分は変更前の 1 行だけを差し替えたものにする
			if rows := strings.Split(before, "\n"); len(rows) > 1 && rapid.Bool().Draw(rt, path+" を 1 行だけ変える") {
				rows[rapid.IntRange(0, len(rows)-1).Draw(rt, path+" の変える行")] = rapid.SampledFrom(diffRandomLine).Draw(rt, path+" の新しい行")
				after = strings.Join(rows, "\n")
			}
			files[path] = pair{before, after}
			writeFile(rt, root, path, files[path].before)
		}
		g.commitAll("はじめ", "#1")
		g.run("switch", "-q", "-c", "story-24")
		for path, p := range files {
			writeFile(rt, root, path, p.after)
		}
		g.commitAll("変える", "#24")

		s := startServe(rt, bin, root, g.env)
		var got reviewResponse
		if code, msg := s.get("/api/review?view=branch&base=main", &got); code != 200 {
			rt.Fatalf("一覧を読めない（%d）: %s", code, msg)
		}
		for path, p := range files {
			var e *reviewEntry
			for i := range got.Entries {
				if got.Entries[i].key() == path {
					e = &got.Entries[i]
				}
			}
			if p.before == p.after {
				if e != nil {
					rt.Errorf("%s: 中身が変わっていないのに一覧にある", path)
				}
				continue
			}
			if e == nil {
				rt.Fatalf("%s: 中身が変わったのに一覧にない（変更前 %q、変更後 %q）", path, p.before, p.after)
			}
			var d entryDiff
			if isNull(e.Diff) {
				rt.Fatalf("%s: 変更したファイルに diff がない", path)
			}
			if err := json.Unmarshal(e.Diff, &d); err != nil {
				rt.Fatalf("%s の diff を読めない: %v（%s）", path, err, e.Diff)
			}
			checkDiffLines(rt, path, p.before, p.after, d.Lines)
			for _, l := range d.Lines {
				seen[l.Op]++
				if l.Parts != nil {
					seen["書き換え"]++
				}
				if l.NoNewline {
					seen["最後の改行なし"]++
				}
			}
		}
	})
	t.Logf("出た回数: %v", seen)
	for _, k := range []string{"equal", "delete", "insert", "書き換え", "最後の改行なし"} {
		if !t.Failed() && seen[k] == 0 {
			t.Errorf("%s が一度も出ていない（生成が偏っている）", k)
		}
	}
}

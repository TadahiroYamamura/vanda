package main_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// CMD-10 約束リストの形: ID は用語・機能・約束を通し、分けたファイルをまたいでも重ならない。
// 重なった ID の誤りは、後に出てきたほう（ファイルの場所の順、同じファイルなら行の順）の行を指す（docs/formats.md）。
// 無作為に作った約束リストで、vanda check が出す誤りの場所が、テストの中で数えた「後に出てきたもの」と一致することを確かめる。

type overlapItem struct {
	kind string // term・feature・claim
	id   string
	file int
}

type overlapLoc struct {
	path string
	line int
}

// 重なりが起きやすいよう、ID は小さな集まりから選ぶ。base は重ならない用語で、約束の対象に使う。
var overlapIDs = []string{"a", "b", "c-1", "d2"}

func renderOverlap(items []overlapItem, files int) (map[string]string, map[int]overlapLoc) {
	srcs := map[string]string{}
	locs := map[int]overlapLoc{}
	for f := 0; f < files; f++ {
		path := fmt.Sprintf(".vanda/promises/p%d.toml", f)
		var b strings.Builder
		line := 1
		write := func(s string) {
			b.WriteString(s)
			line += strings.Count(s, "\n")
		}
		for i, it := range items {
			if it.file != f {
				continue
			}
			write("[[" + it.kind + "]]\n")
			locs[i] = overlapLoc{path, line}
			write(fmt.Sprintf("id = %q\n", it.id))
			switch it.kind {
			case "term", "feature":
				write("name = \"名前\"\ndefinition = \"定義\"\n")
			case "claim":
				write("subjects = [\"base\"]\nstatement = \"文\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"test\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n")
			}
			write("\n")
		}
		srcs[path] = b.String()
	}
	return srcs, locs
}

// expectedOverlaps は、ID ごとに、最初に出てきたもののほかの場所を返す。
func expectedOverlaps(items []overlapItem, locs map[int]overlapLoc) map[overlapLoc]string {
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		la, lb := locs[order[a]], locs[order[b]]
		if la.path != lb.path {
			return la.path < lb.path
		}
		return la.line < lb.line
	})
	seen := map[string]bool{}
	want := map[overlapLoc]string{}
	for _, i := range order {
		if seen[items[i].id] {
			want[locs[i]] = items[i].id
		}
		seen[items[i].id] = true
	}
	return want
}

func Test分けたファイルをまたいでも重なったIDは後に出てきたものの場所だけを誤りにする(t *testing.T) {
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	fixed := map[string]string{
		".vanda/config.toml":        fmt.Sprintf("version = %q\npromises = [\".vanda/promises/*.toml\"]\n\n[[layer]]\nid = \"model\"\nname = \"モデル\"\ndescription = \"判定と状態の変化\"\n", version),
		".vanda/checks/shape.toml":  "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n",
		".vanda/promises/base.toml": "[[term]]\nid = \"base\"\nname = \"もと\"\ndefinition = \"約束の対象\"\n",
	}
	for path, src := range fixed {
		writeFile(t, root, path, src)
	}

	rapid.Check(t, func(rt *rapid.T) {
		files := rapid.IntRange(1, 3).Draw(rt, "ファイルの数")
		items := rapid.SliceOfN(rapid.Custom(func(rt *rapid.T) overlapItem {
			return overlapItem{
				kind: rapid.SampledFrom([]string{"term", "feature", "claim"}).Draw(rt, "表"),
				id:   rapid.SampledFrom(overlapIDs).Draw(rt, "ID"),
				file: rapid.IntRange(0, files-1).Draw(rt, "ファイル"),
			}
		}), 1, 8).Draw(rt, "項目")
		srcs, locs := renderOverlap(items, files)
		matches, _ := filepath.Glob(filepath.Join(root, ".vanda", "promises", "p*.toml"))
		for _, m := range matches {
			if err := os.Remove(m); err != nil {
				rt.Fatalf("%v", err)
			}
		}
		for path, src := range srcs {
			writeFile(rt, root, path, src)
		}
		want := expectedOverlaps(items, locs)

		cmd := exec.Command(bin, "check", ".vanda/checks/shape.toml")
		cmd.Dir = root
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exitErr *exec.ExitError
		if err != nil && !errors.As(err, &exitErr) {
			rt.Fatalf("vanda を動かせない: %v", err)
		}
		if (len(want) == 0) != (err == nil) {
			rt.Fatalf("重なりが %d 件なのに、終了は %v\n%s", len(want), err, stderr.String())
		}
		got := map[overlapLoc]string{}
		for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
			if line == "" {
				continue
			}
			var loc overlapLoc
			found := false
			for l, id := range want {
				if strings.HasPrefix(line, fmt.Sprintf("%s:%d: ", l.path, l.line)) {
					if !strings.Contains(line, id) {
						rt.Fatalf("重なった ID %s がエラーに出ていない: %s", id, line)
					}
					loc, found = l, true
				}
			}
			if !found {
				rt.Fatalf("重なりのほかのエラーが出た: %s\n%s", line, dump(srcs))
			}
			got[loc] = want[loc]
		}
		if len(got) != len(want) {
			rt.Fatalf("重なりのエラーが足りない: want %v, got %v\n%s\n%s", want, got, stderr.String(), dump(srcs))
		}
	})
}

func writeFile(t interface{ Fatalf(string, ...any) }, root, path, src string) {
	abs := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
}

func dump(srcs map[string]string) string {
	var paths []string
	for p := range srcs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "-- %s --\n%s", p, srcs[p])
	}
	return b.String()
}

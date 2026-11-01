package main_test

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// CMD-10 合意のクロスチェック。オラクルは spec/Agreement.tla（spec/AgreementTrace.tla で包んだもの）。
// TLC -simulate が無作為にたどった操作の列を、本物の vanda（一時的な git のリポジトリと .vanda/）に 1 歩ずつ送り、
// 1 歩ごとに、モデルの判定と vanda check の結果が一致するかを比べる。
//
// 比べるもの（監督の決定。案 A）:
//   - Agreed(i): 全部の範囲の agreement の誤りに、その項目が出ないこと
//   - AllPass: 全部の範囲の agreement が通ること
//   - Broken(s): ストーリーの範囲の agreement を --base HEAD で行ったときの誤りに出る項目。--base HEAD では
//     「そのストーリーで変わったもの」が空になるので、外れた合意だけが出る。StoryPass は、この外れた合意の条件だけを比べる
// モデルの changedBy（一度でも変えたストーリー）は、vanda の分かれた点との差分とは意味が違うので比べない。
// ストーリーの範囲の差分は、シナリオテスト（agreement_story_range など）が受け持つ。
//
// 写し方: Items の i1 は約束 item-1、i2 はシナリオのファイル e2e/item-2.txtar。版 n は、約束の文やファイルの中身の v<n>。
// Stories の A・B はストーリーの ID #1・#2。Change(s, i) は中身の書き換えのコミット、Agree(s, i) は合意の記録の書き足しのコミット、
// Revoke(s, i) は、Agree(s, i) で書き足した記録をストーリーの合意の記録のファイルから消すコミット。
// Revoke は、POST /api/revoke でなくファイルを直接書き換えて送る。トレースはすべて main の上で進むので、取り消せる範囲
// （今のブランチで書いた記録だけ）に入る記録がない。また Agree もファイルを直接書いて送っており、比べるのは判定だけ。
// 人はテスト担当 1 人に固定する（モデルは人を区別しない）。人が複数のときの取り消しは、API のテストが受け持つ。

const (
	xcheckDefaultTraces = 150
	xcheckDefaultDepth  = 12
	// 場面ごとに、全部のトレースで少なくともこれだけ出ることを求める。取り消しを足した後、150 本・深さ 12 を 10 通りの
	// 種で数えると、いちばん少ない場面（取り消した後も、ほかのストーリーの合意で合意済のまま）でも 58〜85 回（平均 75）
	// だった。30 本では 16〜37 回で、種によって下回った。種によってたまたま下回らないよう、その半分ほどにした
	xcheckMinScenes = 30
)

var xcheckStory = map[string]string{"A": "#1", "B": "#2"}

// xcheckSeed は、TLC に渡す種を返す。既定では実行のたびに変え、毎回同じトレースしか試さないことを防ぐ。
// 落ちた場面を再現するときだけ、VANDA_XCHECK_SEED で種を決める。使った種は、通っても落ちても記録に出す。
func xcheckSeed(t *testing.T) int {
	t.Helper()
	if os.Getenv("VANDA_XCHECK_SEED") != "" {
		return envInt(t, "VANDA_XCHECK_SEED", 0)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		t.Fatalf("種を作れない: %v", err)
	}
	return int(n.Int64())
}

func envInt(t *testing.T, name string, def int) int {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("%s=%q は整数でない", name, v)
	}
	return n
}

// lookTLC は tlc の場所を返す。make check（と CI）は VANDA_REQUIRE_TLC=1 を立てるので、そこでは tlc がなければ落とす。
// 黙って飛ばされて、CI で何も確かめていないことに気づかないのを防ぐため。
func lookTLC(t *testing.T) string {
	t.Helper()
	tlc, err := exec.LookPath("tlc")
	if err == nil {
		return tlc
	}
	if os.Getenv("VANDA_REQUIRE_TLC") == "1" {
		t.Fatalf("VANDA_REQUIRE_TLC=1 なのに tlc がない（nix develop の中で動かす）")
	}
	t.Skip("tlc がないので飛ばす（nix develop の中で動かす）")
	return ""
}

// generateTraces は、TLC に無作為にたどらせたトレースを、1 本ずつ状態の並びにして返す。
func generateTraces(t *testing.T, tlc string, seed, traces, depth int) [][]map[string]any {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"Agreement.tla", "AgreementTrace.tla"} {
		src, err := os.ReadFile(filepath.Join("..", "..", "spec", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := "CONSTANTS\n    Items = {i1, i2}\n    Stories = {A, B}\n    MaxVer = 3\nSPECIFICATION SpecT\nCHECK_DEADLOCK FALSE\n"
	if err := os.WriteFile(filepath.Join(dir, "xcheck.cfg"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	// 種と workers 1 を決めると、毎回同じトレースになる
	cmd := exec.Command(tlc, "-simulate", fmt.Sprintf("file=%s,num=%d", filepath.Join(dir, "tr"), traces),
		"-depth", strconv.Itoa(depth), "-seed", strconv.Itoa(seed), "-workers", "1", "-noGenerateSpecTE",
		"-metadir", filepath.Join(dir, "meta"), "-config", "xcheck.cfg", "AgreementTrace.tla")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("TLC が失敗した: %v\n%s", err, out)
	}
	files, err := filepath.Glob(filepath.Join(dir, "tr_*"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(files, func(a, b int) bool { return traceNo(files[a]) < traceNo(files[b]) })
	var out [][]map[string]any
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		states, err := parseTraceFile(string(src))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out = append(out, states)
	}
	if len(out) == 0 {
		t.Fatal("TLC がトレースを書き出さなかった")
	}
	return out
}

func traceNo(path string) int {
	n, _ := strconv.Atoi(path[strings.LastIndex(path, "_")+1:])
	return n
}

// ---- モデルの状態から値を取り出す ----

type xcheckState struct {
	step       tlaTuple
	content    map[string]int
	agreements []tlaRecord
	changedBy  map[string][]string
	agreed     map[string]bool
	broken     map[string][]string
	allPass    bool
}

func decodeState(raw map[string]any) (xcheckState, error) {
	var st xcheckState
	var ok bool
	if st.step, ok = raw["step"].(tlaTuple); !ok {
		return st, fmt.Errorf("step がない: %v", raw)
	}
	st.content = map[string]int{}
	content, ok := raw["content"].(tlaFunc)
	if !ok {
		return st, fmt.Errorf("content がない: %v", raw)
	}
	for k, v := range content {
		n, ok := v.(int)
		if !ok {
			return st, fmt.Errorf("content[%s] が整数でない", k)
		}
		st.content[k] = n
	}
	set, ok := raw["agreements"].(tlaSet)
	if !ok {
		return st, fmt.Errorf("agreements がない: %v", raw)
	}
	for _, a := range set {
		rec, ok := a.(tlaRecord)
		if !ok {
			return st, fmt.Errorf("合意が表でない: %v", a)
		}
		st.agreements = append(st.agreements, rec)
	}
	st.changedBy = map[string][]string{}
	if cb, ok := raw["changedBy"].(tlaFunc); ok {
		for k, v := range cb {
			st.changedBy[k] = modelNames(v)
		}
	}
	obs, ok := raw["obs"].(tlaRecord)
	if !ok {
		return st, fmt.Errorf("obs がない: %v", raw)
	}
	st.agreed = map[string]bool{}
	agreed, ok := obs["agreed"].(tlaFunc)
	if !ok {
		return st, fmt.Errorf("obs.agreed がない")
	}
	for k, v := range agreed {
		st.agreed[k], _ = v.(bool)
	}
	st.broken = map[string][]string{}
	broken, ok := obs["broken"].(tlaFunc)
	if !ok {
		return st, fmt.Errorf("obs.broken がない")
	}
	for k, v := range broken {
		st.broken[k] = modelNames(v)
	}
	if st.allPass, ok = obs["allPass"].(bool); !ok {
		return st, fmt.Errorf("obs.allPass がない")
	}
	return st, nil
}

func modelNames(v any) []string {
	var out []string
	if set, ok := v.(tlaSet); ok {
		for _, e := range set {
			if m, ok := e.(tlaModel); ok {
				out = append(out, string(m))
			}
		}
	}
	sort.Strings(out)
	return out
}

// ---- 入力の偏り ----

// countScenes は、見たい場面がトレースに何回出たかを、モデルの状態から数える。
func countScenes(states []xcheckState) map[string]int {
	c := map[string]int{}
	for k, st := range states {
		for _, s := range []string{"A", "B"} {
			if len(st.broken[s]) > 0 {
				c["外れた合意がある"]++
			}
			for _, i := range st.broken[s] {
				if st.agreed[i] {
					c["ほかのストーリーの合意で合意済なのに、s の合意は外れている（RV-14）"]++
				}
			}
		}
		for x, a := range st.agreements {
			for _, b := range st.agreements[x+1:] {
				if a["item"] == b["item"] && a["fp"] == b["fp"] && a["story"] != b["story"] {
					c["同じ中身に 2 つのストーリーが合意している"]++
				}
			}
		}
		changed := false
		for _, n := range st.content {
			changed = changed || n > 0
		}
		if changed && st.allPass {
			c["変えた後に、全部の範囲が通る"]++
		}
		if k > 0 && st.step[0] == "Revoke" {
			i := string(st.step[2].(tlaModel))
			if st.agreed[i] {
				c["取り消した後も、ほかのストーリーの合意で合意済のまま"]++
			} else {
				c["取り消して、提案中に戻る"]++
			}
		}
		if k > 0 && st.step[0] == "Change" {
			s, i := string(st.step[1].(tlaModel)), string(st.step[2].(tlaModel))
			for _, a := range states[k-1].agreements {
				if a["item"] == tlaModel(i) && a["story"] == tlaModel(s) {
					c["前に合意したストーリーが、自分で変えて外れる"]++
					break
				}
			}
		}
	}
	return c
}

var xcheckSceneNames = []string{
	"外れた合意がある",
	"ほかのストーリーの合意で合意済なのに、s の合意は外れている（RV-14）",
	"同じ中身に 2 つのストーリーが合意している",
	"変えた後に、全部の範囲が通る",
	"前に合意したストーリーが、自分で変えて外れる",
	"取り消して、提案中に戻る",
	"取り消した後も、ほかのストーリーの合意で合意済のまま",
}

// ---- vanda の側 ----

type xcheckRepo struct {
	t    *testing.T
	bin  string
	root string
	env  []string
}

const xcheckTerm = "[[term]]\nid = \"t\"\nname = \"n\"\ndefinition = \"d\"\n"

func claimTOML(ver int) string {
	return xcheckTerm + fmt.Sprintf("\n[[claim]]\nid = \"item-1\"\nsubjects = [\"t\"]\nstatement = \"v%d\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"test\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n", ver)
}

// claimFingerprint は、docs/formats.md の決まった形の JSON を手で書き、その SHA-256 を返す。
func claimFingerprint(ver int) string {
	return sha256Hex([]byte(fmt.Sprintf(`{"evidence":[{"layers":["model"],"stage":"test","type":"unit-test"}],"id":"item-1","layers":["model"],"statement":"v%d","subjects":["t"]}`, ver)))
}

func fileContent(ver int) string { return fmt.Sprintf("# v%d\n", ver) }

func newXcheckRepo(t *testing.T, bin, version string) *xcheckRepo {
	root := t.TempDir()
	r := &xcheckRepo{t: t, bin: bin, root: root, env: append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CEILING_DIRECTORIES="+filepath.Dir(root),
		"GIT_AUTHOR_NAME=テスト担当", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=テスト担当", "GIT_COMMITTER_EMAIL=tester@example.com")}
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(`version = %q
agreements = [".vanda/agreements/*.toml"]

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"
types = ["scenario-test"]
`, version))
	writeFile(t, root, ".vanda/checks/all.toml", "[[check]]\nname = \"agreement\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/checks/story.toml", "[[check]]\nname = \"agreement\"\nscope = \"story\"\n")
	writeFile(t, root, ".vanda/promises.toml", claimTOML(0))
	writeFile(t, root, "e2e/item-2.txtar", fileContent(0))
	// 用語 t はモデルの項目でないので、はじめから、どのストーリーでもない #9 で合意しておく
	writeFile(t, root, ".vanda/agreements/story-9.toml", agreementTOML("term", "t",
		sha256Hex([]byte(`{"definition":"d","id":"t","name":"n"}`)), "", "#9", "テスト担当", "tester@example.com"))
	r.git("init", "-q", "-b", "main")
	r.commit("#9 はじめの中身")
	return r
}

func (r *xcheckRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	cmd.Env = r.env
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *xcheckRepo) commit(msg string) {
	r.t.Helper()
	// 中身の版だけが違うファイルは大きさが同じで、同じ時刻のうちに書き換えると、git add -A が stat の一致から
	// 変わっていないとみなし、書き換えをコミットに入れないことがある（トレースが偶然そうなったときだけ不一致になる）。
	// 索引から一度外して、毎回すべてを読み直させる
	r.git("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
}

// apply は、モデルの 1 歩（step）を、次の状態の中身（next）に合わせて vanda の側に写す。
func (r *xcheckRepo) apply(k int, next xcheckState) {
	r.t.Helper()
	op, s, i := next.step[0].(string), string(next.step[1].(tlaModel)), string(next.step[2].(tlaModel))
	story := xcheckStory[s]
	ver := next.content[i]
	switch op {
	case "Change":
		if i == "i1" {
			writeFile(r.t, r.root, ".vanda/promises.toml", claimTOML(ver))
		} else {
			writeFile(r.t, r.root, "e2e/item-2.txtar", fileContent(ver))
		}
	case "Agree":
		path := r.agreementsPath(story)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			r.t.Fatal(err)
		}
		_, err = f.WriteString(xcheckRecord(i, ver, story))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			r.t.Fatal(err)
		}
	case "Revoke":
		path := r.agreementsPath(story)
		src, err := os.ReadFile(path)
		if err != nil {
			r.t.Fatalf("%d 歩目: %s の合意の記録のファイルを読めない: %v", k, story, err)
		}
		rec := xcheckRecord(i, ver, story)
		if strings.Count(string(src), rec) != 1 {
			r.t.Fatalf("%d 歩目: モデルでは取り消す記録がちょうど 1 つあるのに、%s に %d 個ある", k, path, strings.Count(string(src), rec))
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(src), rec, "", 1)), 0o644); err != nil {
			r.t.Fatal(err)
		}
	default:
		r.t.Fatalf("知らない操作 %q", op)
	}
	r.commit(fmt.Sprintf("%s %s %s", story, op, i))
}

func (r *xcheckRepo) agreementsPath(story string) string {
	return filepath.Join(r.root, ".vanda", "agreements", "story-"+strings.TrimPrefix(story, "#")+".toml")
}

// xcheckRecord は、項目 i の版 ver への、ストーリー story の合意の記録を返す。
func xcheckRecord(i string, ver int, story string) string {
	if i == "i1" {
		return agreementTOML("claim", "item-1", claimFingerprint(ver), "", story, "テスト担当", "tester@example.com")
	}
	fp := sha256Hex([]byte(fileContent(ver)))
	return agreementTOML("file", fp, fp, "e2e/item-2.txtar", story, "テスト担当", "tester@example.com")
}

// check は vanda check を動かし、通ったかと、誤りに出た項目（i1・i2）を返す。
func (r *xcheckRepo) check(args ...string) (bool, []string, string) {
	r.t.Helper()
	cmd := exec.Command(r.bin, append([]string{"check"}, args...)...)
	cmd.Dir = r.root
	cmd.Env = r.env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !(errors.As(err, &exitErr) && exitErr.ExitCode() == 1) {
		r.t.Fatalf("vanda check %v が終了コード 0・1 のほかで終わった: %v\n%s%s", args, err, stdout.String(), stderr.String())
	}
	items := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if line == "" {
			continue
		}
		i1 := strings.Contains(line, ".vanda/promises.toml") || strings.Contains(line, "item-1")
		i2 := strings.Contains(line, "item-2.txtar")
		switch {
		case i1 && !i2:
			items["i1"] = true
		case i2 && !i1:
			items["i2"] = true
		default:
			r.t.Fatalf("vanda check %v の誤りが、どの項目のものか分からない: %s", args, line)
		}
	}
	var out []string
	for i := range items {
		out = append(out, i)
	}
	sort.Strings(out)
	return err == nil, out, stdout.String() + stderr.String()
}

// @vanda:evidence agreed-when-fingerprint-matches
// @vanda:evidence old-agreements-kept-as-history
// @vanda:evidence detached-agreement
// @vanda:evidence detached-until-story-reagrees
// @vanda:evidence revoke-returns-to-proposed
func Test合意の判定をTLAのモデルと本物のvandaで1歩ずつ比べると一致する(t *testing.T) {
	t.Parallel()
	tlc := lookTLC(t)
	seed := xcheckSeed(t)
	traces := envInt(t, "VANDA_XCHECK_TRACES", xcheckDefaultTraces)
	depth := envInt(t, "VANDA_XCHECK_DEPTH", xcheckDefaultDepth)

	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	raw := generateTraces(t, tlc, seed, traces, depth)
	all := make([][]xcheckState, len(raw))
	scenes := map[string]int{}
	steps := 0
	for n, tr := range raw {
		for k, s := range tr {
			st, err := decodeState(s)
			if err != nil {
				t.Fatalf("トレース %d の %d 歩目: %v", n, k, err)
			}
			all[n] = append(all[n], st)
		}
		for name, c := range countScenes(all[n]) {
			scenes[name] += c
		}
		steps += len(all[n])
	}
	t.Logf("種 %d（VANDA_XCHECK_SEED=%d で同じトレースを流せる）、トレース %d 本、状態 %d", seed, seed, len(all), steps)
	for _, name := range xcheckSceneNames {
		t.Logf("  %s: %d 回", name, scenes[name])
		if scenes[name] < xcheckMinScenes {
			t.Errorf("場面「%s」が %d 回しか出ない（%d 回以上を求める）。トレースの本数か深さを増やす", name, scenes[name], xcheckMinScenes)
		}
	}

	t.Run("トレース", func(t *testing.T) {
		for n, states := range all {
			t.Run(fmt.Sprintf("%02d", n), func(t *testing.T) {
				t.Parallel()
				r := newXcheckRepo(t, bin, version)
				for k, st := range states {
					if k > 0 {
						r.apply(k, st)
					}
					where := fmt.Sprintf("種 %d のトレース %d の %d 歩目（%v）", seed, n, k+1, st.step)
					pass, unagreed, out := r.check(".vanda/checks/all.toml")
					for _, i := range []string{"i1", "i2"} {
						got := !contains(unagreed, i)
						if got != st.agreed[i] {
							t.Fatalf("%s: %s の合意済が、モデルは %v、vanda は %v\n%s", where, i, st.agreed[i], got, out)
						}
					}
					if pass != st.allPass {
						t.Fatalf("%s: 全部の範囲の審査が、モデルは %v、vanda は %v\n%s", where, st.allPass, pass, out)
					}
					for _, s := range []string{"A", "B"} {
						want := st.broken[s]
						pass, broken, out := r.check(".vanda/checks/story.toml", "--base", "HEAD", "--story", xcheckStory[s])
						if strings.Join(broken, ",") != strings.Join(want, ",") {
							t.Fatalf("%s: %s の外れた合意が、モデルは %v、vanda は %v\n%s", where, xcheckStory[s], want, broken, out)
						}
						if pass != (len(want) == 0) {
							t.Fatalf("%s: %s の範囲の審査（外れた合意の条件）が、モデルは %v、vanda は %v\n%s", where, xcheckStory[s], len(want) == 0, pass, out)
						}
					}
				}
			})
		}
	})
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

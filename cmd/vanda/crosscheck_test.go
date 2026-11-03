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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// CMD-10 合意のクロスチェック。オラクルは spec/Agreement.tla（spec/AgreementTrace.tla で包んだもの）。
// TLC -simulate が無作為にたどった操作の列を、本物の vanda（一時的な git のリポジトリと .vanda/）に 1 歩ずつ送り、
// 1 歩ごとに、モデルの判定と vanda check の結果が一致するかを比べる。
//
// 比べるもの:
//   - Passes(i): 全部の範囲の agreement の誤りに、その項目が出ないこと（今ある項目では Agreed(i) と同じ）
//   - AllPass: 全部の範囲の agreement が通ること
//   - StoryPass(s): ストーリーの範囲の agreement を、比べる元をはじめのコミットにして行ったときに通ること。
//     誤りに出る項目は、比べる元と今で違い（Diff）合意済でない項目と、s の外れた合意（Broken(s)）を合わせたもの
// モデルの changedBy（一度でも変えたストーリー）は判定に使わないので比べない。
//
// 写し方: Items の i1 は約束 item-1、i2・i3（Files）はシナリオのファイル e2e/item-2.txtar・e2e/item-3.txtar。
// 中身の指紋 n（1..MaxFp）は、約束の文やファイルの中身の v<n>。0 は、その項目がない（約束リストにない、ファイルがない）こと。
// ファイルは、指紋が同じなら中身が同じ（写し）。Rename(s, i, j) は、i のファイルを j の場所に動かす 1 つのコミット。
// Stories の A・B はストーリーの ID #1・#2。どの操作も 1 つのコミットにする。Agree(s, i) は合意の記録の書き足し
// （i が今なければ、消したことへの合意）、Revoke(s, i) は、Agree(s, i) で書き足した記録をストーリーの合意の記録のファイルから消す。
// Revoke は、POST /api/revoke でなくファイルを直接書き換えて送る。トレースはすべて main の上で進むので、取り消せる範囲
// （今のブランチで書いた記録だけ）に入る記録がない。また Agree もファイルを直接書いて送っており、比べるのは判定だけ。
// 人はテスト担当 1 人に固定する（モデルは人を区別しない）。人が複数のときの取り消しは、API のテストが受け持つ。
//
// 項目を 3 つ、ファイルを 2 つにするのは、名前の変更（ファイルが 2 つ要る）と写し（同じ中身のファイルが 2 つ）を起こすため。
// ファイルの場所が変わらない操作だけでは、場所で見分けても中身で見分けても結果が同じになり、場所の食い違いを見逃す。

const (
	// 項目を 3 つにし、足す・消す・名前を変える操作を足した後、150 本・深さ 12 では、名前を変えた先が合意済になる場面が
	// 5 回ほどしか出なかったので、本数と深さを増やした
	xcheckDefaultTraces = 300
	xcheckDefaultDepth  = 16
	// 場面ごとに、全部のトレースで少なくともこれだけ出ることを求める。300 本・深さ 16 を 3 通りの種で数えると、いちばん少ない
	// 場面（名前を変えた先の場所に、同じ中身への記録があり合意済）でも 32〜40 回だった。種によってたまたま下回らないよう、
	// その半分ほどにした
	xcheckMinScenes = 15
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
	cfg := "CONSTANTS\n    Items = {i1, i2, i3}\n    Files = {i2, i3}\n    Stories = {A, B}\n    MaxFp = 2\nSPECIFICATION SpecT\nCHECK_DEADLOCK FALSE\n"
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
	base       map[string]int
	agreements []tlaRecord
	agreed     map[string]bool
	passes     map[string]bool
	broken     map[string][]string
	storyPass  map[string]bool
	allPass    bool
}

func decodeIntFunc(raw map[string]any, name string) (map[string]int, error) {
	f, ok := raw[name].(tlaFunc)
	if !ok {
		return nil, fmt.Errorf("%s がない: %v", name, raw)
	}
	out := map[string]int{}
	for k, v := range f {
		n, ok := v.(int)
		if !ok {
			return nil, fmt.Errorf("%s[%s] が整数でない", name, k)
		}
		out[k] = n
	}
	return out, nil
}

func decodeBoolFunc(obs tlaRecord, name string) (map[string]bool, error) {
	f, ok := obs[name].(tlaFunc)
	if !ok {
		return nil, fmt.Errorf("obs.%s がない", name)
	}
	out := map[string]bool{}
	for k, v := range f {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("obs.%s[%s] が真偽でない", name, k)
		}
		out[k] = b
	}
	return out, nil
}

func decodeState(raw map[string]any) (xcheckState, error) {
	var st xcheckState
	var ok bool
	var err error
	if st.step, ok = raw["step"].(tlaTuple); !ok {
		return st, fmt.Errorf("step がない: %v", raw)
	}
	if st.content, err = decodeIntFunc(raw, "content"); err != nil {
		return st, err
	}
	if st.base, err = decodeIntFunc(raw, "base"); err != nil {
		return st, err
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
	obs, ok := raw["obs"].(tlaRecord)
	if !ok {
		return st, fmt.Errorf("obs がない: %v", raw)
	}
	if st.agreed, err = decodeBoolFunc(obs, "agreed"); err != nil {
		return st, err
	}
	if st.passes, err = decodeBoolFunc(obs, "passes"); err != nil {
		return st, err
	}
	if st.storyPass, err = decodeBoolFunc(obs, "storyPass"); err != nil {
		return st, err
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

// storyErrItems は、モデルの判定から、s の範囲の審査の誤りに出るはずの項目を返す。比べる元と今で違い合意済でない項目と、
// s の外れた合意を合わせたもの（StoryPass の 2 つの条件のそれぞれを破る項目）。
func (st xcheckState) storyErrItems(s string) []string {
	set := map[string]bool{}
	for _, i := range xcheckItems {
		if st.content[i] != st.base[i] && !st.agreed[i] {
			set[i] = true
		}
	}
	for _, i := range st.broken[s] {
		set[i] = true
	}
	var out []string
	for i := range set {
		out = append(out, i)
	}
	sort.Strings(out)
	return out
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

var xcheckItems = []string{"i1", "i2", "i3"}

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
			if !st.storyPass[s] && len(st.broken[s]) == 0 {
				c["外れた合意はないが、比べる元と違う項目が合意済でなく、ストーリーの範囲で止まる"]++
			}
		}
		for x, a := range st.agreements {
			for _, b := range st.agreements[x+1:] {
				if a["item"] == b["item"] && a["fp"] == b["fp"] && a["story"] != b["story"] {
					c["同じ中身に 2 つのストーリーが合意している"]++
				}
			}
		}
		diff := false
		for _, i := range xcheckItems {
			diff = diff || st.content[i] != st.base[i]
		}
		if diff && st.allPass {
			c["比べる元と違う項目があり、全部の範囲が通る"]++
		}
		if st.content["i2"] != 0 && st.content["i2"] == st.content["i3"] {
			c["2 つのファイルが同じ中身（写し）"]++
			if st.agreed["i2"] != st.agreed["i3"] {
				c["写しの片方だけが合意済"]++
			}
		}
		for _, a := range st.agreements {
			if a["fp"] == 0 {
				c["消したことへの合意の記録がある"]++
				break
			}
		}
		for _, i := range xcheckItems {
			if st.content[i] == 0 && !st.passes[i] {
				c["合意したことのある項目が今なく、全部の範囲で止まる"]++
			}
		}
		if k == 0 {
			continue
		}
		op := st.step[0].(string)
		i := string(st.step[2].(tlaModel))
		switch op {
		case "Revoke":
			if st.agreed[i] {
				c["取り消した後も、ほかのストーリーの合意で合意済のまま"]++
			} else {
				c["取り消して、提案中に戻る"]++
			}
			if st.content[i] == 0 {
				c["消したことへの合意を取り消す"]++
			}
		case "Change":
			s := string(st.step[1].(tlaModel))
			for _, a := range states[k-1].agreements {
				if a["item"] == tlaModel(i) && a["story"] == tlaModel(s) {
					c["前に合意したストーリーが、自分で変えて外れる"]++
					break
				}
			}
		case "Add":
			c["足す"]++
			if st.agreed[i] {
				c["足し直した中身に、前の合意の記録があり合意済"]++
			}
		case "Delete":
			c["消す"]++
		case "Rename":
			c["名前を変える"]++
			if j := string(st.step[3].(tlaModel)); st.agreed[j] {
				c["名前を変えた先の場所に、同じ中身への記録があり合意済（名前を元に戻すなど）"]++
			} else {
				c["名前を変えた先が提案中"]++
			}
		case "Agree":
			if st.content[i] == 0 {
				c["消したことに合意する"]++
			}
		}
	}
	return c
}

var xcheckSceneNames = []string{
	"外れた合意がある",
	"ほかのストーリーの合意で合意済なのに、s の合意は外れている（RV-14）",
	"外れた合意はないが、比べる元と違う項目が合意済でなく、ストーリーの範囲で止まる",
	"同じ中身に 2 つのストーリーが合意している",
	"比べる元と違う項目があり、全部の範囲が通る",
	"前に合意したストーリーが、自分で変えて外れる",
	"取り消して、提案中に戻る",
	"取り消した後も、ほかのストーリーの合意で合意済のまま",
	"足す",
	"足し直した中身に、前の合意の記録があり合意済",
	"消す",
	"名前を変える",
	"名前を変えた先が提案中",
	"名前を変えた先の場所に、同じ中身への記録があり合意済（名前を元に戻すなど）",
	"2 つのファイルが同じ中身（写し）",
	"写しの片方だけが合意済",
	"消したことに合意する",
	"消したことへの合意の記録がある",
	"消したことへの合意を取り消す",
	"合意したことのある項目が今なく、全部の範囲で止まる",
}

// ---- vanda の側 ----

type xcheckRepo struct {
	t    *testing.T
	bin  string
	root string
	env  []string
	base string // はじめのコミット。ストーリーの範囲の審査の比べる元（モデルの base）
}

var xcheckPaths = map[string]string{"i2": "e2e/item-2.txtar", "i3": "e2e/item-3.txtar"}

const xcheckTerm = "[[term]]\nid = \"t\"\nname = \"n\"\ndefinition = \"d\"\n"

func claimTOML(ver int) string {
	return fmt.Sprintf("\n[[claim]]\nid = \"item-1\"\nsubjects = [\"t\"]\nstatement = \"v%d\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n", ver)
}

// claimFingerprint は、docs/formats.md の決まった形の JSON を手で書き、その SHA-256 を返す。
func claimFingerprint(ver int) string {
	return sha256Hex([]byte(fmt.Sprintf(`{"evidence":[{"layers":["model"],"stage":"implementation","type":"unit-test"}],"id":"item-1","layers":["model"],"statement":"v%d","subjects":["t"]}`, ver)))
}

func fileContent(ver int) string { return fmt.Sprintf("# v%d\n", ver) }

// newXcheckRepo は、モデルのはじめの状態 init の中身のリポジトリを作り、はじめのコミットにする。
func newXcheckRepo(t *testing.T, bin, version string, init xcheckState) *xcheckRepo {
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
	r.sync(init)
	// 用語 t はモデルの項目でないので、はじめから、どのストーリーでもない #9 で合意しておく
	writeFile(t, root, ".vanda/agreements/story-9.toml", agreementTOML("term", "t",
		sha256Hex([]byte(`{"definition":"d","id":"t","name":"n"}`)), "", "#9", "テスト担当", "tester@example.com"))
	r.git("init", "-q", "-b", "main")
	r.commit("はじめの中身", "#9")
	r.base = r.git("rev-parse", "HEAD")
	return r
}

func (r *xcheckRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	cmd.Env = r.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit は、作業ツリーのすべてを、ストーリー story のコミットにする。
func (r *xcheckRepo) commit(subject, story string) {
	r.t.Helper()
	(&rvGit{t: r.t, root: r.root, env: r.env}).commitAll(subject, story)
}

// sync は、項目の中身を、モデルの状態 st の content に合わせて作業ツリーに書く。0 なら、その項目をなくす。
func (r *xcheckRepo) sync(st xcheckState) {
	r.t.Helper()
	promises := xcheckTerm
	if n := st.content["i1"]; n > 0 {
		promises += claimTOML(n)
	}
	writeFile(r.t, r.root, ".vanda/promises.toml", promises)
	for i, p := range xcheckPaths {
		if n := st.content[i]; n > 0 {
			writeFile(r.t, r.root, p, fileContent(n))
		} else if err := os.Remove(filepath.Join(r.root, filepath.FromSlash(p))); err != nil && !os.IsNotExist(err) {
			r.t.Fatal(err)
		}
	}
}

// apply は、モデルの 1 歩（step）を、次の状態（next）に合わせて vanda の側に写す。
func (r *xcheckRepo) apply(k int, next xcheckState) {
	r.t.Helper()
	op, s, i := next.step[0].(string), string(next.step[1].(tlaModel)), string(next.step[2].(tlaModel))
	story := xcheckStory[s]
	ver := next.content[i]
	switch op {
	case "Change", "Add", "Delete", "Rename":
		r.sync(next)
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
	r.commit(fmt.Sprintf("%s %s", op, i), story)
}

func (r *xcheckRepo) agreementsPath(story string) string {
	return filepath.Join(r.root, ".vanda", "agreements", "story-"+strings.TrimPrefix(story, "#")+".toml")
}

// xcheckRecord は、項目 i の中身 ver への、ストーリー story の合意の記録を返す。ver が 0 なら、i を消したことへの合意の記録。
func xcheckRecord(i string, ver int, story string) string {
	const name, email = "テスト担当", "tester@example.com"
	if i == "i1" {
		if ver == 0 {
			return deletionAgreementTOML("claim", "item-1", "", story, name, email)
		}
		return agreementTOML("claim", "item-1", claimFingerprint(ver), "", story, name, email)
	}
	path := xcheckPaths[i]
	if ver == 0 {
		return deletionAgreementTOML("file", path, path, story, name, email)
	}
	fp := sha256Hex([]byte(fileContent(ver)))
	return agreementTOML("file", fp, fp, path, story, name, email)
}

var xcheckRecordLine = regexp.MustCompile(`^(\.vanda/agreements/[^:]+\.toml):(\d+):`)

// itemOf は、vanda check の誤りの 1 行が、どの項目（i1・i2・i3）のものかを返す。消したファイルには行がないので、
// 合意の記録の行を指す誤りは、その行を含む [[agreement]] の表が指すもので決める。
func (r *xcheckRepo) itemOf(line string) (string, bool) {
	var found []string
	for i, p := range xcheckPaths {
		if strings.Contains(line, p) {
			found = append(found, i)
		}
	}
	if strings.Contains(line, ".vanda/promises.toml") || strings.Contains(line, "item-1") {
		found = append(found, "i1")
	}
	if len(found) == 0 {
		if m := xcheckRecordLine.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			if i, ok := r.recordItem(m[1], n); ok {
				found = append(found, i)
			}
		}
	}
	if len(found) != 1 {
		return "", false
	}
	return found[0], true
}

// recordItem は、合意の記録のファイル file の n 行目を含む [[agreement]] の表が指す項目を返す。
func (r *xcheckRepo) recordItem(file string, n int) (string, bool) {
	src, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(file)))
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(src), "\n")
	start := -1
	for x := 0; x < n && x < len(lines); x++ {
		if strings.TrimSpace(lines[x]) == "[[agreement]]" {
			start = x
		}
	}
	if start < 0 {
		return "", false
	}
	for x := start + 1; x < len(lines) && strings.TrimSpace(lines[x]) != "[[agreement]]"; x++ {
		switch l := strings.TrimSpace(lines[x]); {
		case l == `key = "item-1"`:
			return "i1", true
		case strings.HasPrefix(l, "path = "):
			for i, p := range xcheckPaths {
				if l == fmt.Sprintf("path = %q", p) {
					return i, true
				}
			}
		}
	}
	return "", false
}

// check は vanda check を動かし、通ったかと、誤りに出た項目（i1・i2・i3）を返す。
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
		i, ok := r.itemOf(line)
		if !ok {
			r.t.Fatalf("vanda check %v の誤りが、どの項目のものか分からない: %s", args, line)
		}
		items[i] = true
	}
	var out []string
	for i := range items {
		out = append(out, i)
	}
	sort.Strings(out)
	return err == nil, out, stdout.String() + stderr.String()
}

// @vanda:evidence agreed-when-fingerprint-matches
// @vanda:evidence renamed-file-needs-agreement
// @vanda:evidence deleted-item-needs-agreement
// @vanda:evidence pass-only-when-all-seen
// @vanda:evidence story-pass-only-when-seen
// @vanda:evidence old-agreements-kept-as-history
// @vanda:evidence detached-agreement
// @vanda:evidence detached-until-story-reagrees
// @vanda:evidence revoke-returns-to-proposed
// @vanda:evidence check-agreement-story-scope
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
				r := newXcheckRepo(t, bin, version, states[0])
				for k, st := range states {
					if k > 0 {
						r.apply(k, st)
					}
					where := fmt.Sprintf("種 %d のトレース %d の %d 歩目（%v）", seed, n, k+1, st.step)
					pass, errItems, out := r.check(".vanda/checks/all.toml")
					for _, i := range xcheckItems {
						got := !contains(errItems, i)
						if got != st.passes[i] {
							t.Fatalf("%s: %s が全部の範囲の審査で止まらないかが、モデルは %v、vanda は %v\n%s", where, i, st.passes[i], got, out)
						}
					}
					if pass != st.allPass {
						t.Fatalf("%s: 全部の範囲の審査が、モデルは %v、vanda は %v\n%s", where, st.allPass, pass, out)
					}
					for _, s := range []string{"A", "B"} {
						want := st.storyErrItems(s)
						if st.storyPass[s] != (len(want) == 0) {
							t.Fatalf("%s: テストの数え方の誤り。モデルの StoryPass(%s) は %v なのに、誤りに出るはずの項目が %v", where, s, st.storyPass[s], want)
						}
						pass, got, out := r.check(".vanda/checks/story.toml", "--base", r.base, "--story", xcheckStory[s])
						if strings.Join(got, ",") != strings.Join(want, ",") {
							t.Fatalf("%s: %s の範囲の審査の誤りに出る項目が、モデルは %v（外れた合意 %v）、vanda は %v\n%s", where, xcheckStory[s], want, st.broken[s], got, out)
						}
						if pass != st.storyPass[s] {
							t.Fatalf("%s: %s の範囲の審査が、モデルは %v、vanda は %v\n%s", where, xcheckStory[s], st.storyPass[s], pass, out)
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

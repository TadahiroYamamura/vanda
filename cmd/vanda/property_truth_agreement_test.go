package main_test

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 性質と前提への承認を、vanda check の承認のチェックで確かめる（docs/plans/promise-reorg-shape-test.md の「フィンガープリントと承認」）。
// フィンガープリントの元の JSON は、約束リストの項目から手で書く（docs/formats.md の「フィンガープリント」）。

// ptList は、約束リストのうち、テストの中で変える所。lone・loneTruth は、どこからも指されない性質と前提の ID（空なら書かない）
type ptList struct {
	top, leaf, truth string // 性質 reliable・性質 no-lost-task・前提 disk-works の文
	lone, loneTruth  string
}

var ptBase = ptList{top: "頼れる", leaf: "登録したタスクは失われない", truth: "ディスクに書いたものは、読み戻せる", lone: "fast", loneTruth: "net-works"}

// 性質 no-lost-task は性質 reliable を refines し、前提 disk-works を assumes する。機能 add-task は no-lost-task を refines し、
// 用語 task と仕様 c は disk-works を assumes する
func (l ptList) promises() string {
	var b strings.Builder
	fmt.Fprintf(&b, `[[property]]
id = "reliable"
statement = %q

[[property]]
id = "no-lost-task"
statement = %q
refines = ["reliable"]
assumes = ["disk-works"]

[[truth]]
id = "disk-works"
statement = %q

[[term]]
id = "task"
name = "タスク"
definition = "やること"
assumes = ["disk-works"]

[[feature]]
id = "add-task"
name = "タスクの登録"
definition = "タスクを足す"
refines = ["no-lost-task"]

[[spec]]
id = "c"
refines = ["add-task"]
assumes = ["disk-works"]
statement = "タスクは重ならない"
layers = ["model"]

[[spec.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`, l.top, l.leaf, l.truth)
	if l.lone != "" {
		fmt.Fprintf(&b, "\n[[property]]\nid = %q\nstatement = \"速い\"\n", l.lone)
	}
	if l.loneTruth != "" {
		fmt.Fprintf(&b, "\n[[truth]]\nid = %q\nstatement = \"網はつながる\"\n", l.loneTruth)
	}
	return b.String()
}

func ptJSONStr(s string) string { return `"` + s + `"` }

func ptTopJSON(stmt string) string {
	return `{"id":"reliable","statement":` + ptJSONStr(stmt) + `}`
}

func ptLeafJSON(stmt string) string {
	return `{"assumes":["disk-works"],"id":"no-lost-task","refines":["reliable"],"statement":` + ptJSONStr(stmt) + `}`
}

func ptTruthJSON(stmt string) string {
	return `{"id":"disk-works","statement":` + ptJSONStr(stmt) + `}`
}

func ptLoneJSON(id string) string { return `{"id":` + ptJSONStr(id) + `,"statement":"速い"}` }

func ptLoneTruthJSON(id string) string {
	return `{"id":` + ptJSONStr(id) + `,"statement":"網はつながる"}`
}

const (
	ptTermJSON    = `{"assumes":["disk-works"],"definition":"やること","id":"task","name":"タスク"}`
	ptFeatureJSON = `{"definition":"タスクを足す","id":"add-task","name":"タスクの登録","refines":["no-lost-task"]}`
	ptSpecJSON    = `{"assumes":["disk-works"],"evidence":[{"layers":["model"],"stage":"implementation","type":"unit-test"}],"id":"c","layers":["model"],"refines":["add-task"],"statement":"タスクは重ならない"}`
)

// ptIDs は、テストの約束リストに出てくるすべての ID。誤りの行がどの項目を指すかを、この中から探す
var ptIDs = []string{"reliable", "no-lost-task", "disk-works", "task", "add-task", "c", "fast", "quick", "net-works", "network-works"}

// ptMentioned は、チェックの出力の誤りの行（約束リストか承認の記録の場所で始まる行）が指す項目を、"<種類の言葉> <ID>" の形で返す。
// 消したことへの承認を求める行は、終わりに "（消した）" を付ける。ID は、ほかの ID の一部（task と no-lost-task）と取り違えないよう、前後の文字で区切る
func ptMentioned(out string) []string {
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, ".vanda/") {
			continue
		}
		for _, id := range ptIDs {
			if !regexp.MustCompile(`(^|[^a-z0-9-])` + regexp.QuoteMeta(id) + `($|[^a-z0-9-])`).MatchString(line) {
				continue
			}
			word := "?"
			for _, w := range []string{"性質", "前提", "用語", "機能", "仕様"} {
				if strings.Contains(line, w+" "+id) {
					word = w
				}
			}
			s := word + " " + id
			if strings.Contains(line, "消した") {
				s += "（消した）"
			}
			set[s] = true
		}
	}
	out2 := []string{}
	for s := range set {
		out2 = append(out2, s)
	}
	sort.Strings(out2)
	return out2
}

type ptRepo struct {
	t         *testing.T
	bin, root string
	g         *rvGit
	records   string
}

func newPTRepo(t *testing.T) *ptRepo {
	t.Helper()
	bin, version := refSetup(t)
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(refShapeConfig, version))
	writeFile(t, root, ".vanda/checks/all.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n\n[[check]]\nname = \"agreement\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/checks/story.toml", "[[check]]\nname = \"agreement\"\nscope = \"story\"\n")
	return &ptRepo{t: t, bin: bin, root: root, g: g}
}

func (r *ptRepo) agree(story, kind, id, json string) {
	r.records += agreementTOML(kind, id, sha256Hex([]byte(json)), "", story, "テスト担当", "tester@example.com")
}

func (r *ptRepo) agreeDeleted(story, kind, id string) {
	r.records += deletionAgreementTOML(kind, id, "", story, "テスト担当", "tester@example.com")
}

func (r *ptRepo) commit(l ptList, subject, story string) {
	writeFile(r.t, r.root, ".vanda/promises.toml", l.promises())
	writeFile(r.t, r.root, ".vanda/agreements.toml", r.records)
	r.g.commitAll(subject, story)
}

// check は、チェックを動かし、誤りの行が指す項目が want とちょうど合うことを確かめる。want が空なら、チェックが通ることを確かめる
func (r *ptRepo) check(what string, want []string, args ...string) {
	r.t.Helper()
	if len(args) == 0 {
		args = []string{".vanda/checks/all.toml"}
	}
	code, out := runCheck(r.t, r.bin, r.root, args...)
	if len(want) == 0 {
		if code != 0 {
			r.t.Errorf("%s: チェックが通らない（%d）\n%s", what, code, out)
		}
		return
	}
	if code != 1 {
		r.t.Errorf("%s: チェックが止まらない（%d）\n%s", what, code, out)
		return
	}
	sort.Strings(want)
	if got := ptMentioned(out); fmt.Sprint(got) != fmt.Sprint(want) {
		r.t.Errorf("%s: 誤りの行が指す項目 %v（思ったもの %v）\n%s", what, got, want, out)
	}
}

func Test性質と前提は足すか変えると承認のチェックで止まり承認すると通り消すかIDを変えると消したことへの承認を求める(t *testing.T) {
	t.Parallel()
	r := newPTRepo(t)

	// 用語・機能・仕様だけを承認する。性質と前提は、足しただけで提案中
	r.agree("#1", "term", "task", ptTermJSON)
	r.agree("#1", "feature", "add-task", ptFeatureJSON)
	r.agree("#1", "spec", "c", ptSpecJSON)
	l := ptBase
	r.commit(l, "約束リストを置く", "#1")
	r.check("性質と前提を足した後", []string{"性質 reliable", "性質 no-lost-task", "性質 fast", "前提 disk-works", "前提 net-works"})

	r.agree("#1", "property", "reliable", ptTopJSON(l.top))
	r.agree("#1", "property", "no-lost-task", ptLeafJSON(l.leaf))
	r.agree("#1", "property", "fast", ptLoneJSON("fast"))
	r.agree("#1", "truth", "disk-works", ptTruthJSON(l.truth))
	r.agree("#1", "truth", "net-works", ptLoneTruthJSON("net-works"))
	r.commit(l, "性質と前提を承認する", "#1")
	r.check("性質と前提を承認した後", nil)

	// 前提の文を変える。assumes している性質・用語・仕様の承認は失効しない
	l.truth = "ディスクに書いたものは、いつでも読み戻せる"
	r.commit(l, "前提の文を変える", "#1")
	r.check("前提の文を変えた後", []string{"前提 disk-works"})
	r.agree("#1", "truth", "disk-works", ptTruthJSON(l.truth))
	r.commit(l, "前提を承認する", "#1")
	r.check("変えた前提を承認した後", nil)

	// 機能が refines する性質の文を変える。指している機能の承認は失効しない
	l.leaf = "登録したタスクは、消すまで失われない"
	r.commit(l, "性質の文を変える", "#1")
	r.check("機能が refines する性質の文を変えた後", []string{"性質 no-lost-task"})
	r.agree("#1", "property", "no-lost-task", ptLeafJSON(l.leaf))
	r.commit(l, "性質を承認する", "#1")
	r.check("変えた性質を承認した後", nil)

	// 性質が refines する上の性質の文を変える。指している性質の承認は失効しない
	l.top = "いつも頼れる"
	r.commit(l, "上の性質の文を変える", "#1")
	r.check("性質が refines する性質の文を変えた後", []string{"性質 reliable"})
	// 承認した中身に戻すと、また通る（承認はフィンガープリントで探す）
	l.top = ptBase.top
	r.commit(l, "上の性質の文を戻す", "#1")
	r.check("上の性質の文を承認した中身に戻した後", nil)

	// ID を変えると、中身が同じでも新しいものとして承認を求め、前の ID は消したものになる
	l.lone, l.loneTruth = "quick", "network-works"
	r.commit(l, "性質と前提の ID を変える", "#1")
	r.check("性質と前提の ID を変えた後", []string{"性質 quick", "前提 network-works", "性質 fast（消した）", "前提 net-works（消した）"})
	r.agree("#1", "property", "quick", ptLoneJSON("quick"))
	r.agree("#1", "truth", "network-works", ptLoneTruthJSON("network-works"))
	r.commit(l, "新しい ID を承認する", "#1")
	r.check("新しい ID だけを承認した後", []string{"性質 fast（消した）", "前提 net-works（消した）"})
	r.agreeDeleted("#1", "property", "fast")
	r.agreeDeleted("#1", "truth", "net-works")
	r.commit(l, "消したことを承認する", "#1")
	r.check("前の ID を消したことを承認した後", nil)

	// 承認したことのある性質と前提を消すと、消したことへの承認を求める
	l.lone, l.loneTruth = "", ""
	r.commit(l, "性質と前提を消す", "#1")
	r.check("性質と前提を消した後", []string{"性質 quick（消した）", "前提 network-works（消した）"})
	// 種類の違う、消したことへの承認は数えない
	r.agreeDeleted("#1", "truth", "quick")
	r.agreeDeleted("#1", "property", "network-works")
	r.commit(l, "種類を取り違えて、消したことを承認する", "#1")
	r.check("種類の違う、消したことへの承認を書いた後", []string{"性質 quick（消した）", "前提 network-works（消した）"})
	r.agreeDeleted("#1", "property", "quick")
	r.agreeDeleted("#1", "truth", "network-works")
	r.commit(l, "消したことを承認する", "#1")
	r.check("消したことを承認した後", nil)
}

// ストーリーの失効した承認（docs/formats.md の「失効した承認」）は、性質と前提にも同じに当てる
func Testストーリーが承認した性質と前提をほかのストーリーが変えるとそのストーリーのチェックは承認し直すまで止まる(t *testing.T) {
	t.Parallel()
	r := newPTRepo(t)
	l := ptBase
	for _, story := range []string{"#1", "#2"} {
		r.agree(story, "property", "reliable", ptTopJSON(l.top))
		r.agree(story, "truth", "disk-works", ptTruthJSON(l.truth))
	}
	r.agree("#1", "term", "task", ptTermJSON)
	r.agree("#1", "feature", "add-task", ptFeatureJSON)
	r.agree("#1", "spec", "c", ptSpecJSON)
	r.agree("#1", "property", "no-lost-task", ptLeafJSON(l.leaf))
	r.agree("#1", "property", "fast", ptLoneJSON("fast"))
	r.agree("#1", "truth", "net-works", ptLoneTruthJSON("net-works"))
	r.commit(l, "約束リストを置く", "#1")
	r.check("はじめ", nil)

	// ブランチに #2 と #3 のコミットを置く（ストーリーが 1 つに決まらないので、--story で名指しする）
	r.g.run("switch", "-q", "-c", "story-2-3")
	r.commit(l, "ストーリー #2 の作業", "#2")
	l.top, l.truth = "いつも頼れる", "ディスクに書いたものは、いつでも読み戻せる"
	r.agree("#3", "property", "reliable", ptTopJSON(l.top))
	r.agree("#3", "truth", "disk-works", ptTruthJSON(l.truth))
	r.commit(l, "性質と前提の文を変えて承認する", "#3")
	r.check("ストーリー #3 が変えて承認した後（スコープ all）", nil)
	r.check("ストーリー #3 のチェック", nil, ".vanda/checks/story.toml", "--base", "main", "--story", "#3")
	// #2 は前の中身を承認していた。今の中身に #3 が承認していても、#2 が承認し直すまで失効したまま
	r.check("ストーリー #2 のチェック", []string{"性質 reliable", "前提 disk-works"}, ".vanda/checks/story.toml", "--base", "main", "--story", "#2")
	r.agree("#2", "property", "reliable", ptTopJSON(l.top))
	r.commit(l, "性質を承認し直す", "#2")
	r.check("ストーリー #2 が性質だけを承認し直した後", []string{"前提 disk-works"}, ".vanda/checks/story.toml", "--base", "main", "--story", "#2")
	r.agree("#2", "truth", "disk-works", ptTruthJSON(l.truth))
	r.commit(l, "前提を承認し直す", "#2")
	r.check("ストーリー #2 が前提も承認し直した後", nil, ".vanda/checks/story.toml", "--base", "main", "--story", "#2")
}

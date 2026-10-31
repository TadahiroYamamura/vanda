package main_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// RV-06 合意する（POST /api/agree）。リポジトリは review_repo_test.go の newReviewRepo（HEAD は story-24）。

type agreeResponse struct {
	Commit    string `json:"commit"`
	Agreement struct {
		Kind        string  `json:"kind"`
		Key         string  `json:"key"`
		Fingerprint string  `json:"fingerprint"`
		Path        *string `json:"path"`
		User        struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
		AgreedAt string `json:"agreedAt"`
		Story    string `json:"story"`
	} `json:"agreement"`
}

var (
	reviewClaimFP = sha256Hex([]byte(reviewStoryItemJSON["claim task-id-unique"]))
	reviewTitleFP = sha256Hex([]byte(reviewStoryItemJSON["claim task-title-required"]))
	reviewFileFP  = sha256Hex([]byte(reviewScenarioStory))
)

func agreeClaim(id, fp string) map[string]any {
	return map[string]any{"kind": "claim", "id": id, "path": nil, "fingerprint": fp, "story": reviewStory}
}

func agreeFile(path, fp string) map[string]any {
	return map[string]any{"kind": "file", "id": nil, "path": path, "fingerprint": fp, "story": reviewStory}
}

type agreementRecord struct {
	Kind        string `toml:"kind"`
	Key         string `toml:"key"`
	Fingerprint string `toml:"fingerprint"`
	Path        string `toml:"path"`
	Story       string `toml:"story"`
	AgreedAt    any    `toml:"agreed_at"`
	User        struct {
		Name  string `toml:"name"`
		Email string `toml:"email"`
	} `toml:"user"`
}

func readAgreements(t *testing.T, root string) []agreementRecord {
	t.Helper()
	var doc struct {
		Agreement []agreementRecord `toml:"agreement"`
	}
	if _, err := toml.DecodeFile(filepath.Join(root, ".vanda", "agreements.toml"), &doc); err != nil {
		t.Fatalf("合意の記録を読めない: %v", err)
	}
	return doc.Agreement
}

func setupAgree(t *testing.T) (string, *reviewRepo, *vandaServer) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	return bin, r, startServe(t, bin, r.Root, r.Env)
}

func committedFiles(r *reviewRepo, rev string) []string {
	files := strings.Fields(r.git("show", "--name-only", "--format=", rev))
	sort.Strings(files)
	return files
}

// RV-06・RV-04
func Test合意すると合意の記録が1件増えその約束のフラグだけが外れほかの行は変わらず1つのコミットになる(t *testing.T) {
	_, r, s := setupAgree(t)
	head := r.git("rev-parse", "HEAD")
	before := readAgreements(t, r.Root)

	var got agreeResponse
	if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), &got); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}

	if n := r.git("rev-list", "--count", head+"..HEAD"); n != "1" {
		t.Errorf("コミットが %s 個できた（1 つにする）", n)
	}
	if now := r.git("rev-parse", "HEAD"); got.Commit != now || r.git("rev-parse", "HEAD^") != head {
		t.Errorf("返したコミット %s が、今の HEAD %s（親は元の HEAD）でない", got.Commit, now)
	}
	if files := committedFiles(r, "HEAD"); !reflect.DeepEqual(files, []string{".vanda/agreements.toml", ".vanda/promises.toml"}) {
		t.Errorf("コミットに入ったファイル: %v", files)
	}
	if subject := r.git("log", "-1", "--format=%s"); subject != "(chore) #24 で 約束 task-id-unique に合意する" {
		t.Errorf("コミットのメッセージ: %q", subject)
	}
	if status := r.git("status", "--porcelain"); status != "" {
		t.Errorf("合意の後に、コミットしていない変更が残った:\n%s", status)
	}

	after := readAgreements(t, r.Root)
	if len(after) != len(before)+1 || !reflect.DeepEqual(after[:len(before)], before) {
		t.Fatalf("合意の記録が、元のものの後に 1 件足したものでない（%d 件 → %d 件）", len(before), len(after))
	}
	rec := after[len(after)-1]
	if rec.Kind != "claim" || rec.Key != "task-id-unique" || rec.Fingerprint != reviewClaimFP || rec.Path != "" ||
		rec.Story != reviewStory || rec.User.Name != reviewUserName || rec.User.Email != reviewUserEmail || rec.AgreedAt == nil {
		t.Errorf("書き足した合意の記録: %+v", rec)
	}
	a := got.Agreement
	if a.Kind != "claim" || a.Key != "task-id-unique" || a.Fingerprint != reviewClaimFP || a.Path != nil ||
		a.Story != reviewStory || a.User.Name != reviewUserName || a.User.Email != reviewUserEmail || a.AgreedAt == "" {
		t.Errorf("返した合意の記録: %+v", a)
	}

	// フラグを外すのは、合意した約束の flags と guesses の行だけ。コメント・空行・順・ほかの約束のフラグは変えない
	want := strings.Replace(strings.Replace(reviewPromisesStory,
		"flags = [\"ai-proposal\"]\n", "", 1),
		"guesses = [\"タスクは消しても ID を使い回さない\"]\n", "", 1)
	data, err := os.ReadFile(filepath.Join(r.Root, ".vanda", "promises.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("約束リストが、合意した約束の flags と guesses の行だけを消したものでない\n--- 今\n%s\n--- 思ったもの\n%s", data, want)
	}

	var review reviewResponse
	if code, msg := s.get("/api/review?view=branch&base=main", &review); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	if e := findEntry(review.Entries, "task-id-unique"); e == nil || str(e.State) != "agreed" || len(e.Flags) != 0 {
		t.Errorf("合意した約束が、一覧で合意済・フラグなしになっていない: %+v", e)
	}
}

// RV-06・CMD-10
func Test全部に合意すると段1の由来と合意の審査が通る(t *testing.T) {
	bin, r, s := setupAgree(t)
	if code, _, _ := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main"); code != 1 {
		t.Fatalf("合意する前から、合意の審査が通っている")
	}
	for _, body := range []map[string]any{
		agreeClaim("task-id-unique", reviewClaimFP),
		agreeClaim("task-title-required", reviewTitleFP),
		agreeFile("e2e/add-task.txtar", reviewFileFP),
	} {
		if code, msg := s.post("/api/agree", body, nil); code != 200 {
			t.Fatalf("%v に合意できない（%d）: %s", body, code, msg)
		}
		// 合意のコミットにもストーリーの ID が入るので、途中でも由来の審査は通る
		if code, out, errs := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main"); !strings.Contains(out, "審査 story-origin（story）: 通った") {
			t.Errorf("%v に合意した後、由来の審査が通らない（%d）\n%s%s", body, code, out, errs)
		}
	}
	if code, out, errs := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main"); code != 0 {
		t.Errorf("全部に合意しても、ストーリーの範囲の審査が通らない（%d）\n%s%s", code, out, errs)
	}
	if code, out, errs := r.vanda(bin, "check", ".vanda/checks/all.toml"); code != 0 {
		t.Errorf("全部に合意しても、全部の範囲の審査が通らない（%d）\n%s%s", code, out, errs)
	}
}

// RV-06
func Test合意と関係のない書きかけの変更は索引に足したものも合意のコミットに入らず残る(t *testing.T) {
	_, r, s := setupAgree(t)
	writeFile(t, r.Root, "db/schema.sql", reviewSchema+"-- 書きかけ\n")
	writeFile(t, r.Root, "spec/Task.tla", strings.Replace(reviewTLA, "====", "\\* 書きかけ\n====", 1))
	r.git("add", "spec/Task.tla")
	writeFile(t, r.Root, "notes.md", "メモ\n")

	if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}
	if files := committedFiles(r, "HEAD"); !reflect.DeepEqual(files, []string{".vanda/agreements.toml", ".vanda/promises.toml"}) {
		t.Errorf("コミットに入ったファイル: %v", files)
	}
	if staged := r.git("diff", "--cached", "--name-only"); staged != "spec/Task.tla" {
		t.Errorf("索引に足していた変更が残っていない: %q", staged)
	}
	if unstaged := r.git("diff", "--name-only"); unstaged != "db/schema.sql" {
		t.Errorf("書きかけの変更が残っていない: %q", unstaged)
	}
	if untracked := r.git("ls-files", "--others", "--exclude-standard"); untracked != "notes.md" {
		t.Errorf("git が追っていないファイルが残っていない: %q", untracked)
	}
}

// RV-06
func Test合意で変えるファイルに書きかけの変更があると合意を断り理由を出し何も変えない(t *testing.T) {
	cases := []struct {
		name string
		file string
		body map[string]any
	}{
		{"約束リストに書きかけがあるときの約束への合意", ".vanda/promises.toml", agreeClaim("task-id-unique", reviewClaimFP)},
		{"合意の記録に書きかけがあるときの約束への合意", ".vanda/agreements.toml", agreeClaim("task-id-unique", reviewClaimFP)},
		{"合意の記録に書きかけがあるときのファイルへの合意", ".vanda/agreements.toml", agreeFile("e2e/add-task.txtar", reviewFileFP)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, r, s := setupAgree(t)
			data, err := os.ReadFile(filepath.Join(r.Root, c.file))
			if err != nil {
				t.Fatal(err)
			}
			dirty := string(data) + "\n# 書きかけ\n"
			writeFile(t, r.Root, c.file, dirty)
			head := r.git("rev-parse", "HEAD")

			if code, msg := s.post("/api/agree", c.body, nil); code != 409 || msg == "" {
				t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
			}
			if now := r.git("rev-parse", "HEAD"); now != head {
				t.Errorf("断ったのにコミットができた")
			}
			if data, _ := os.ReadFile(filepath.Join(r.Root, c.file)); string(data) != dirty {
				t.Errorf("断ったのに %s が書き換わった", c.file)
			}
			// rvGit.run は前後の空白を落とすので、「 M」の前の空白はない
			if status := r.git("status", "--porcelain"); status != "M "+c.file {
				t.Errorf("断ったのに、ほかのファイルが変わった:\n%s", status)
			}
		})
	}
}

// RV-06
func Test合意する人のgitのuser_nameかuser_emailがないと合意できず理由を出す(t *testing.T) {
	for _, key := range []string{"user.name", "user.email"} {
		t.Run(key, func(t *testing.T) {
			bin := buildVanda(t)
			version, err := runningVersion(bin)
			if err != nil {
				t.Fatal(err)
			}
			r := newReviewRepo(t, version)
			r.unsetUser(key)
			s := startServe(t, bin, r.Root, r.Env)
			head := r.git("rev-parse", "HEAD")

			var ctx struct {
				User *struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"user"`
			}
			if code, msg := s.get("/api/context", &ctx); code != 200 || ctx.User != nil {
				t.Errorf("%s がないのに、context の user が null でない（%d %q）: %+v", key, code, msg, ctx.User)
			}
			if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 400 || msg == "" {
				t.Errorf("%d（%q）を返した。400 で断り、理由を出す", code, msg)
			}
			if r.git("rev-parse", "HEAD") != head || r.git("status", "--porcelain") != "" {
				t.Errorf("断ったのに、リポジトリが変わった")
			}
		})
	}
}

// RV-06
func Testファイルにも合意でき合意の記録だけが1つのコミットになる(t *testing.T) {
	_, r, s := setupAgree(t)
	head := r.git("rev-parse", "HEAD")
	before := readAgreements(t, r.Root)

	var got agreeResponse
	if code, msg := s.post("/api/agree", agreeFile("e2e/add-task.txtar", reviewFileFP), &got); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}
	if n := r.git("rev-list", "--count", head+"..HEAD"); n != "1" {
		t.Errorf("コミットが %s 個できた", n)
	}
	if files := committedFiles(r, "HEAD"); !reflect.DeepEqual(files, []string{".vanda/agreements.toml"}) {
		t.Errorf("コミットに入ったファイル: %v", files)
	}
	if subject := r.git("log", "-1", "--format=%s"); subject != "(chore) #24 で ファイル e2e/add-task.txtar に合意する" {
		t.Errorf("コミットのメッセージ: %q", subject)
	}
	after := readAgreements(t, r.Root)
	if len(after) != len(before)+1 {
		t.Fatalf("合意の記録が %d 件 → %d 件", len(before), len(after))
	}
	rec := after[len(after)-1]
	if rec.Kind != "file" || rec.Key != reviewFileFP || rec.Fingerprint != reviewFileFP || rec.Path != "e2e/add-task.txtar" ||
		rec.Story != reviewStory || rec.User.Name != reviewUserName || rec.User.Email != reviewUserEmail {
		t.Errorf("書き足した合意の記録: %+v", rec)
	}
	if got.Agreement.Path == nil || *got.Agreement.Path != "e2e/add-task.txtar" || got.Agreement.Key != reviewFileFP {
		t.Errorf("返した合意の記録: %+v", got.Agreement)
	}
}

// RV-06: 画面に見せていた中身と今の中身が違えば断る
func Test見せていた中身の指紋が今の中身と違うと合意を断る(t *testing.T) {
	_, r, s := setupAgree(t)
	head := r.git("rev-parse", "HEAD")
	old := sha256Hex([]byte(reviewItemJSON["claim task-id-unique"]))
	if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", old), nil); code != 409 || msg == "" {
		t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
	}
	if r.git("rev-parse", "HEAD") != head {
		t.Errorf("断ったのにコミットができた")
	}
}

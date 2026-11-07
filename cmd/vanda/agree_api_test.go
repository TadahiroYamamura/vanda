package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

// RV-06 合意する（POST /api/agree）。リポジトリは review_repo_test.go の newReviewRepo（HEAD は story-24）。

type agreeResponse struct {
	Commit    string `json:"commit"`
	Agreement struct {
		Kind        string  `json:"kind"`
		Key         string  `json:"key"`
		Fingerprint *string `json:"fingerprint"`
		Deleted     bool    `json:"deleted"`
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
	Deleted     bool   `toml:"deleted"`
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

// wantStoryTrailer は、HEAD のコミットのメッセージの末尾に、Story: <story> のトレーラーがちょうど 1 つあるかを、git のトレーラーの読み方で確かめる。
func wantStoryTrailer(t *testing.T, r *reviewRepo, story string) {
	t.Helper()
	if got := r.git("log", "-1", "--format=%(trailers:key=Story,valueonly)"); got != story {
		t.Errorf("コミットの Story: のトレーラーが %q（%s のはず）\n%s", got, story, r.git("log", "-1", "--format=%B"))
	}
}

// RV-06・RV-04
// @vanda:evidence agree-writes-and-commits
// @vanda:evidence agree-removes-flags
// @vanda:evidence agree-commit-has-story-trailer
func Test合意すると合意の記録が1件増えその約束のフラグだけが外れほかの行は変わらず1つのコミットになる(t *testing.T) {
	t.Parallel()
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
	wantStoryTrailer(t, r, reviewStory)
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
	if a.Kind != "claim" || a.Key != "task-id-unique" || str(a.Fingerprint) != reviewClaimFP || a.Path != nil ||
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
// @vanda:evidence agree-writes-and-commits
func Test全部に合意すると段1の由来と合意の審査が通る(t *testing.T) {
	t.Parallel()
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
// @vanda:evidence agree-writes-and-commits
func Test合意と関係のない書きかけの変更は索引に足したものも合意のコミットに入らず残る(t *testing.T) {
	t.Parallel()
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
// @vanda:evidence agree-rejects-uncommitted
func Test合意で変えるファイルに書きかけの変更があると合意を断り理由を出し何も変えない(t *testing.T) {
	t.Parallel()
	dirtyScenario := reviewScenarioStory + "# 書きかけ\n"
	cases := []struct {
		name string
		file string
		body map[string]any
		// 合意の対象が書いてあるファイルの書きかけは、先にコミットするよう理由に書く（docs/formats.md）
		hint bool
	}{
		{"約束リストに書きかけがあるときの約束への合意", ".vanda/promises.toml", agreeClaim("task-id-unique", reviewClaimFP), true},
		{"合意の記録に書きかけがあるときの約束への合意", ".vanda/agreements.toml", agreeClaim("task-id-unique", reviewClaimFP), false},
		{"合意の記録に書きかけがあるときのファイルへの合意", ".vanda/agreements.toml", agreeFile("e2e/add-task.txtar", reviewFileFP), false},
		// 見せていた指紋は書きかけの中身のもの（指紋の違いで断られるのでなく、書きかけで断られることを見るため）
		{"合意するファイルそのものに書きかけがあるときのファイルへの合意", "e2e/add-task.txtar", agreeFile("e2e/add-task.txtar", sha256Hex([]byte(dirtyScenario))), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, r, s := setupAgree(t)
			data, err := os.ReadFile(filepath.Join(r.Root, c.file))
			if err != nil {
				t.Fatal(err)
			}
			dirty := string(data) + "# 書きかけ\n"
			writeFile(t, r.Root, c.file, dirty)
			head := r.git("rev-parse", "HEAD")

			code, msg := s.post("/api/agree", c.body, nil)
			if code != 409 || msg == "" {
				t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
			}
			if c.hint && !strings.Contains(msg, "コミット") {
				t.Errorf("理由に、先にコミットするよう書いていない: %q", msg)
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
// @vanda:evidence context-no-user-cannot-agree
func Test合意する人のgitのuser_nameかuser_emailがないと合意できず理由を出す(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"user.name", "user.email"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
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
// @vanda:evidence agree-writes-and-commits
func Testファイルにも合意でき合意の記録だけが1つのコミットになる(t *testing.T) {
	t.Parallel()
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

// RV-06: 同じストーリーで、今の中身にもう合意していれば断る
// @vanda:evidence agree-rejects-duplicate
func Test同じストーリーで今の中身にもう合意していれば合意を断る(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	if code, msg := s.post("/api/agree", agreeFile("e2e/add-task.txtar", reviewFileFP), nil); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}
	head := r.git("rev-parse", "HEAD")
	before := readAgreements(t, r.Root)
	if code, msg := s.post("/api/agree", agreeFile("e2e/add-task.txtar", reviewFileFP), nil); code != 409 || msg == "" {
		t.Errorf("2 度目の合意に %d（%q）を返した。409 で断り、理由を出す", code, msg)
	}
	if r.git("rev-parse", "HEAD") != head || len(readAgreements(t, r.Root)) != len(before) {
		t.Errorf("断ったのに、コミットか合意の記録が増えた")
	}
}

// RV-06: 画面に見せていた中身と今の中身が違えば断る
// @vanda:evidence agree-rejects-stale
func Test見せていた中身の指紋が今の中身と違うと合意を断る(t *testing.T) {
	t.Parallel()
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

// ---- RV-17 合意を取り消す（POST /api/revoke） ----

func revokeClaim(id, fp string) map[string]any {
	return map[string]any{"kind": "claim", "id": id, "path": nil, "fingerprint": fp, "story": reviewStory, "base": "main"}
}

type revokeResponse struct {
	Commit      string `json:"commit"`
	StillAgreed bool   `json:"stillAgreed"`
	Removed     int    `json:"removed"`
}

// inlineRecord は、docs/formats.md の user = { … } の形で、task-id-unique の今の中身への合意の記録を書く（前に空行 1 つ）。
func inlineRecord(story, name, email string) string {
	return fmt.Sprintf("\n[[agreement]]\nkind = \"claim\"\nkey = \"task-id-unique\"\nfingerprint = %q\nagreed_at = 2026-10-31T12:00:00+09:00\nstory = %q\nuser = { name = %q, email = %q }\n",
		reviewClaimFP, story, name, email)
}

func readFileString(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// setupRevoke は、API で合意して task-id-unique のフラグを外し、合意の記録のファイルを、ベースブランチの中身 base に
// extra を足したものに書き換えてコミットする。base（返すもの）は、合意する前（ベースブランチと同じ）の合意の記録のファイル。
func setupRevoke(t *testing.T, extra func(base string) string) (string, *reviewRepo, *vandaServer, string) {
	t.Helper()
	bin, r, s := setupAgree(t)
	base := readFileString(t, r.Root, ".vanda/agreements.toml")
	if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}
	text := extra(base)
	writeFile(t, r.Root, ".vanda/agreements.toml", text)
	r.g.commitAll("合意の記録を書き直す", reviewStory)
	return bin, r, s, base
}

type revokeReview struct {
	Entries []struct {
		ID     *string `json:"id"`
		State  *string `json:"state"`
		Revoke *struct {
			Possible bool    `json:"possible"`
			Reason   *string `json:"reason"`
		} `json:"revoke"`
	} `json:"entries"`
}

func revokeEntry(t *testing.T, s *vandaServer) (state string, possible *bool, reason string) {
	t.Helper()
	var rv revokeReview
	if code, msg := s.get("/api/review?view=branch&base=main&story=%2324", &rv); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	for _, e := range rv.Entries {
		if str(e.ID) == "task-id-unique" {
			if e.Revoke != nil {
				p := e.Revoke.Possible
				return str(e.State), &p, str(e.Revoke.Reason)
			}
			return str(e.State), nil, ""
		}
	}
	t.Fatalf("一覧に task-id-unique がない")
	return "", nil, ""
}

// RV-17
// @vanda:evidence revoke-commits-and-keeps-flags
// @vanda:evidence revoke-own-branch-only
// @vanda:evidence revoke-returns-to-proposed
// @vanda:evidence agree-commit-has-story-trailer
func Test自分が今のブランチで書いた合意を取り消すとその記録と直前の空行だけが消え1つのコミットになりフラグは戻らない(t *testing.T) {
	t.Parallel()
	_, r, s, base := setupRevoke(t, func(base string) string {
		return base + "\n# 以下は #24 の合意\n" + inlineRecord(reviewStory, reviewUserName, reviewUserEmail) + "\n# 終わりのコメント\n"
	})
	want := base + "\n# 以下は #24 の合意\n" + "\n# 終わりのコメント\n"
	promises := readFileString(t, r.Root, ".vanda/promises.toml")
	if state, possible, reason := revokeEntry(t, s); state != "agreed" || possible == nil || !*possible {
		t.Fatalf("取り消せるはずの合意が、一覧で取り消せるとなっていない（state %q、possible %v、reason %q）", state, possible, reason)
	}
	// 関係のない書きかけの変更
	writeFile(t, r.Root, "db/schema.sql", reviewSchema+"-- 書きかけ\n")
	writeFile(t, r.Root, "notes.md", "メモ\n")
	head := r.git("rev-parse", "HEAD")

	var got revokeResponse
	if code, msg := s.post("/api/revoke", revokeClaim("task-id-unique", reviewClaimFP), &got); code != 200 {
		t.Fatalf("取り消せない（%d）: %s", code, msg)
	}
	if n := r.git("rev-list", "--count", head+"..HEAD"); n != "1" {
		t.Errorf("コミットが %s 個できた（1 つにする）", n)
	}
	if got.Commit != r.git("rev-parse", "HEAD") || got.StillAgreed || got.Removed != 1 {
		t.Errorf("返したもの: %+v（commit は今の HEAD、stillAgreed は false、removed は 1）", got)
	}
	if files := committedFiles(r, "HEAD"); !reflect.DeepEqual(files, []string{".vanda/agreements.toml"}) {
		t.Errorf("コミットに入ったファイル: %v", files)
	}
	if subject := r.git("log", "-1", "--format=%s"); subject != "(chore) #24 で 約束 task-id-unique の合意を取り消す" {
		t.Errorf("コミットのメッセージ: %q", subject)
	}
	wantStoryTrailer(t, r, reviewStory)
	if now := readFileString(t, r.Root, ".vanda/agreements.toml"); now != want {
		t.Errorf("合意の記録のファイルが、その記録と直前の空行だけを消したものでない\n--- 今\n%s\n--- 思ったもの\n%s", now, want)
	}
	if now := readFileString(t, r.Root, ".vanda/promises.toml"); now != promises {
		t.Errorf("取り消しで約束リストが変わった（フラグは戻さない）\n%s", now)
	}
	if unstaged := r.git("diff", "--name-only"); unstaged != "db/schema.sql" {
		t.Errorf("書きかけの変更が残っていない: %q", unstaged)
	}
	if untracked := r.git("ls-files", "--others", "--exclude-standard"); untracked != "notes.md" {
		t.Errorf("git が追っていないファイルが残っていない: %q", untracked)
	}
	if state, _, _ := revokeEntry(t, s); state != "proposed" {
		t.Errorf("取り消した後、ほかに合う記録がないのに提案中に戻らない（%q）", state)
	}
}

// RV-17。人の違う記録は残り、同じ人の同じ記録が 2 つあれば両方消える。ほかのストーリーの記録と、前の中身への記録も残る
// @vanda:evidence revoke-own-branch-only
// @vanda:evidence revoke-removes-all-duplicates
// @vanda:evidence revoke-returns-to-proposed
func Test取り消すと今の人の記録だけが消えほかの人とほかのストーリーの記録は残り合意済のままになる(t *testing.T) {
	t.Parallel()
	other := inlineRecord(reviewStory, "ほかの人", "other@example.com")
	story30 := inlineRecord("#30", reviewUserName, reviewUserEmail)
	self := inlineRecord(reviewStory, reviewUserName, reviewUserEmail)
	// 自分が、このストーリーで、前の中身に書いた記録（今の中身への合意ではない）
	stale := strings.Replace(self, reviewClaimFP, sha256Hex([]byte(reviewItemJSON["claim task-id-unique"])), 1)
	_, r, s, base := setupRevoke(t, func(base string) string {
		return base + "\n# #24 の合意\n" + self + other + stale + story30 + self
	})
	want := base + "\n# #24 の合意\n" + other + stale + story30

	var got revokeResponse
	if code, msg := s.post("/api/revoke", revokeClaim("task-id-unique", reviewClaimFP), &got); code != 200 {
		t.Fatalf("取り消せない（%d）: %s", code, msg)
	}
	if !got.StillAgreed || got.Removed != 2 {
		t.Errorf("返したもの: %+v（ほかの人の合意が残るので stillAgreed は true、自分の 2 つを消すので removed は 2）", got)
	}
	if now := readFileString(t, r.Root, ".vanda/agreements.toml"); now != want {
		t.Errorf("合意の記録のファイル\n--- 今\n%s\n--- 思ったもの\n%s", now, want)
	}
	if state, _, _ := revokeEntry(t, s); state != "agreed" {
		t.Errorf("ほかの人の合意が残っているのに、合意済でない（%q）", state)
	}
}

// RV-17。誰が × どこで書いたか、の組み合わせを全部回す
// @vanda:evidence revoke-own-branch-only
// @vanda:evidence revoke-refused
func Test取り消せるのは今の人が今のブランチで書いた合意だけでほかは一覧で取り消せないとなりAPIで断られる(t *testing.T) {
	t.Parallel()
	writers := []struct {
		name, user, email string
		self              bool
	}{
		{"自分", reviewUserName, reviewUserEmail, true},
		{"名前だけ違う人", "ほかの人", reviewUserEmail, false},
		{"メールだけ違う人", reviewUserName, "other@example.com", false},
		{"名前もメールも違う人", "ほかの人", "other@example.com", false},
	}
	for _, w := range writers {
		for _, onBase := range []bool{false, true} {
			place := "今のブランチ"
			if onBase {
				place = "ベースブランチ"
			}
			t.Run(w.name+"が"+place+"で書いた", func(t *testing.T) {
				t.Parallel()
				bin := buildVanda(t)
				version, err := runningVersion(bin)
				if err != nil {
					t.Fatal(err)
				}
				r := newReviewRepo(t, version)
				// フラグを外した今の中身への合意を、手で書く。ベースブランチなら main に書いて取り込む
				promises := readFileString(t, r.Root, ".vanda/promises.toml")
				promises = strings.Replace(strings.Replace(promises, "flags = [\"ai-proposal\"]\n", "", 1), "guesses = [\"タスクは消しても ID を使い回さない\"]\n", "", 1)
				writeFile(t, r.Root, ".vanda/promises.toml", promises)
				r.g.commitAll("フラグを外す", reviewStory)
				rec := inlineRecord(reviewStory, w.user, w.email)
				if onBase {
					r.git("switch", "-q", "main")
					writeFile(t, r.Root, ".vanda/agreements.toml", readFileString(t, r.Root, ".vanda/agreements.toml")+rec)
					r.g.commitAll("合意を書く", reviewStory)
					r.git("switch", "-q", reviewBranch)
					r.git("merge", "-q", "--no-edit", "main")
				} else {
					writeFile(t, r.Root, ".vanda/agreements.toml", readFileString(t, r.Root, ".vanda/agreements.toml")+rec)
					r.g.commitAll("合意を書く", reviewStory)
				}
				s := startServe(t, bin, r.Root, r.Env)
				can := w.self && !onBase
				state, possible, reason := revokeEntry(t, s)
				if state != "agreed" || possible == nil {
					t.Fatalf("手で書いた合意で合意済になっていない（state %q、revoke %v）", state, possible)
				}
				if *possible != can || (!can && reason == "") {
					t.Errorf("一覧の取り消せるかが %v（理由 %q）。%v にする（取り消せないなら理由を出す）", *possible, reason, can)
				}
				before := readFileString(t, r.Root, ".vanda/agreements.toml")
				head := r.git("rev-parse", "HEAD")
				code, msg := s.post("/api/revoke", revokeClaim("task-id-unique", reviewClaimFP), nil)
				if can {
					if code != 200 {
						t.Fatalf("取り消せない（%d）: %s", code, msg)
					}
					if n := r.git("rev-list", "--count", head+"..HEAD"); n != "1" {
						t.Errorf("コミットが %s 個できた", n)
					}
					if now := readFileString(t, r.Root, ".vanda/agreements.toml"); now != strings.Replace(before, rec, "", 1) {
						t.Errorf("取り消した記録だけが消えていない\n%s", now)
					}
					return
				}
				if code != 409 || msg == "" {
					t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
				}
				if r.git("rev-parse", "HEAD") != head || readFileString(t, r.Root, ".vanda/agreements.toml") != before || r.git("status", "--porcelain") != "" {
					t.Errorf("断ったのに、リポジトリが変わった")
				}
			})
		}
	}
}

// RV-17
// @vanda:evidence revoke-commits-and-keeps-flags
func Test合意の記録のファイルに書きかけの変更があると取り消しを断り理由を出し何も変えない(t *testing.T) {
	t.Parallel()
	_, r, s, _ := setupRevoke(t, func(base string) string {
		return base + inlineRecord(reviewStory, reviewUserName, reviewUserEmail)
	})
	dirty := readFileString(t, r.Root, ".vanda/agreements.toml") + "# 書きかけ\n"
	writeFile(t, r.Root, ".vanda/agreements.toml", dirty)
	head := r.git("rev-parse", "HEAD")
	if code, msg := s.post("/api/revoke", revokeClaim("task-id-unique", reviewClaimFP), nil); code != 409 || msg == "" {
		t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
	}
	if r.git("rev-parse", "HEAD") != head || readFileString(t, r.Root, ".vanda/agreements.toml") != dirty {
		t.Errorf("断ったのに、リポジトリが変わった")
	}
}

// RV-17・CMD-10
func Test取り消すと段1の合意の審査で止まりもう一度合意すると通る(t *testing.T) {
	t.Parallel()
	bin, r, s := setupAgree(t)
	for _, body := range []map[string]any{
		agreeClaim("task-id-unique", reviewClaimFP),
		agreeClaim("task-title-required", reviewTitleFP),
		agreeFile("e2e/add-task.txtar", reviewFileFP),
	} {
		if code, msg := s.post("/api/agree", body, nil); code != 200 {
			t.Fatalf("%v に合意できない（%d）: %s", body, code, msg)
		}
	}
	if code, out, errs := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main"); code != 0 {
		t.Fatalf("全部に合意しても審査が通らない（%d）\n%s%s", code, out, errs)
	}
	if code, msg := s.post("/api/revoke", revokeClaim("task-id-unique", reviewClaimFP), nil); code != 200 {
		t.Fatalf("取り消せない（%d）: %s", code, msg)
	}
	if !strings.Contains(r.git("log", "-1", "--format=%B"), reviewStory) {
		t.Errorf("取り消しのコミットのメッセージにストーリーの ID がない")
	}
	code, out, errs := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main")
	if code != 1 || !strings.Contains(out+errs, "task-id-unique") {
		t.Errorf("取り消した後、合意の審査が task-id-unique で止まらない（%d）\n%s%s", code, out, errs)
	}
	if !strings.Contains(out, "審査 story-origin（story）: 通った") {
		t.Errorf("取り消しのコミットで由来の審査が止まった\n%s%s", out, errs)
	}
	if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 200 {
		t.Fatalf("もう一度合意できない（%d）: %s", code, msg)
	}
	if code, out, errs := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main"); code != 0 {
		t.Errorf("もう一度合意しても審査が通らない（%d）\n%s%s", code, out, errs)
	}
}

// RV-06: キーを引用符で囲んだ flags・guesses（"flags"・'guesses'）も、合意でその行だけが消える
// @vanda:evidence agree-removes-flags
func Test引用符で囲んだキーのflagsとguessesの約束にも合意できその行だけが消える(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	text := readFileString(t, r.Root, ".vanda/promises.toml")
	flags, guesses := "flags = [\"ai-proposal\"]\n", "guesses = [\"タスクは消しても ID を使い回さない\"]\n"
	quoted := strings.Replace(strings.Replace(text, flags, "\"flags\" = [\"ai-proposal\"]\n", 1), guesses, "'guesses' = [\"タスクは消しても ID を使い回さない\"]\n", 1)
	if quoted == text || strings.Count(quoted, "\"flags\" = ") != 1 || strings.Count(quoted, "'guesses' = ") != 1 {
		t.Fatalf("約束リストのキーを引用符で囲めない:\n%s", quoted)
	}
	writeFile(t, r.Root, ".vanda/promises.toml", quoted)
	r.g.commitTracked("キーを引用符で囲む", reviewStory)

	if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}
	want := strings.Replace(strings.Replace(quoted, "\"flags\" = [\"ai-proposal\"]\n", "", 1), "'guesses' = [\"タスクは消しても ID を使い回さない\"]\n", "", 1)
	if now := readFileString(t, r.Root, ".vanda/promises.toml"); now != want {
		t.Errorf("約束リストが、引用符で囲んだ flags と guesses の行だけを消したものでない\n--- 今\n%s\n--- 思ったもの\n%s", now, want)
	}
	if files := committedFiles(r, "HEAD"); !reflect.DeepEqual(files, []string{".vanda/agreements.toml", ".vanda/promises.toml"}) {
		t.Errorf("コミットに入ったファイル: %v", files)
	}
}

// RV-06 のガードレール: 合意の記録を書くファイルが決まらない（設定の agreements のパターンのどれにも当たるファイルがなく、
// 最初のパターンにワイルドカードがある）ときは、ファイルを作らずに 409 で断る（docs/formats.md）
// @vanda:evidence agree-writes-and-commits
func Test合意の記録を書くファイルがパターンから決まらなければ合意を断りファイルを作らずコミットもしない(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	cfg := readFileString(t, r.Root, ".vanda/config.toml")
	first, rest, ok := strings.Cut(cfg, "\n")
	if !ok || !strings.HasPrefix(first, "version = ") {
		t.Fatalf("設定の 1 行目が version でない: %q", first)
	}
	writeFile(t, r.Root, ".vanda/config.toml", first+"\nagreements = [\".vanda/agreements/*.toml\"]\n"+rest)
	r.git("rm", "-q", ".vanda/agreements.toml")
	r.g.commitTracked("合意の記録をストーリーごとのファイルに分ける", reviewStory)
	head := r.git("rev-parse", "HEAD")
	promises := readFileString(t, r.Root, ".vanda/promises.toml")

	for _, body := range []map[string]any{
		agreeClaim("task-id-unique", reviewClaimFP),
		agreeFile("e2e/add-task.txtar", reviewFileFP),
	} {
		if code, msg := s.post("/api/agree", body, nil); code != 409 || msg == "" {
			t.Errorf("%v: %d（%q）を返した。409 で断り、理由を出す", body, code, msg)
		}
	}
	if r.git("rev-parse", "HEAD") != head {
		t.Errorf("断ったのに、コミットができた")
	}
	if readFileString(t, r.Root, ".vanda/promises.toml") != promises {
		t.Errorf("断ったのに、約束リストが変わった（フラグを外した）")
	}
	if st := r.git("status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Errorf("断ったのに、作業ツリーにファイルができたか変わった: %q", st)
	}
}

// RV-06 のガードレール: 合意でフラグを外すとき、外し方が決まっていない書き方（複数行の文字列の flags・guesses）で
// 書いた約束リストは書き換えず、合意を 409 で断る。約束リストと合意の記録を壊さないため（docs/formats.md）
// @vanda:evidence agree-removes-flags
func Testフラグを複数行の文字列で書いた約束には合意を断り約束リストと合意の記録を書き換えない(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, from, to string }{
		{"guessesが複数行の文字列", `guesses = ["タスクは消しても ID を使い回さない"]`, "guesses = [\"\"\"タスクは消しても\nID を使い回さない\"\"\"]"},
		{"flagsが複数行の文字列", `flags = ["ai-proposal"]`, `flags = ["""ai-proposal"""]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, r, s := setupAgree(t)
			text := readFileString(t, r.Root, ".vanda/promises.toml")
			if !strings.Contains(text, c.from) {
				t.Fatalf("約束リストに %q がない", c.from)
			}
			writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(text, c.from, c.to, 1))
			r.g.commitTracked("推測の書き方を変える", reviewStory)
			promises := readFileString(t, r.Root, ".vanda/promises.toml")
			agreements := readFileString(t, r.Root, ".vanda/agreements.toml")
			head := r.git("rev-parse", "HEAD")
			// flags・guesses は指紋に入らないので、見せていた指紋は前のまま
			if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 409 || msg == "" {
				t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
			}
			if r.git("rev-parse", "HEAD") != head || readFileString(t, r.Root, ".vanda/promises.toml") != promises || readFileString(t, r.Root, ".vanda/agreements.toml") != agreements {
				t.Errorf("断ったのに、約束リストか合意の記録が変わったか、コミットができた")
			}
		})
	}
}

type deletedReview struct {
	Entries []struct {
		Kind        string          `json:"kind"`
		Path        *string         `json:"path"`
		Change      string          `json:"change"`
		State       *string         `json:"state"`
		Fingerprint *string         `json:"fingerprint"`
		Before      json.RawMessage `json:"before"`
		After       json.RawMessage `json:"after"`
		Agree       *struct {
			Possible bool `json:"possible"`
		} `json:"agree"`
		Revoke *struct {
			Possible bool `json:"possible"`
		} `json:"revoke"`
	} `json:"entries"`
}

// RV-01・RV-06・RV-17: 消したもの
// @vanda:evidence review-deleted-shown
// @vanda:evidence revoke-returns-to-proposed
func Test消したファイルは一覧に削除として前の中身と提案中で出て消したことに合意すると合意済になり取り消すと提案中に戻る(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	if err := os.Remove(filepath.Join(r.Root, "db", "schema.sql")); err != nil {
		t.Fatal(err)
	}
	r.g.commitAll("スキーマの定義を消す", reviewStory)

	type seen struct {
		state, fingerprint            string
		before                        string
		afterNull                     bool
		agreePossible, revokePossible *bool
	}
	look := func() seen {
		t.Helper()
		var rv deletedReview
		if code, msg := s.get("/api/review?view=branch&base=main&story=%2324", &rv); code != 200 {
			t.Fatalf("一覧を読めない（%d）: %s", code, msg)
		}
		for _, e := range rv.Entries {
			if e.Kind != "file" || str(e.Path) != "db/schema.sql" {
				continue
			}
			if e.Change != "deleted" {
				t.Fatalf("消したファイルの change が %q", e.Change)
			}
			var before struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(e.Before, &before); err != nil {
				t.Fatalf("before を読めない: %v: %s", err, e.Before)
			}
			out := seen{state: str(e.State), fingerprint: str(e.Fingerprint), before: before.Text, afterNull: isNull(e.After)}
			if e.Agree != nil {
				p := e.Agree.Possible
				out.agreePossible = &p
			}
			if e.Revoke != nil {
				p := e.Revoke.Possible
				out.revokePossible = &p
			}
			return out
		}
		t.Fatalf("一覧に、消した db/schema.sql がない")
		return seen{}
	}

	got := look()
	if got.state != "proposed" || got.fingerprint != "" || got.before != reviewSchema || !got.afterNull {
		t.Errorf("消したことに合意する前: %+v（提案中、指紋なし、前の中身、後の中身なし のはず）", got)
	}
	if got.agreePossible == nil || !*got.agreePossible {
		t.Errorf("消したことに合意できない: %+v", got)
	}

	head := r.git("rev-parse", "HEAD")
	body := map[string]any{"kind": "file", "id": nil, "path": "db/schema.sql", "fingerprint": nil, "story": reviewStory, "deleted": true}
	var res agreeResponse
	if code, msg := s.post("/api/agree", body, &res); code != 200 {
		t.Fatalf("消したことに合意できない（%d）: %s", code, msg)
	}
	if !res.Agreement.Deleted || res.Agreement.Fingerprint != nil || res.Agreement.Key != "db/schema.sql" || str(res.Agreement.Path) != "db/schema.sql" {
		t.Errorf("返した合意の記録: %+v", res.Agreement)
	}
	if n := r.git("rev-list", "--count", head+"..HEAD"); n != "1" {
		t.Errorf("コミットが %s 個できた", n)
	}
	if subject := r.git("log", "-1", "--format=%s"); subject != "(chore) #24 で ファイル db/schema.sql を消したことに合意する" {
		t.Errorf("コミットのメッセージ: %q", subject)
	}
	recs := readAgreements(t, r.Root)
	rec := recs[len(recs)-1]
	if rec.Kind != "file" || rec.Key != "db/schema.sql" || !rec.Deleted || rec.Fingerprint != "" || rec.Path != "db/schema.sql" ||
		rec.Story != reviewStory || rec.User.Name != reviewUserName || rec.User.Email != reviewUserEmail {
		t.Errorf("書き足した合意の記録: %+v", rec)
	}
	got = look()
	if got.state != "agreed" {
		t.Errorf("消したことに合意した後の状態が %q", got.state)
	}
	if got.agreePossible == nil || *got.agreePossible {
		t.Errorf("このストーリーで消したことにもう合意したのに、また合意できる: %+v", got)
	}
	if got.revokePossible == nil || !*got.revokePossible {
		t.Errorf("今の人が今のブランチで書いた、消したことへの合意を取り消せない: %+v", got)
	}

	var rv revokeResponse
	if code, msg := s.post("/api/revoke", map[string]any{"kind": "file", "id": nil, "path": "db/schema.sql", "fingerprint": nil, "story": reviewStory, "base": "main", "deleted": true}, &rv); code != 200 {
		t.Fatalf("消したことへの合意を取り消せない（%d）: %s", code, msg)
	}
	if rv.StillAgreed || rv.Removed != 1 {
		t.Errorf("取り消しの答え: %+v", rv)
	}
	if subject := r.git("log", "-1", "--format=%s"); subject != "(chore) #24 で ファイル db/schema.sql を消したことへの合意を取り消す" {
		t.Errorf("コミットのメッセージ: %q", subject)
	}
	if got := look(); got.state != "proposed" {
		t.Errorf("取り消した後の状態が %q（提案中に戻るはず）", got.state)
	}
}

// RV-06: 今あるものを消したことには合意できない。約束にはなく、docs/formats.md の POST /api/agree の deleted に書いてある振る舞い
func Test今あるファイルを消したことへの合意は断る(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	head := r.git("rev-parse", "HEAD")
	body := map[string]any{"kind": "file", "id": nil, "path": "db/schema.sql", "fingerprint": nil, "story": reviewStory, "deleted": true}
	if code, msg := s.post("/api/agree", body, nil); code != 409 || msg == "" {
		t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
	}
	if r.git("rev-parse", "HEAD") != head {
		t.Errorf("断ったのにコミットができた")
	}
}

// ---- 合意と取り消しのコミットのメッセージ・合意の記録の書き方 ----

// 合意と取り消しの対象。用語と機能は main で #1 が合意済だが、#24 では合意していないので合意できる
var agreeKinds = []struct {
	label, kind, id, path, fp string
}{
	{"約束", "claim", "task-id-unique", "", reviewClaimFP},
	{"用語", "term", "task", "", sha256Hex([]byte(reviewItemJSON["term task"]))},
	{"機能", "feature", "add-task", "", sha256Hex([]byte(reviewItemJSON["feature add-task"]))},
	{"ファイル", "file", "", "e2e/add-task.txtar", reviewFileFP},
}

func agreeBody(kind, id, path, fp string) map[string]any {
	body := map[string]any{"kind": kind, "id": nil, "path": nil, "fingerprint": fp, "story": reviewStory}
	if kind == "file" {
		body["path"] = path
	} else {
		body["id"] = id
	}
	return body
}

// @vanda:evidence agree-commit-message
// @vanda:evidence revoke-commit-message
func Test合意と取り消しのコミットのsubjectは種類とIDか場所をストーリーのIDと合わせて書く(t *testing.T) {
	t.Parallel()
	for _, k := range agreeKinds {
		t.Run(k.label, func(t *testing.T) {
			t.Parallel()
			_, r, s := setupAgree(t)
			target := k.id
			if k.kind == "file" {
				target = k.path
			}
			if code, msg := s.post("/api/agree", agreeBody(k.kind, k.id, k.path, k.fp), nil); code != 200 {
				t.Fatalf("合意できない（%d）: %s", code, msg)
			}
			if got, want := r.git("log", "-1", "--format=%s"), "(chore) #24 で "+k.label+" "+target+" に合意する"; got != want {
				t.Errorf("合意のコミットの subject: %q（思ったもの %q）", got, want)
			}
			body := agreeBody(k.kind, k.id, k.path, k.fp)
			body["base"] = "main"
			if code, msg := s.post("/api/revoke", body, nil); code != 200 {
				t.Fatalf("取り消せない（%d）: %s", code, msg)
			}
			if got, want := r.git("log", "-1", "--format=%s"), "(chore) #24 で "+k.label+" "+target+" の合意を取り消す"; got != want {
				t.Errorf("取り消しのコミットの subject: %q（思ったもの %q）", got, want)
			}
		})
	}
}

// appendedText は、合意の前の中身 before に書き足された部分を返す。前の中身を変えていれば落とす
func appendedText(t *testing.T, before, after string) string {
	t.Helper()
	if !strings.HasPrefix(after, before) {
		t.Fatalf("合意の記録のファイルが、前の中身の終わりに書き足したものでない\n--- 前\n%s\n--- 後\n%s", before, after)
	}
	return after[len(before):]
}

// @vanda:evidence agree-record-form
func Test書き足す合意の記録はuserをインラインの表で書きagreed_atに手元の時差を付けた秒までの日時を書く(t *testing.T) {
	t.Parallel()
	for tz, offset := range map[string]string{"Asia/Tokyo": "+09:00", "Asia/Kolkata": "+05:30"} {
		t.Run(tz, func(t *testing.T) {
			t.Parallel()
			bin := buildVanda(t)
			version, err := runningVersion(bin)
			if err != nil {
				t.Fatal(err)
			}
			r := newReviewRepo(t, version)
			s := startServe(t, bin, r.Root, append(append([]string(nil), r.Env...), "TZ="+tz))
			before := readFileString(t, r.Root, ".vanda/agreements.toml")
			// 秒より細かい所を切り捨てて比べる
			start := time.Now().Truncate(time.Second)
			if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 200 {
				t.Fatalf("合意できない（%d）: %s", code, msg)
			}
			end := time.Now()
			added := appendedText(t, before, readFileString(t, r.Root, ".vanda/agreements.toml"))
			if !regexp.MustCompile(`(?m)^user = \{ ?name = "` + reviewUserName + `", email = "` + reviewUserEmail + `" ?\}$`).MatchString(added) {
				t.Errorf("user が user = { name = …, email = … } の 1 行で書かれていない\n%s", added)
			}
			if strings.Contains(added, "[agreement.user]") {
				t.Errorf("user を表の見出しで書いた\n%s", added)
			}
			m := regexp.MustCompile(`(?m)^agreed_at = (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})` + regexp.QuoteMeta(offset) + `$`).FindStringSubmatch(added)
			if m == nil {
				t.Fatalf("agreed_at が、秒までの日時に手元の時差 %s を付けた形でない\n%s", offset, added)
			}
			at, err := time.Parse(time.RFC3339, m[1]+offset)
			if err != nil {
				t.Fatal(err)
			}
			if at.Before(start) || at.After(end) {
				t.Errorf("agreed_at %s が、合意した時（%s〜%s）でない", at, start, end)
			}
		})
	}
}

// @vanda:evidence agree-record-user-from-git
func Test書き足す合意の記録のuserには合意した時のgitのuser_nameとuser_emailを書く(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// vanda serve を起こした後に変えた名前とメールを使う
	r.git("config", "user.name", "後から決めた人")
	r.git("config", "user.email", "later@example.com")
	var got agreeResponse
	if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), &got); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}
	recs := readAgreements(t, r.Root)
	if rec := recs[len(recs)-1]; rec.User.Name != "後から決めた人" || rec.User.Email != "later@example.com" {
		t.Errorf("書き足した合意の記録の user: %+v", rec.User)
	}
	if got.Agreement.User.Name != "後から決めた人" || got.Agreement.User.Email != "later@example.com" {
		t.Errorf("返した合意の記録の user: %+v", got.Agreement.User)
	}
	// もう一度変えれば、次の合意はその人になる
	r.git("config", "user.name", "さらに後の人")
	if code, msg := s.post("/api/agree", agreeFile("e2e/add-task.txtar", reviewFileFP), nil); code != 200 {
		t.Fatalf("合意できない（%d）: %s", code, msg)
	}
	recs = readAgreements(t, r.Root)
	if rec := recs[len(recs)-1]; rec.User.Name != "さらに後の人" || rec.User.Email != "later@example.com" {
		t.Errorf("2 度目の合意の記録の user: %+v", rec.User)
	}
}

// @vanda:evidence agree-record-file
func Test合意の記録はagreementsのパターンの順で最初に当たったファイルに書き足し当たらなければワイルドカードのない最初のパターンのファイルを作る(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		patterns string            // 設定の agreements。空なら書かない
		files    map[string]string // 置く合意の記録のファイル（.vanda/agreements.toml は消す）
		want     string            // 書き足すファイル。空なら 409 で断る
	}{
		{"2つ目のパターンだけに当たる", `[".vanda/agreements/*.toml", ".vanda/b.toml"]`, map[string]string{".vanda/b.toml": "# b\n"}, ".vanda/b.toml"},
		{"どちらにも当たれば前のパターンのもの", `[".vanda/b.toml", ".vanda/a.toml"]`, map[string]string{".vanda/a.toml": "# a\n", ".vanda/b.toml": "# b\n"}, ".vanda/b.toml"},
		{"ワイルドカードで当たったもの", `[".vanda/agreements/*.toml"]`, map[string]string{".vanda/agreements/x.toml": "# x\n"}, ".vanda/agreements/x.toml"},
		// 設定に書かない既定のパターン（.vanda/agreements.toml）は、ファイルがなくても誤りにならない
		{"当たらず最初のパターンにワイルドカードがない", "", map[string]string{}, ".vanda/agreements.toml"},
		// 設定にはっきり書いた、まだないファイルも作る（ないディレクトリの下でも）
		{"当たらず設定にはっきり書いた最初のパターンにワイルドカードがない", `[".vanda/agreements/po.toml", ".vanda/more/*.toml"]`, map[string]string{}, ".vanda/agreements/po.toml"},
		{"当たらず設定にはっきり書いた既定と同じ場所", `[".vanda/agreements.toml"]`, map[string]string{}, ".vanda/agreements.toml"},
		{"当たらず最初のパターンにワイルドカードがある", `[".vanda/agreements/*.toml", ".vanda/more/*.toml"]`, map[string]string{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, r, s := setupAgree(t)
			cfg := readFileString(t, r.Root, ".vanda/config.toml")
			first, rest, _ := strings.Cut(cfg, "\n")
			if c.patterns != "" {
				writeFile(t, r.Root, ".vanda/config.toml", first+"\nagreements = "+c.patterns+"\n"+rest)
			}
			r.git("rm", "-q", ".vanda/agreements.toml")
			for p, b := range c.files {
				writeFile(t, r.Root, p, b)
			}
			r.g.commitAll("合意の記録のファイルを変える", reviewStory)
			head := r.git("rev-parse", "HEAD")
			code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil)
			if c.want == "" {
				if code != 409 || msg == "" {
					t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
				}
				if r.git("rev-parse", "HEAD") != head || r.git("status", "--porcelain", "--untracked-files=all") != "" {
					t.Errorf("断ったのに、コミットができたかファイルが変わった")
				}
				return
			}
			if code != 200 {
				t.Fatalf("合意できない（%d）: %s", code, msg)
			}
			if files := committedFiles(r, "HEAD"); !reflect.DeepEqual(files, []string{".vanda/promises.toml", c.want}) && !reflect.DeepEqual(files, []string{c.want, ".vanda/promises.toml"}) {
				t.Errorf("コミットに入ったファイル: %v（%s と約束リストのはず）", files, c.want)
			}
			now := readFileString(t, r.Root, c.want)
			added := appendedText(t, c.files[c.want], now)
			if !strings.Contains(added, `key = "task-id-unique"`) || !strings.Contains(added, reviewClaimFP) {
				t.Errorf("%s に、合意の記録が書き足されていない\n%s", c.want, now)
			}
			for p, b := range c.files {
				if p != c.want && readFileString(t, r.Root, p) != b {
					t.Errorf("書き足すファイルでない %s が変わった", p)
				}
			}
		})
	}
}

// @vanda:evidence agreements-explicit-file-may-be-absent
func Test設定のagreementsにはっきり書いたファイルがなければ一覧と約束の詳細は誤りにせず合意の記録を空とみなす(t *testing.T) {
	t.Parallel()
	hasChanged := func(e *reviewEntry) bool {
		return e != nil && slices.Contains(e.Flags, "changed-since-agreement")
	}
	cases := []struct{ name, patterns string }{
		{"既定と同じ場所", `[".vanda/agreements.toml"]`},
		{"既定と違う場所", `[".vanda/agreements/po.toml"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, r, s := setupAgree(t)
			// 合意の記録を読めていれば、#1 で合意した前の中身から変わった task-id-unique に「差分あり」が付く
			var before reviewResponse
			if code, msg := s.get("/api/review?view=branch&base=main", &before); code != 200 || !hasChanged(findEntry(before.Entries, "task-id-unique")) {
				t.Fatalf("用意した合意の記録が読めていない（%d）: %s\n%+v", code, msg, before.Entries)
			}
			cfg := readFileString(t, r.Root, ".vanda/config.toml")
			first, rest, _ := strings.Cut(cfg, "\n")
			writeFile(t, r.Root, ".vanda/config.toml", first+"\nagreements = "+c.patterns+"\n"+rest)
			r.git("rm", "-q", ".vanda/agreements.toml")
			r.g.commitAll("合意の記録のファイルを設定に書き、消す", reviewStory)

			var got reviewResponse
			if code, msg := s.get("/api/review?view=branch&base=main", &got); code != 200 {
				t.Fatalf("一覧が %d: %s", code, msg)
			}
			if len(got.Entries) == 0 {
				t.Fatal("一覧が空")
			}
			for _, e := range got.Entries {
				if str(e.State) != "proposed" || hasChanged(&e) {
					t.Errorf("%s%s: state %q・flags %v（合意の記録が空なので、提案中で差分ありも付かないはず）", str(e.ID), str(e.Path), str(e.State), e.Flags)
				}
			}
			var claim claimDetail
			if code, msg := s.get("/api/claims/task-id-unique", &claim); code != 200 || claim.State != "proposed" {
				t.Errorf("約束の詳細が %d・state %q: %s", code, claim.State, msg)
			}
		})
	}
}

// @vanda:evidence agree-inline-flags-refused
func Testフラグか推測をインラインの表の中か複数行の文字列で書いた約束には合意を409で断り何も変えない(t *testing.T) {
	t.Parallel()
	// task-id-unique を、インラインの表の配列（1 行の claim = [{ … }]）で書き直したもの。task-title-required も同じ配列に入れる
	// TOML では、表の見出しより後のキーはその表のものになるので、claim = [...] を先頭に置く
	inline := `claim = [{ id = "task-id-unique", subjects = ["task-id"], statement = "タスクの ID は、すべてのタスクを通して重ならない", layers = ["model", "db"], flags = ["ai-proposal"], guesses = ["タスクは消しても ID を使い回さない"], ` +
		`evidence = [{ stage = "implementation", type = "scenario-test", layers = ["model"] }, { stage = "plan", type = "model-check", layers = ["model"] }, { stage = "implementation", type = "guardrail", layers = ["db"] }, { stage = "implementation", type = "unit-test", layers = ["model"] }] }, ` +
		`{ id = "task-title-required", subjects = ["add-task"], statement = "タスクの題は空にできない", layers = ["model"], flags = ["ai-proposal"], guesses = ["題の長さの上限はまだ決めていない"], evidence = [{ stage = "implementation", type = "scenario-test", layers = ["model"] }] }]` + "\n\n" +
		strings.SplitN(reviewPromisesStory, "# ID の重なり\n", 2)[0]
	cases := []struct{ name, promises string }{
		{"インラインの表の中", inline},
		{"guessesが複数行のリテラル文字列", strings.Replace(reviewPromisesStory, `guesses = ["タスクは消しても ID を使い回さない"]`, "guesses = ['''\nタスクは消しても ID を使い回さない''']", 1)},
		{"flagsが複数行の基本文字列", strings.Replace(reviewPromisesStory, "flags = [\"ai-proposal\"]\nguesses = [\"タスクは消しても", "flags = [\"\"\"\nai-proposal\"\"\"]\nguesses = [\"タスクは消しても", 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, r, s := setupAgree(t)
			if c.promises == reviewPromisesStory {
				t.Fatal("約束リストを書き換えられない")
			}
			writeFile(t, r.Root, ".vanda/promises.toml", c.promises)
			r.g.commitTracked("フラグの書き方を変える", reviewStory)
			// 書き方を変えても指紋は変わらない。今の中身の指紋で合意しようとする
			var claim claimDetail
			if code, msg := s.get("/api/claims/task-id-unique", &claim); code != 200 || claim.Fingerprint != reviewClaimFP {
				t.Fatalf("書き方を変えた約束を読めないか、指紋が変わった（%d、%s）: %s", code, claim.Fingerprint, msg)
			}
			promises := readFileString(t, r.Root, ".vanda/promises.toml")
			agreements := readFileString(t, r.Root, ".vanda/agreements.toml")
			head := r.git("rev-parse", "HEAD")
			if code, msg := s.post("/api/agree", agreeClaim("task-id-unique", reviewClaimFP), nil); code != 409 || msg == "" {
				t.Errorf("%d（%q）を返した。409 で断り、理由を出す", code, msg)
			}
			if r.git("rev-parse", "HEAD") != head || readFileString(t, r.Root, ".vanda/promises.toml") != promises || readFileString(t, r.Root, ".vanda/agreements.toml") != agreements {
				t.Errorf("断ったのに、約束リストか合意の記録が変わったか、コミットができた")
			}
		})
	}
}

// ---- 合意と取り消しを 1 件ずつ行う ----

type rawResult struct {
	what string
	code int
	body []byte
	err  error
}

// rawCall は、テストのゴルーチンの外からも呼べるよう、t を使わずに API を呼ぶ（vandaServer.do は t.Fatalf を呼ぶため）。
func rawCall(base, what, method, path string, body any) rawResult {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return rawResult{what: what, err: err}
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		return rawResult{what: what, err: err}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rawResult{what: what, err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return rawResult{what: what, code: resp.StatusCode, body: data, err: err}
}

// blockingHook は、合意・取り消しのコミットの途中（pre-commit のフック）で止まるフックを置く。
// フックは、動いた回数を started に 1 行ずつ書き、n 回目は release-<n> のファイルができるまで待つ。
type blockingHook struct {
	t   *testing.T
	dir string
}

func newBlockingHook(t *testing.T, r *reviewRepo) *blockingHook {
	t.Helper()
	h := &blockingHook{t: t, dir: t.TempDir()}
	script := fmt.Sprintf("#!/bin/sh\necho x >> %q\nn=$(wc -l < %q | tr -d ' ')\nwhile [ ! -f %q-$n ]; do sleep 0.02; done\n",
		filepath.Join(h.dir, "started"), filepath.Join(h.dir, "started"), filepath.Join(h.dir, "release"))
	if err := os.WriteFile(filepath.Join(r.Root, ".git", "hooks", "pre-commit"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// テストが途中で落ちても、止まったフックを残さない
	t.Cleanup(func() {
		for i := 1; i <= 10; i++ {
			h.release(i)
		}
	})
	return h
}

func (h *blockingHook) started() int {
	data, err := os.ReadFile(filepath.Join(h.dir, "started"))
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "\n")
}

func (h *blockingHook) waitStarted(n int) {
	h.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for h.started() < n {
		if time.Now().After(deadline) {
			h.t.Fatalf("15 秒たっても、合意か取り消しのコミットが %d 回目に入らない（%d 回）", n, h.started())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *blockingHook) release(n int) {
	if err := os.WriteFile(filepath.Join(h.dir, fmt.Sprintf("release-%d", n)), nil, 0o644); err != nil {
		h.t.Errorf("%v", err)
	}
}

// claimState は、一覧の中身から、約束 id の state を読む。
func claimState(body []byte, id string) string {
	var rv reviewResponse
	if err := json.Unmarshal(body, &rv); err != nil {
		return "（一覧を読めない: " + err.Error() + "）"
	}
	for _, e := range rv.Entries {
		if e.Kind == "claim" && str(e.ID) == id {
			return str(e.State)
		}
	}
	return "（一覧にない）"
}

// 合意と取り消しのコミットの途中で止めたあいだに、ほかの要求を出し、どれも答えないことを確かめる。
// 答えないことは、決まった時間（waitWindow）のあいだ答えが来ないことでしか確かめられない
// @vanda:evidence serve-agree-one-at-a-time
func Test合意と取り消しのあいだはほかの合意と一覧と約束の詳細とファイルの読み込みを待たせ終わった後に答える(t *testing.T) {
	t.Parallel()
	const waitWindow = 500 * time.Millisecond
	_, r, s := setupAgree(t)
	h := newBlockingHook(t, r)
	results := make(chan rawResult, 10)
	send := func(what, method, path string, body any) {
		go func() { results <- rawCall(s.URL, what, method, path, body) }()
	}
	// noAnswer は、waitWindow のあいだ、どの要求も答えず、合意か取り消しのコミットが started 件より増えないことを確かめる
	noAnswer := func(phase string, started int) {
		t.Helper()
		select {
		case res := <-results:
			t.Fatalf("%s: %s が、合意か取り消しのコミットを待たずに答えた（%d）: %s", phase, res.what, res.code, res.body)
		case <-time.After(waitWindow):
		}
		if n := h.started(); n != started {
			t.Fatalf("%s: 合意か取り消しのコミットが同時に %d 件動いた", phase, n)
		}
	}
	// collect は、n 件の答えを集める。合意か取り消しのコミットが次の回に入ったら、そのつど通す
	collect := func(phase string, n, next int) map[string]rawResult {
		t.Helper()
		got := map[string]rawResult{}
		deadline := time.After(30 * time.Second)
		for len(got) < n {
			select {
			case res := <-results:
				got[res.what] = res
			case <-time.After(10 * time.Millisecond):
				if h.started() >= next {
					h.release(next)
					next++
				}
			case <-deadline:
				t.Fatalf("%s: 30 秒たっても答えが %d 件しか来ない: %v", phase, len(got), got)
			}
		}
		for what, res := range got {
			if res.err != nil || res.code != 200 {
				t.Errorf("%s: %s が失敗した（%d、%v）: %s", phase, what, res.code, res.err, res.body)
			}
		}
		return got
	}

	// 合意のあいだ
	send("1 件目の合意", http.MethodPost, "/api/agree", agreeClaim("task-id-unique", reviewClaimFP))
	h.waitStarted(1)
	send("2 件目の合意", http.MethodPost, "/api/agree", agreeClaim("task-title-required", reviewTitleFP))
	send("一覧", http.MethodGet, "/api/review?view=branch&base=main", nil)
	send("約束の詳細", http.MethodGet, "/api/claims/task-id-unique", nil)
	send("ファイル", http.MethodGet, "/api/file?path="+q("e2e/add-task.txtar"), nil)
	noAnswer("合意のあいだ", 1)
	h.release(1)
	got := collect("合意の後", 5, 2)
	if st := claimState(got["一覧"].body, "task-id-unique"); st != "agreed" {
		t.Errorf("合意のあいだに求めた一覧の task-id-unique が %s（合意を待ってから読むので agreed のはず）", st)
	}
	var detail claimDetail
	if err := json.Unmarshal(got["約束の詳細"].body, &detail); err != nil || detail.State != "agreed" {
		t.Errorf("合意のあいだに求めた約束の詳細の state が %q（合意を待ってから読むので agreed のはず）: %v", detail.State, err)
	}

	// 取り消しのあいだ
	send("取り消し", http.MethodPost, "/api/revoke", revokeClaim("task-id-unique", reviewClaimFP))
	h.waitStarted(3)
	send("取り消しのあいだの一覧", http.MethodGet, "/api/review?view=branch&base=main", nil)
	send("取り消しのあいだの約束の詳細", http.MethodGet, "/api/claims/task-id-unique", nil)
	send("取り消しのあいだのファイル", http.MethodGet, "/api/file?path="+q("e2e/add-task.txtar"), nil)
	noAnswer("取り消しのあいだ", 3)
	h.release(3)
	got = collect("取り消しの後", 4, 4)
	if st := claimState(got["取り消しのあいだの一覧"].body, "task-id-unique"); st != "proposed" {
		t.Errorf("取り消しのあいだに求めた一覧の task-id-unique が %s（取り消しを待ってから読むので proposed のはず）", st)
	}
	if n := len(readAgreements(t, r.Root)); n != 8 {
		t.Errorf("合意の記録が %d 件（はじめの 7 件に、2 件足して 1 件消したので 8 件のはず）", n)
	}
}

// buildRaceVanda は、データの競合を見つける仕組み（-race）を入れて vanda をビルドする。vanda サーバーは別のプロセスで動くので、
// テストを go test -race で動かしても、vanda サーバーの中の競合は見つからないため
func buildRaceVanda(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vanda")
	if out, err := exec.Command("go", "build", "-race", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("vanda を -race でビルドできない: %v\n%s", err, out)
	}
	return bin
}

// @vanda:evidence serve-agree-one-at-a-time
func Test同時に出した合意と取り消しと読み込みはどれも失敗せず合意と取り消しが1件ずつ1つのコミットになり競合がない(t *testing.T) {
	t.Parallel()
	bin := buildRaceVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	s := startServe(t, bin, r.Root, r.Env)
	before := r.git("rev-parse", "HEAD")

	reads := []struct{ what, path string }{
		{"一覧", "/api/review?view=branch&base=main"},
		{"約束の詳細", "/api/claims/task-id-unique"},
		{"ファイル", "/api/file?path=" + q("e2e/add-task.txtar")},
		{"画面で選べるもの", "/api/context"},
	}
	// reading は、writes が終わるまで、読み込みを並べて出し続ける
	reading := func(stop <-chan struct{}, out chan<- rawResult) {
		var wg sync.WaitGroup
		for _, rd := range reads {
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						out <- rawCall(s.URL, rd.what, http.MethodGet, rd.path, nil)
					}
				}()
			}
		}
		wg.Wait()
		close(out)
	}
	// run は、書き込みの要求を一度に出し、そのあいだ読み込みを出し続け、どれも 200 で答えたかを確かめる
	run := func(phase string, writes map[string]map[string]any, path string) {
		t.Helper()
		stop := make(chan struct{})
		readResults := make(chan rawResult, 64)
		go reading(stop, readResults)
		var wg sync.WaitGroup
		writeResults := make(chan rawResult, len(writes))
		for what, body := range writes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				writeResults <- rawCall(s.URL, what, http.MethodPost, path, body)
			}()
		}
		wg.Wait()
		close(stop)
		close(writeResults)
		for res := range writeResults {
			if res.err != nil || res.code != 200 {
				t.Errorf("%s: %s が失敗した（%d、%v）: %s", phase, res.what, res.code, res.err, res.body)
			}
		}
		n := 0
		for res := range readResults {
			n++
			if res.err != nil || res.code != 200 {
				t.Errorf("%s: 書き込みと同時に出した %s が失敗した（%d、%v）: %s", phase, res.what, res.code, res.err, res.body)
			}
		}
		if n == 0 {
			t.Errorf("%s: 書き込みと同時に読み込みを出せていない", phase)
		}
	}

	run("合意", map[string]map[string]any{
		"task-id-unique への合意":      agreeClaim("task-id-unique", reviewClaimFP),
		"task-title-required への合意": agreeClaim("task-title-required", reviewTitleFP),
		"e2e/add-task.txtar への合意":  agreeFile("e2e/add-task.txtar", reviewFileFP),
	}, "/api/agree")
	if n := r.git("rev-list", "--count", before+"..HEAD"); n != "3" {
		t.Errorf("同時に出した 3 件の合意のコミットが %s 個（1 件ずつ 3 個のはず）", n)
	}
	run("取り消し", map[string]map[string]any{
		"task-id-unique の取り消し":      revokeClaim("task-id-unique", reviewClaimFP),
		"task-title-required の取り消し": revokeClaim("task-title-required", reviewTitleFP),
	}, "/api/revoke")
	if n := r.git("rev-list", "--count", before+"..HEAD"); n != "5" {
		t.Errorf("合意と取り消しのコミットが %s 個（合意 3 個と取り消し 2 個の 5 個のはず）", n)
	}
	var keys []string
	for _, a := range readAgreements(t, r.Root) {
		if a.Story == reviewStory {
			keys = append(keys, a.Kind+" "+a.Key)
		}
	}
	if want := []string{"file " + reviewFileFP}; !reflect.DeepEqual(keys, want) {
		t.Errorf("#24 の合意の記録: %v（%v のはず）", keys, want)
	}
	s.stop()
	if strings.Contains(s.Output(), "DATA RACE") {
		t.Errorf("vanda サーバーの中で、データの競合が見つかった:\n%s", s.Output())
	}
}

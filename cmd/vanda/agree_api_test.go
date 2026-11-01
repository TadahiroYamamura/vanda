package main_test

import (
	"fmt"
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
// @vanda:evidence agree-writes-and-commits
// @vanda:evidence agree-removes-flags
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
	r.git("add", "-A")
	r.git("commit", "-q", "-m", reviewStory+" 合意の記録を書き直す")
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
	if state, _, _ := revokeEntry(t, s); state == "agreed" {
		t.Errorf("取り消した後も合意済のまま")
	}
}

// RV-17。人の違う記録は残り、同じ人の同じ記録が 2 つあれば両方消える。ほかのストーリーの記録も残る
// @vanda:evidence revoke-own-branch-only
func Test取り消すと今の人の記録だけが消えほかの人とほかのストーリーの記録は残り合意済のままになる(t *testing.T) {
	t.Parallel()
	other := inlineRecord(reviewStory, "ほかの人", "other@example.com")
	story30 := inlineRecord("#30", reviewUserName, reviewUserEmail)
	self := inlineRecord(reviewStory, reviewUserName, reviewUserEmail)
	_, r, s, base := setupRevoke(t, func(base string) string {
		return base + "\n# #24 の合意\n" + self + other + story30 + self
	})
	want := base + "\n# #24 の合意\n" + other + story30

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
				r.git("add", "-A")
				r.git("commit", "-q", "-m", reviewStory+" フラグを外す")
				rec := inlineRecord(reviewStory, w.user, w.email)
				if onBase {
					r.git("switch", "-q", "main")
					writeFile(t, r.Root, ".vanda/agreements.toml", readFileString(t, r.Root, ".vanda/agreements.toml")+rec)
					r.git("add", "-A")
					r.git("commit", "-q", "-m", reviewStory+" 合意を書く")
					r.git("switch", "-q", reviewBranch)
					r.git("merge", "-q", "--no-edit", "main")
				} else {
					writeFile(t, r.Root, ".vanda/agreements.toml", readFileString(t, r.Root, ".vanda/agreements.toml")+rec)
					r.git("add", "-A")
					r.git("commit", "-q", "-m", reviewStory+" 合意を書く")
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

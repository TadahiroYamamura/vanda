package review

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/config"
)

// センサー（agree_sensor.go・review_sensor.go の事後条件の確かめ）のテスト。
// 事後条件は、合意・取り消しが正しく動く限り外からは破れないので、Hooks の口から破る状態を作り、センサーが 500 で知らせることを見る。
// 口を nil にしたとき（破らないとき）に通ることも、同じリポジトリの作り方で確かめる。
// Hooks はパッケージの変数なので、このファイルのテストは並べて動かさない。

const sensorConfig = `version = "0.1.0"

[[layer]]
id = "model"
name = "モデル"
description = "判定"

[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"
types = ["scenario-test"]

[story]
id_pattern = "#[0-9]+"

[gui]
base = "main"
`

const sensorPromisesBase = `[[term]]
id = "t"
name = "用語"
definition = "定義"

[[claim]]
id = "c"
subjects = ["t"]
statement = "前の文"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "scenario-test"
layers = ["model"]
`

// 変えた後の約束には flags・guesses を付け、合意でそれが消えることを事後条件にする
var sensorPromisesStory = strings.Replace(sensorPromisesBase, `statement = "前の文"`, "statement = \"後の文\"\nflags = [\"ai-proposal\"]\nguesses = [\"推測\"]", 1)

const (
	sensorScenarioBase  = "# @vanda:evidence c\nexec true\n"
	sensorScenarioStory = "# @vanda:evidence c\nexec true\nexec false\n"
	sensorStory         = "#2"
)

type sensorRepo struct {
	t    *testing.T
	root string
	o    Options
}

func (r *sensorRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit は、作業ツリーのすべてを、ストーリー story のコミット（末尾に Story: のトレーラー）にする。同じ大きさの書き換えも
// コミットに入るよう、索引から一度外して読み直させる（git add -A は stat の一致から変わっていないとみなすことがある）
func (r *sensorRepo) commit(subject, story string) {
	r.t.Helper()
	r.git("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", subject+"\n\nStory: "+story)
}

func (r *sensorRepo) write(path, body string) {
	r.t.Helper()
	p := filepath.Join(r.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *sensorRepo) read(path string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(path)))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

// newSensorRepo は、main に前の中身、story-2 に後の中身（約束の文とシナリオを変え、約束に flags・guesses を付けた）をコミットしたリポジトリを作る。
func newSensorRepo(t *testing.T) *sensorRepo {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	r := &sensorRepo{t: t, root: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "テスト担当")
	r.git("config", "user.email", "tester@example.com")
	r.write(".vanda/config.toml", sensorConfig)
	r.write(".vanda/promises.toml", sensorPromisesBase)
	r.write(".vanda/agreements.toml", "")
	r.write("e2e/s.txtar", sensorScenarioBase)
	r.commit("はじめ", "#1")
	r.git("switch", "-q", "-c", "story-2")
	r.write(".vanda/promises.toml", sensorPromisesStory)
	r.write("e2e/s.txtar", sensorScenarioStory)
	r.commit("変える", sensorStory)
	cfg, errs := config.Load(r.root)
	if errs != nil {
		t.Fatal(errs)
	}
	r.o = Options{Root: r.root, Cfg: cfg}
	return r
}

func (r *sensorRepo) claimReq() AgreeRequest {
	r.t.Helper()
	d, err := Claim(r.o, "c")
	if err != nil || d.Fingerprint == nil {
		r.t.Fatalf("約束の詳細を読めない: %v", err)
	}
	id := "c"
	return AgreeRequest{Kind: "claim", ID: &id, Fingerprint: *d.Fingerprint, Story: sensorStory}
}

func (r *sensorRepo) fileReq() AgreeRequest {
	path := "e2e/s.txtar"
	return AgreeRequest{Kind: "file", Path: &path, Fingerprint: agreements.FileFingerprint([]byte(sensorScenarioStory)), Story: sensorStory}
}

// state は、HEAD と、合意で書くファイルの中身。センサーが書いたファイルを元に戻したかを見るのに使う。
func (r *sensorRepo) state() [3]string {
	return [3]string{r.git("rev-parse", "HEAD"), r.read(".vanda/promises.toml"), r.read(".vanda/agreements.toml")}
}

// setHook は口に f を付け、呼ばれたかを返す。呼ばれずに 500 になったもの（センサーより前の誤り）を、センサーが知らせたとみなさないため
func setHook(t *testing.T, p *func(string), f func(string)) *bool {
	called := new(bool)
	*p = func(root string) {
		*called = true
		f(root)
	}
	t.Cleanup(func() { *p = nil })
	return called
}

func wantSensor500(t *testing.T, what string, called *bool, err *Error) {
	t.Helper()
	if !*called {
		t.Fatalf("%s: 口が呼ばれる前に終わった: %+v", what, err)
	}
	if err == nil || err.Status != http.StatusInternalServerError || err.Message == "" {
		t.Fatalf("%s: 事後条件を破ったのに 500 で知らせない: %+v", what, err)
	}
}

// @vanda:evidence agree-writes-and-commits
// @vanda:evidence agree-removes-flags
// @vanda:evidence agree-rejects-uncommitted
// @vanda:evidence revoke-commits-and-keeps-flags
func Test口がnilで事後条件を破らなければ合意も取り消しもセンサーに止められない(t *testing.T) {
	r := newSensorRepo(t)
	for _, req := range []AgreeRequest{r.claimReq(), r.fileReq()} {
		if _, err := Agree(r.o, req, time.Now()); err != nil {
			t.Fatalf("%s に合意できない: %+v", req.Kind, err)
		}
	}
	if strings.Contains(r.read(".vanda/promises.toml"), "flags") {
		t.Errorf("合意したのに flags が残っている")
	}
	req := r.claimReq()
	if _, err := Revoke(r.o, RevokeRequest{Kind: req.Kind, ID: req.ID, Fingerprint: req.Fingerprint, Story: sensorStory}); err != nil {
		t.Fatalf("取り消せない: %+v", err)
	}
}

// @vanda:evidence agree-removes-flags
func Test合意の記録を書いた後に約束リストへflagsを書き戻すと合意を500にしてファイルを元に戻す(t *testing.T) {
	r := newSensorRepo(t)
	before := r.state()
	called := setHook(t, &Hooks.AfterAgreeWrite, func(root string) {
		if err := os.WriteFile(filepath.Join(root, ".vanda/promises.toml"), []byte(sensorPromisesStory), 0o644); err != nil {
			t.Error(err)
		}
	})
	_, err := Agree(r.o, r.claimReq(), time.Now())
	wantSensor500(t, "flags を書き戻す", called, err)
	if r.state() != before {
		t.Errorf("センサーが止めたのに、ファイルが元に戻っていないかコミットができた")
	}
}

// @vanda:evidence agree-writes-and-commits
func Test合意の記録を書いた後に記録を書き換えると合意を500にしてファイルを元に戻す(t *testing.T) {
	cases := []struct {
		name string
		edit func(string) string
	}{
		{"書き足した記録を消す", func(string) string { return "" }},
		{"書き足した記録のストーリーを変える", func(s string) string { return strings.Replace(s, `"`+sensorStory+`"`, `"#9"`, 1) }},
		{"書き足した記録の指紋を変える", func(s string) string {
			return strings.Replace(s, "sha256:", "sha256:0", 1)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newSensorRepo(t)
			for _, req := range []AgreeRequest{r.claimReq(), r.fileReq()} {
				before := r.state()
				called := setHook(t, &Hooks.AfterAgreeWrite, func(root string) {
					p := filepath.Join(root, ".vanda/agreements.toml")
					b, err := os.ReadFile(p)
					if err != nil {
						t.Error(err)
						return
					}
					after := c.edit(string(b))
					if after == string(b) {
						t.Errorf("書き換えられなかった:\n%s", b)
					}
					if err := os.WriteFile(p, []byte(after), 0o644); err != nil {
						t.Error(err)
					}
				})
				_, err := Agree(r.o, req, time.Now())
				wantSensor500(t, req.Kind, called, err)
				if r.state() != before {
					t.Errorf("%s: センサーが止めたのに、ファイルが元に戻っていないかコミットができた", req.Kind)
				}
			}
		})
	}
}

// @vanda:evidence agree-writes-and-commits
func Test合意のコミットにほかのファイルを入れると合意を500にしてそのコミットを伝える(t *testing.T) {
	r := newSensorRepo(t)
	called := setHook(t, &Hooks.AfterAgreeCommit, func(root string) {
		r.write("notes.md", "メモ\n")
		r.git("add", "notes.md")
		r.git("commit", "-q", "--amend", "--no-edit")
	})
	_, err := Agree(r.o, r.claimReq(), time.Now())
	wantSensor500(t, "ほかのファイルを入れる", called, err)
	// センサーが見つけたのは、合意で作ったコミット（amend する前の HEAD）
	made := r.git("rev-parse", "HEAD@{1}")
	if !strings.Contains(err.Message, made) {
		t.Errorf("合意で作ったコミット %s を伝えていない: %s", made, err.Message)
	}
}

// @vanda:evidence agree-rejects-uncommitted
func Test合意のコミットの後に合意の対象のファイルが書き換わっていると合意を500にする(t *testing.T) {
	r := newSensorRepo(t)
	cases := []struct {
		req  AgreeRequest
		path string
	}{
		{r.claimReq(), ".vanda/promises.toml"},
		{r.fileReq(), "e2e/s.txtar"},
	}
	for _, c := range cases {
		called := setHook(t, &Hooks.AfterAgreeCommit, func(root string) {
			if err := os.WriteFile(filepath.Join(root, c.path), []byte(r.read(c.path)+"# 書き換え\n"), 0o644); err != nil {
				t.Error(err)
			}
		})
		_, err := Agree(r.o, c.req, time.Now())
		wantSensor500(t, c.path+" を書き換える", called, err)
		r.git("checkout", "-q", "--", c.path)
	}
}

// @vanda:evidence revoke-commits-and-keeps-flags
func Test取り消しのコミットに約束リストの変更を入れると取り消しを500にする(t *testing.T) {
	r := newSensorRepo(t)
	req := r.claimReq()
	if _, err := Agree(r.o, req, time.Now()); err != nil {
		t.Fatalf("合意できない: %+v", err)
	}
	called := setHook(t, &Hooks.AfterRevokeCommit, func(root string) {
		// フラグを戻した約束リストを、取り消しのコミットに入れる
		r.write(".vanda/promises.toml", sensorPromisesStory)
		r.git("add", ".vanda/promises.toml")
		r.git("commit", "-q", "--amend", "--no-edit")
	})
	_, err := Revoke(r.o, RevokeRequest{Kind: req.Kind, ID: req.ID, Fingerprint: req.Fingerprint, Story: sensorStory})
	wantSensor500(t, "約束リストの変更を入れる", called, err)
}

// 一覧の作り方が正しければ外からは破れないので、食い違う changes と entries を直に渡す
// @vanda:evidence review-count-verified
func Test_checkCountは一覧の数が差分の数から名前の変更を引いたものに合わないか同じものが2度出れば500にする(t *testing.T) {
	ch := func(keys ...string) []rawChange {
		var out []rawChange
		for _, k := range keys {
			out = append(out, rawChange{key: k})
		}
		return out
	}
	en := func(key, change string) Entry { return Entry{key: key, Change: change} }
	cases := []struct {
		name    string
		changes []rawChange
		es      []Entry
		ok      bool
	}{
		{"数が合う", ch("claim:a", "path:x"), []Entry{en("claim:a", "modified"), en("path:x", "added")}, true},
		{"名前の変更は2つの差分で1件", ch("path:x", "path:y"), []Entry{en("path:y", "renamed")}, true},
		{"空", nil, nil, true},
		{"一覧が足りない", ch("claim:a", "path:x"), []Entry{en("claim:a", "modified")}, false},
		{"一覧が多い", ch("claim:a"), []Entry{en("claim:a", "modified"), en("path:x", "added")}, false},
		{"同じものが2度", ch("claim:a", "path:x"), []Entry{en("claim:a", "modified"), en("claim:a", "modified")}, false},
		{"名前の変更でないのに差分2つで1件", ch("path:x", "path:y"), []Entry{en("path:y", "modified")}, false},
	}
	for _, c := range cases {
		err := checkCount(c.changes, c.es)
		if c.ok && err != nil {
			t.Errorf("%s: 食い違っていないのに止めた: %+v", c.name, err)
		}
		if !c.ok && (err == nil || err.Status != http.StatusInternalServerError || err.Message == "") {
			t.Errorf("%s: 食い違っているのに 500 で知らせない: %+v", c.name, err)
		}
	}
}

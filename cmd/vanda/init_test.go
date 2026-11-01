package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/creack/pty"
)

// CMD-11 vanda init。テストは並べて動かす（問いごとに画面が落ち着くのを待つので、並べないと 1 つのパッケージの
// テストが go test の既定の 10 分を超える）。どれもテストごとの一時ディレクトリで動くので、互いに関わらない。
// 問いかけは擬似端末（pty）で vanda を動かし、問いを読んで答えを書く（docs/plans/init-test.md）。
// vanda init は、必ずテストごとの一時ディレクトリの git のリポジトリで動かす。
//
// 問いの文言は決まっていないので、問いは計画の順（ベースブランチ、ストーリーの置き場所、リポジトリかコマンド、
// ID の書き方、言語ごとのアノテーション、作るか）にたどり、それぞれの問いに出るはずの言葉で確かめる。
// 既定の答えは、計画のとおり、問いの中の [ ] の中から読む。

// ---- 一時的なリポジトリ ----

type initRepo struct {
	t    *testing.T
	root string
	g    *rvGit
}

// newInitRepo は、files を git に追わせたリポジトリを作る。remote が空でなければ origin に足し、originHEAD なら
// origin/HEAD が origin/main を指すようにする。
func newInitRepo(t *testing.T, remote string, originHEAD bool, files ...string) *initRepo {
	t.Helper()
	root := t.TempDir()
	r := &initRepo{t: t, root: root, g: newRVGit(t, root)}
	r.g.run("init", "-q", "-b", "main")
	writeFile(t, root, "README.md", "# 例\n")
	for _, f := range files {
		// Go のファイルは、アノテーションを読むときに Go として読めるものにする
		content := "\n"
		if strings.HasSuffix(f, ".go") {
			content = "package x\n"
		}
		writeFile(t, root, f, content)
	}
	r.g.run("add", "-A")
	r.g.run("commit", "-q", "-m", "はじめ")
	if remote != "" {
		r.g.run("remote", "add", "origin", remote)
	}
	if originHEAD {
		r.g.run("update-ref", "refs/remotes/origin/main", "HEAD")
		r.g.run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	}
	return r
}

func (r *initRepo) exists(path string) bool {
	_, err := os.Lstat(filepath.Join(r.root, path))
	return err == nil
}

// ---- 擬似端末の画面 ----

// vtScreen は、擬似端末に書かれた文字から、今の画面の文字を組み立てる小さな端末。huh は、問いを書くたびに
// カーソルを上へ戻して消し（CSI A・CSI J）、描き直すので、書かれた文字を並べただけでは今の問いが読めない。
// 扱うのは、改行・復帰・後退・カーソルの移動・消去だけ。色などほかの制御は読み捨てる。行は捨てずに下へ伸ばす。
type vtScreen struct {
	lines              [][]rune
	row                int
	col                int
	savedRow, savedCol int
	pending            []byte
}

// cellWidth は、端末で文字が占める桁の数の見積もり。日本語などの全角は 2 桁にする（行の中のカーソルの移動のため）。
func cellWidth(r rune) int {
	if r >= 0x1100 && (r <= 0x115f || (r >= 0x2e80 && r <= 0xa4cf) || (r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) || (r >= 0xfe30 && r <= 0xfe4f) || (r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6)) {
		return 2
	}
	return 1
}

func (v *vtScreen) line() []rune {
	for len(v.lines) <= v.row {
		v.lines = append(v.lines, nil)
	}
	return v.lines[v.row]
}

// put は、今の行の col 桁目に文字を書く。全角の 2 桁目は 0 で埋める。
func (v *vtScreen) put(r rune) {
	l := v.line()
	w := cellWidth(r)
	for len(l) < v.col+w {
		l = append(l, ' ')
	}
	l[v.col] = r
	if w == 2 {
		l[v.col+1] = 0
	}
	v.lines[v.row] = l
	v.col += w
}

func (v *vtScreen) feed(b []byte) {
	data := append(v.pending, b...)
	v.pending = nil
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == 0x1b:
			n, ok := v.escape(data[i:])
			if !ok {
				v.pending = append([]byte(nil), data[i:]...)
				return
			}
			i += n
			continue
		case c == '\r':
			v.col = 0
		case c == '\n':
			v.row++
			v.line()
		case c == '\b':
			if v.col > 0 {
				v.col--
			}
		case c == '\t':
			v.col = (v.col/8 + 1) * 8
		case c < 0x20 || c == 0x7f:
		default:
			r, size := utf8.DecodeRune(data[i:])
			if r == utf8.RuneError && size == 1 && !utf8.FullRune(data[i:]) {
				v.pending = append([]byte(nil), data[i:]...)
				return
			}
			v.put(r)
			i += size
			continue
		}
		i++
	}
}

var csiPattern = regexp.MustCompile(`^\x1b\[([0-9;?<>=$!]*)([ -/]*)([@-~])`)

// escape は、ESC から始まる制御を 1 つ読み、使った長さを返す。まだ終わりまで届いていなければ ok が false。
func (v *vtScreen) escape(d []byte) (int, bool) {
	if len(d) < 2 {
		return 0, false
	}
	switch d[1] {
	case '[':
		m := csiPattern.FindSubmatch(d)
		if m == nil {
			if len(d) > 64 {
				return 2, true // 読めない制御は ESC [ だけ捨てる
			}
			return 0, false
		}
		v.csi(string(m[1]), string(m[2]), m[3][0])
		return len(m[0]), true
	case ']', 'P', '_', '^': // OSC・DCS など。BEL か ST（ESC \）まで捨てる
		for j := 2; j < len(d); j++ {
			if d[j] == 0x07 {
				return j + 1, true
			}
			if d[j] == 0x1b && j+1 < len(d) && d[j+1] == '\\' {
				return j + 2, true
			}
		}
		return 0, false
	case 'M': // 1 行上へ（逆の改行）。huh は、選択肢のカーソルを動かすときに使う
		if v.row > 0 {
			v.row--
		}
		return 2, true
	case '7':
		v.savedRow, v.savedCol = v.row, v.col
		return 2, true
	case '8':
		v.row, v.col = v.savedRow, v.savedCol
		return 2, true
	default:
		return 2, true
	}
}

func (v *vtScreen) csi(params, inter string, final byte) {
	if inter != "" || strings.ContainsAny(params, "?<>=$!") {
		return // 端末の動きの切り替えと問い合わせは、画面の文字に関わらない
	}
	n := 1
	if p := strings.Split(params, ";")[0]; p != "" {
		fmt.Sscanf(p, "%d", &n)
	}
	switch final {
	case 'A':
		v.row = max(0, v.row-n)
	case 'B':
		v.row += n
	case 'C':
		v.col += n
	case 'D':
		v.col = max(0, v.col-n)
	case 'G':
		v.col = max(0, n-1)
	case 'J':
		mode := 0
		fmt.Sscanf(params, "%d", &mode)
		if mode == 2 || mode == 3 {
			v.lines = nil
			return
		}
		l := v.line()
		if v.col < len(l) {
			v.lines[v.row] = l[:v.col]
		}
		if mode == 0 && v.row+1 < len(v.lines) {
			v.lines = v.lines[:v.row+1]
		}
	case 'K':
		mode := 0
		fmt.Sscanf(params, "%d", &mode)
		l := v.line()
		switch mode {
		case 0:
			if v.col < len(l) {
				v.lines[v.row] = l[:v.col]
			}
		case 2:
			v.lines[v.row] = nil
		}
	}
}

func (v *vtScreen) text() string {
	var b strings.Builder
	for i, l := range v.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		var s strings.Builder
		for _, r := range l {
			if r != 0 {
				s.WriteRune(r)
			}
		}
		b.WriteString(strings.TrimRight(s.String(), " "))
	}
	return b.String()
}

// ---- 擬似端末で動かす ----

type initSession struct {
	t      *testing.T
	ptmx   *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	out    bytes.Buffer
	vt     vtScreen
	stderr bytes.Buffer
	done   chan struct{}
	err    error
}

// startInit は、標準入力と標準出力を擬似端末にして vanda init を動かす。標準エラーは別に受ける。
func startInit(t *testing.T, bin, dir string, env []string) *initSession {
	t.Helper()
	s := &initSession{t: t, done: make(chan struct{})}
	s.cmd = exec.Command(bin, "init")
	s.cmd.Dir = dir
	s.cmd.Env = append(append([]string(nil), env...), "TERM=xterm-256color")
	s.cmd.Stderr = &lockedWriter{mu: &s.mu, w: &s.stderr}
	ptmx, err := pty.StartWithSize(s.cmd, &pty.Winsize{Rows: 50, Cols: 120})
	if err != nil {
		t.Fatalf("擬似端末で vanda init を動かせない: %v", err)
	}
	s.ptmx = ptmx
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.out.Write(buf[:n])
				s.vt.feed(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		s.err = s.cmd.Wait()
		close(s.done)
	}()
	t.Cleanup(func() {
		select {
		case <-s.done:
		default:
			_ = s.cmd.Process.Kill()
			<-s.done
		}
		_ = ptmx.Close()
	})
	return s
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (s *initSession) screen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vt.text()
}

// all は、画面に出た文字を返す（消された問いは残らないが、作った後の案内は残る）。
func (s *initSession) all() string { return s.screen() }

func (s *initSession) keys(k string) {
	s.t.Helper()
	if _, err := s.ptmx.Write([]byte(k)); err != nil {
		s.t.Fatalf("キー %q を送れない: %v", k, err)
	}
}

const (
	keyEnter = "\r"
	keyUp    = "\x1b[A"
	keyDown  = "\x1b[B"
	keySpace = " "
	keyClear = "\x15" // Ctrl+U。入力の欄の、カーソルより前を消す
	keyEOF   = "\x04" // Ctrl+D
)

// press は、キーを送り、画面が変わるのを 1 秒待つ。変わらなければ、送り直す（3 度まで）。問いが入れ替わる間に
// 送ったキーを、vanda 側が読み落とすことがあるため。画面が変わったかを返す。
func (s *initSession) press(k string) bool {
	s.t.Helper()
	before := s.settle()
	for try := 0; try < 3; try++ {
		s.keys(k)
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-s.done:
				return true
			default:
			}
			if s.screen() != before {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return false
}

// initField は、画面の一番下の問い（行の頭が ┃ のまとまり）。
type initField struct {
	title string
	lines []string // 2 行目から。行の頭の ┃ と、続く空白 1 つを除いたもの
}

func fieldOf(screen string) (initField, bool) {
	ls := strings.Split(screen, "\n")
	end := -1
	for i := len(ls) - 1; i >= 0; i-- {
		if strings.HasPrefix(ls[i], "┃") {
			end = i
			break
		}
	}
	if end < 0 {
		return initField{}, false
	}
	start := end
	for start > 0 && strings.HasPrefix(ls[start-1], "┃") {
		start--
	}
	var f initField
	for i := start; i <= end; i++ {
		l := strings.TrimPrefix(strings.TrimPrefix(ls[i], "┃"), " ")
		if i == start {
			f.title = strings.TrimSpace(l)
		} else {
			f.lines = append(f.lines, l)
		}
	}
	return f, true
}

// cursorLine は、カーソルのある行（> の付いた行）の、> を除いた中身を返す。
func (f initField) cursorLine() (string, bool) {
	for _, l := range f.lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, ">") {
			return strings.TrimSpace(strings.TrimPrefix(t, ">")), true
		}
	}
	return "", false
}

// errorText は、画面の一番下の問いより後に出た、理由（* で始まり。を含む行）を返す。
func errorText(screen string) string {
	ls := strings.Split(screen, "\n")
	last := -1
	for i, l := range ls {
		if strings.HasPrefix(l, "┃") {
			last = i
		}
	}
	for _, l := range ls[last+1:] {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "*") && strings.Contains(t, "。") {
			return t
		}
	}
	return ""
}

// waitField は、題に keyword を含む問いが画面の一番下に出て、200 ミリ秒描き直されなくなるまで待つ。
func (s *initSession) waitField(what, keyword string) initField {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	last, since := "", time.Now()
	for time.Now().Before(deadline) {
		sc := s.screen()
		if sc != last {
			last, since = sc, time.Now()
		} else if f, ok := fieldOf(sc); ok && strings.Contains(f.title, keyword) && time.Since(since) > 200*time.Millisecond {
			return f
		}
		select {
		case <-s.done:
			if f, ok := fieldOf(s.screen()); !ok || !strings.Contains(f.title, keyword) {
				s.t.Fatalf("%s の問いを待っている間に vanda init が終わった\n--- 画面\n%s\n--- 標準エラー\n%s", what, s.screen(), s.stderr.String())
			}
		default:
		}
		time.Sleep(30 * time.Millisecond)
	}
	s.t.Fatalf("%s の問い（題に「%s」）が出ない\n--- 画面\n%s", what, keyword, s.screen())
	return initField{}
}

// settle は、画面が 200 ミリ秒描き直されなくなるまで待つ。
func (s *initSession) settle() string {
	deadline := time.Now().Add(5 * time.Second)
	last, since := s.screen(), time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
		sc := s.screen()
		if sc != last {
			last, since = sc, time.Now()
		} else if time.Since(since) > 200*time.Millisecond {
			return sc
		}
	}
	return last
}

// wait は、vanda init が終わるのを待ち、終了コードを返す。
func (s *initSession) wait() int {
	s.t.Helper()
	select {
	case <-s.done:
	case <-time.After(15 * time.Second):
		s.t.Fatalf("vanda init が終わらない\n--- 画面\n%s", s.screen())
	}
	time.Sleep(100 * time.Millisecond)
	var exitErr *exec.ExitError
	if errors.As(s.err, &exitErr) {
		return exitErr.ExitCode()
	}
	if s.err != nil {
		s.t.Fatalf("vanda init の終わりを待てない: %v", s.err)
	}
	return 0
}

// ---- 問いをたどる ----

var storyChoices = []string{"GitHub Issues", "外部のコマンド", "使わない"}

var langNames = []string{"Go", "シナリオ", "TLA+", "SQL"}

func langOf(line string) string {
	for _, n := range langNames {
		if strings.Contains(line, n) {
			return n
		}
	}
	return ""
}

// initPlan は、問いへの答え。空の文字列は、既定の答えのまま Enter を押すこと。
type initPlan struct {
	base      string
	story     string // 選択肢の名前
	detail    string // リポジトリかコマンド
	idPattern string
	toggle    []string          // 言語を選ぶ問いで、選びを切り替える言語
	patterns  map[string]string // 言語 → 読むファイルのパターン
	confirm   string            // y・n
	// 形に合わない答え。問い（detail・id）ごとに、正しい答えの前に順に書く
	invalid map[string][]string
}

type initSeen struct {
	baseDefault  string
	storyDefault string // 選択肢の名前
	storyOptions []string
	detailAsked  bool
	detailDef    string
	idDefault    string
	langOptions  []string
	langSelected []string // 選ぶ問いが始まったときに選ばれていた言語
	langsAsked   []string // パターンを聞かれた言語
	patterns     map[string]string
}

// input は、入力の問いで、欄に入っている既定の答えを読み、形に合わない答えを順に書いて、そのたびに理由が出て
// 同じ問いにとどまることを確かめ、最後に a を書く（空なら既定のまま）。
func (s *initSession) input(what, keyword string, invalid []string, a string) string {
	s.t.Helper()
	f := s.waitField(what, keyword)
	def, _ := f.cursorLine()
	for _, bad := range invalid {
		if cur, _ := f.cursorLine(); cur != "" {
			s.press(keyClear)
		}
		if bad != "" {
			s.press(bad)
		}
		s.press(keyEnter)
		s.settle()
		f, _ = fieldOf(s.screen())
		again, ok := fieldOf(s.screen())
		if !ok || !strings.Contains(again.title, keyword) {
			s.t.Errorf("%s に形に合わない答え %q を書いても、同じ問いにとどまらない\n%s", what, bad, s.screen())
		} else if errorText(s.screen()) == "" {
			s.t.Errorf("%s に形に合わない答え %q を書いても、理由が出ない\n%s", what, bad, s.screen())
		}
	}
	if a != "" {
		if cur, _ := f.cursorLine(); cur != "" {
			s.press(keyClear)
		}
		s.press(a)
	}
	if !s.press(keyEnter) {
		s.t.Fatalf("%s で Enter を押しても先へ進まない\n%s", what, s.screen())
	}
	return def
}

// selectOne は、1 つ選ぶ問いで、カーソルのある選択肢（既定）を読み、want を選ぶ（空なら既定のまま）。
func (s *initSession) selectOne(what, keyword string, options []string, want string) (string, []string) {
	s.t.Helper()
	f := s.waitField(what, keyword)
	def, _ := f.cursorLine()
	var shown []string
	for _, l := range f.lines {
		for _, o := range options {
			if strings.Contains(l, o) {
				shown = append(shown, o)
			}
		}
	}
	// 選択肢は、一番下で下へ動かすと一番上へ戻る。今のカーソルから下へ動かして want を探す
	if want != "" {
		for i := 0; i <= len(options); i++ {
			f, _ := fieldOf(s.settle())
			if cur, _ := f.cursorLine(); strings.Contains(cur, want) {
				break
			}
			s.press(keyDown)
		}
	}
	s.press(keyEnter)
	return def, shown
}

// multiSelect は、複数選ぶ問いで、カーソルを一番上へ動かしてから一番下まで動かして、選択肢と選ばれているか（✓）を
// 読み、toggle にある言語の選びをスペースで切り替える。問いの題も返す。複数選ぶ問いは、1 つ選ぶ問いと違い、
// 端で止まる（一番下で下へ動かしても一番上へ戻らない）。カーソルは、はじめに選ばれていた言語から始まる。
func (s *initSession) multiSelect(what, keyword string, toggle []string) (title string, options, selected []string) {
	s.t.Helper()
	title = s.waitField(what, keyword).title
	for i := 0; i <= len(langNames) && s.press(keyUp); i++ {
	}
	seen := map[string]bool{}
	for i := 0; i < len(langNames)+2; i++ {
		f, _ := fieldOf(s.settle())
		cur, _ := f.cursorLine()
		lang := langOf(cur)
		if lang == "" || seen[lang] {
			break
		}
		seen[lang] = true
		options = append(options, lang)
		if strings.Contains(cur, "✓") {
			selected = append(selected, lang)
		}
		for _, tg := range toggle {
			if tg == lang {
				s.press(keySpace)
			}
		}
		if !s.press(keyDown) {
			break
		}
	}
	// 並びは langNames の中の順に直す（読んだ順は画面の上からの順と同じだが、比べやすくするため）
	order := func(xs []string) []string {
		var out []string
		for _, n := range langNames {
			for _, x := range xs {
				if x == n {
					out = append(out, n)
				}
			}
		}
		return out
	}
	s.press(keyEnter)
	return title, order(options), order(selected)
}

// waitNext は、題が prev でない問いが画面の一番下に出て、描き直されなくなるまで待つ。
func (s *initSession) waitNext(what, prev string) initField {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if f, ok := fieldOf(s.settle()); ok && f.title != prev {
			return f
		}
		select {
		case <-s.done:
			s.t.Fatalf("%s の問いを待っている間に vanda init が終わった\n--- 画面\n%s\n--- 標準エラー\n%s", what, s.screen(), s.stderr.String())
		default:
		}
	}
	s.t.Fatalf("%s の問いが出ない\n--- 画面\n%s", what, s.screen())
	return initField{}
}

// run は、計画の順に問いをたどり、plan の答えを書く。作るかの問いに答えたところで返す。
func (s *initSession) run(plan initPlan) initSeen {
	s.t.Helper()
	seen := initSeen{patterns: map[string]string{}}
	seen.baseDefault = s.input("ベースブランチ", "ベースブランチ", nil, plan.base)
	def, shown := s.selectOne("ストーリーの置き場所", "ストーリーの置き場所", storyChoices, plan.story)
	seen.storyOptions = shown
	for _, o := range storyChoices {
		if strings.Contains(def, o) {
			seen.storyDefault = o
		}
	}
	chosen := plan.story
	if chosen == "" {
		chosen = seen.storyDefault
	}
	switch chosen {
	case "GitHub Issues":
		seen.detailAsked = true
		seen.detailDef = s.input("リポジトリ", "リポジトリ", plan.invalid["detail"], plan.detail)
	case "外部のコマンド":
		seen.detailAsked = true
		seen.detailDef = s.input("コマンド", "コマンド", plan.invalid["detail"], plan.detail)
	}
	seen.idDefault = s.input("ID の書き方", "ID", plan.invalid["id"], plan.idPattern)
	var prev string
	prev, seen.langOptions, seen.langSelected = s.multiSelect("言語", "言語", plan.toggle)
	for {
		f := s.waitNext("言語のパターンか作るか", prev)
		if strings.Contains(f.title, "作りますか") {
			if plan.confirm == "n" {
				s.press("n")
			} else {
				s.press("y")
			}
			return seen
		}
		lang := langOf(f.title)
		if lang == "" || !strings.Contains(f.title, "読むファイル") {
			s.t.Fatalf("言語ごとのパターンの問いでも、作るかの問いでもない\n%s", s.screen())
		}
		seen.langsAsked = append(seen.langsAsked, lang)
		seen.patterns[lang] = s.input(lang+" のパターン", f.title, nil, plan.patterns[lang])
		prev = f.title
	}
}

// guidance は、作った後の案内（「次にすること」から後）を返す。
func (s *initSession) guidance() string {
	sc := s.screen()
	if i := strings.Index(sc, "次にすること"); i >= 0 {
		return sc[i:]
	}
	return ""
}

// ---- 作ったファイル ----

type initConfig struct {
	Version    string           `toml:"version"`
	Layer      []map[string]any `toml:"layer"`
	Annotation []struct {
		Paths   []string `toml:"paths"`
		Adapter string   `toml:"adapter"`
		Command []string `toml:"command"`
	} `toml:"annotation"`
	Story *struct {
		IDPattern  string   `toml:"id_pattern"`
		Adapter    string   `toml:"adapter"`
		Repository string   `toml:"repository"`
		Command    []string `toml:"command"`
	} `toml:"story"`
	Gui *struct {
		Base string `toml:"base"`
	} `toml:"gui"`
}

func readInitConfig(t *testing.T, root string) initConfig {
	t.Helper()
	var c initConfig
	md, err := toml.DecodeFile(filepath.Join(root, ".vanda", "config.toml"), &c)
	if err != nil {
		t.Fatalf("作った設定を読めない: %v", err)
	}
	// promises・agreements は既定のままなので書かない。ほかにも決まっていない項目を書かない
	if u := md.Undecoded(); len(u) > 0 {
		t.Errorf("設定に、書かないはずの項目がある: %v", u)
	}
	return c
}

// annotationsOf は、設定の [[annotation]] を「アダプタ パターン」の並びにする（順は問わない）。
func annotationsOf(c initConfig) []string {
	var out []string
	for _, a := range c.Annotation {
		out = append(out, a.Adapter+" "+strings.Join(a.Paths, ",")+strings.Join(a.Command, " "))
	}
	sort.Strings(out)
	return out
}

// 計画の「問い 5 の言語の見つけ方と既定のパターン」の表
var initLangs = []struct {
	name, file, pattern, adapter string
}{
	{"Go", "internal/x/x_test.go", "**/*_test.go", "go"},
	{"シナリオ", "e2e/a.txtar", "**/*.txtar", "txtar"},
	{"TLA+", "spec/S.tla", "**/*.tla", "tla"},
	{"SQL", "db/schema.sql", "**/*.sql", "sql"},
}

// checkNoComments は、作った 4 つのファイルにコメントの行がないことを確かめる。
func checkNoComments(t *testing.T, root string) {
	t.Helper()
	for _, f := range []string{"config.toml", "promises.toml", "agreements.toml", "checks/ci.toml"} {
		data, err := os.ReadFile(filepath.Join(root, ".vanda", f))
		if err != nil {
			t.Errorf(".vanda/%s ができていない: %v", f, err)
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				t.Errorf(".vanda/%s の %d 行目がコメント: %q", f, i+1, line)
			}
		}
	}
}

func initSetup(t *testing.T) (string, string) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	return bin, version
}

// ---- テスト ----

// 偽の GitHub。vanda doctor が呼ぶ GET /repos/<owner>/<repo> に、Issues が使えるリポジトリとして応える
const fakeRepoToken = "gho_inittesttoken"

type fakeRepoAPI struct {
	URL  string
	mu   sync.Mutex
	hits int
}

func newFakeRepoAPI(t *testing.T, repo string) *fakeRepoAPI {
	t.Helper()
	f := &fakeRepoAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		if req.Method != http.MethodGet || req.URL.Path != "/repos/"+repo || (auth != "Bearer "+fakeRepoToken && auth != "token "+fakeRepoToken) {
			writeGitHubError(w, http.StatusNotFound, "Not Found")
			return
		}
		f.mu.Lock()
		f.hits++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"full_name":"` + repo + `","has_issues":true}`))
	}))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

func (f *fakeRepoAPI) called() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits > 0
}

// CMD-11
func Test既定の答えのまま進めると読み取った値で4つのファイルができdoctorとciの審査が通る(t *testing.T) {
	t.Parallel()
	bin, version := initSetup(t)
	var files []string
	for _, l := range initLangs {
		files = append(files, l.file)
	}
	r := newInitRepo(t, "https://github.com/owner/app.git", true, files...)
	s := startInit(t, bin, r.root, r.g.env)
	seen := s.run(initPlan{})
	if code := s.wait(); code != 0 {
		t.Fatalf("vanda init が %d で終わった\n%s\n%s", code, s.screen(), s.stderr.String())
	}
	if seen.baseDefault != "origin/main" || seen.storyDefault != "GitHub Issues" || seen.detailDef != "owner/app" || seen.idDefault != "#[0-9]+" {
		t.Errorf("既定の答え: %+v", seen)
	}

	c := readInitConfig(t, r.root)
	if c.Version != version {
		t.Errorf("version が %q（動いている vanda は %q）", c.Version, version)
	}
	if len(c.Layer) != 0 {
		t.Errorf("[[layer]] を書いている: %v", c.Layer)
	}
	if c.Gui == nil || c.Gui.Base != "origin/main" {
		t.Errorf("[gui] の base: %+v", c.Gui)
	}
	if c.Story == nil || c.Story.Adapter != "github" || c.Story.Repository != "owner/app" || c.Story.IDPattern != "#[0-9]+" || c.Story.Command != nil {
		t.Errorf("[story]: %+v", c.Story)
	}
	var want []string
	for _, l := range initLangs {
		want = append(want, l.adapter+" "+l.pattern)
	}
	sort.Strings(want)
	if got := annotationsOf(c); !reflect.DeepEqual(got, want) {
		t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
	}
	for _, f := range []string{"promises.toml", "agreements.toml"} {
		data, err := os.ReadFile(filepath.Join(r.root, ".vanda", f))
		if err != nil || strings.TrimSpace(string(data)) != "" {
			t.Errorf(".vanda/%s が空でない（%v）: %q", f, err, data)
		}
	}
	checkNoComments(t, r.root)
	// 作った後の案内に、[[layer]] を書くことと、CI に足す vanda check が出る
	g := s.guidance()
	if !strings.Contains(g, "[[layer]]") {
		t.Errorf("案内に [[layer]] を書くことが出ていない\n%s", s.screen())
	}
	if !strings.Contains(g, "vanda check .vanda/checks/ci.toml --base origin/main") {
		t.Errorf("案内に、CI に足す vanda check が出ていない\n%s", s.screen())
	}
	// コミットはしない
	if n := r.g.run("rev-list", "--count", "HEAD"); n != "1" {
		t.Errorf("vanda init がコミットした（%s 個）", n)
	}

	// vanda doctor は GitHub に繋がるかを確かめるので、作った設定の api_url だけを偽の GitHub に向ける
	gh := newFakeRepoAPI(t, "owner/app")
	cfgText, err := os.ReadFile(filepath.Join(r.root, ".vanda", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, r.root, ".vanda/config.toml", strings.Replace(string(cfgText), "[story]\n", "[story]\napi_url = \""+gh.URL+"\"\n", 1))
	env := append(append([]string(nil), r.g.env...), "GH_TOKEN="+fakeRepoToken)
	doctor := exec.Command(bin, "doctor")
	doctor.Dir, doctor.Env = r.root, env
	if out, err := doctor.CombinedOutput(); err != nil {
		t.Errorf("作った設定で vanda doctor が通らない: %v\n%s", err, out)
	}
	if !gh.called() {
		t.Errorf("vanda doctor が、偽の GitHub のリポジトリを読みに来ていない")
	}
	// 作った直後のブランチにはストーリーの ID のあるコミットがないので、--story がなければストーリーが分からず止まる
	check := exec.Command(bin, "check", ".vanda/checks/ci.toml", "--base", "origin/main")
	check.Dir, check.Env = r.root, env
	if out, err := check.CombinedOutput(); err == nil || !strings.Contains(string(out), "ストーリー") {
		t.Errorf("--story なしの vanda check が、ストーリーが分からないとして止まらない: %v\n%s", err, out)
	}
	check = exec.Command(bin, "check", ".vanda/checks/ci.toml", "--base", "origin/main", "--story", "#1")
	check.Dir, check.Env = r.root, env
	if out, err := check.CombinedOutput(); err != nil {
		t.Errorf("作った審査の設定で vanda check --story #1 が通らない: %v\n%s", err, out)
	}
}

// CMD-11: 審査の設定には、今ある審査を全部、docs/formats.md の「審査の種類」の順に、審査ごとに決まった範囲で書く
func Test審査の設定には今ある審査が全部docs_formats_mdの順に決まった範囲で入る(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	r := newInitRepo(t, "", false)
	s := startInit(t, bin, r.root, r.g.env)
	s.run(initPlan{idPattern: "#[0-9]+"})
	if code := s.wait(); code != 0 {
		t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
	}
	var doc struct {
		Check []struct {
			Name  string `toml:"name"`
			Scope string `toml:"scope"`
		} `toml:"check"`
	}
	if _, err := toml.DecodeFile(filepath.Join(r.root, ".vanda", "checks", "ci.toml"), &doc); err != nil {
		t.Fatalf("審査の設定を読めない: %v", err)
	}
	var got []string
	for _, c := range doc.Check {
		got = append(got, c.Name+" "+c.Scope)
	}
	// 書ける範囲が 1 つのものはそれ。all と story の両方を書ける agreement と evidence-present は、ストーリーで変わったものを見る story
	want := []string{
		"promise-list-shape all", "annotation-links all", "story-origin story", "agreement story",
		"agreement-targets all", "evidence-present story", "unread-files all",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("審査の設定: %v（思ったもの %v）", got, want)
	}
	checkNoComments(t, r.root)
}

// CMD-11: remote の URL の形ごとの、ストーリーの置き場所とリポジトリの既定の答え
func Test_remoteのURLの形ごとにストーリーの置き場所とリポジトリの既定の答えが決まる(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	cases := []struct {
		name, url string
		github    bool
	}{
		{"httpsの形", "https://github.com/owner/app.git", true},
		{"httpsの形で.gitのないもの", "https://github.com/owner/app", true},
		{"sshの形", "git@github.com:owner/app.git", true},
		{"ssh_URLの形", "ssh://git@github.com/owner/app.git", true},
		{"sshの別名のホスト", "git@github.com_work:owner/app.git", true},
		{"GitHubでないホストのhttps", "https://gitlab.com/owner/app.git", false},
		{"GitHubでないホストのssh", "git@gitlab.com:owner/app.git", false},
		{"remoteなし", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newInitRepo(t, c.url, c.url != "")
			// 既定の置き場所を読んだ後、GitHub Issues を選んで、リポジトリの既定を見る
			s := startInit(t, bin, r.root, r.g.env)
			seen := s.run(initPlan{story: "GitHub Issues", detail: "owner/app"})
			s.wait()
			wantStory := "使わない"
			if c.github {
				wantStory = "GitHub Issues"
			}
			if seen.storyDefault != wantStory {
				t.Errorf("ストーリーの置き場所の既定: %q（思ったもの %q）", seen.storyDefault, wantStory)
			}
			if !reflect.DeepEqual(seen.storyOptions, storyChoices) {
				t.Errorf("ストーリーの置き場所の選択肢: %v（思ったもの %v）", seen.storyOptions, storyChoices)
			}
			wantDetail := ""
			if c.github {
				wantDetail = "owner/app"
			}
			if seen.detailDef != wantDetail {
				t.Errorf("リポジトリの既定: %q（思ったもの %q。空は既定なし）", seen.detailDef, wantDetail)
			}
			wantBase := "main"
			if c.url != "" {
				wantBase = "origin/main"
			}
			if seen.baseDefault != wantBase {
				t.Errorf("ベースブランチの既定: %q（思ったもの %q）", seen.baseDefault, wantBase)
			}
		})
	}
}

// CMD-11: ストーリーの置き場所の選択肢の全部で、問い 3・4 の出方と既定と、書かれる [story] が合う
func Testストーリーの置き場所の選択肢ごとに問いの出方と既定とstoryの中身が合う(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	cases := []struct {
		choice      string
		detail      string
		idPattern   string
		detailAsked bool
		detailDef   string // 空なら既定なし
		idDefault   string // 空なら既定なし
		adapter     string
		repository  string
		command     []string
		wantPattern string
	}{
		{"GitHub Issues", "", "", true, "owner/app", "#[0-9]+", "github", "owner/app", nil, "#[0-9]+"},
		// 外部のコマンドの答えは、空白で区切って引数に分ける（引用符は扱わない）
		{"外部のコマンド", "tools/story-adapter --strict", "PROJ-[0-9]+", true, "", "", "", "", []string{"tools/story-adapter", "--strict"}, "PROJ-[0-9]+"},
		{"使わない", "", "PROJ-[0-9]+", false, "", "", "", "", nil, "PROJ-[0-9]+"},
	}
	for _, c := range cases {
		t.Run(c.choice, func(t *testing.T) {
			t.Parallel()
			r := newInitRepo(t, "https://github.com/owner/app.git", true)
			s := startInit(t, bin, r.root, r.g.env)
			seen := s.run(initPlan{story: c.choice, detail: c.detail, idPattern: c.idPattern})
			if code := s.wait(); code != 0 {
				t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
			}
			if seen.detailAsked != c.detailAsked {
				t.Errorf("リポジトリかコマンドの問いが出たか: %v（思ったもの %v）", seen.detailAsked, c.detailAsked)
			}
			if seen.detailDef != c.detailDef {
				t.Errorf("リポジトリかコマンドの既定: %q（思ったもの %q）", seen.detailDef, c.detailDef)
			}
			if seen.idDefault != c.idDefault {
				t.Errorf("ID の書き方の既定: %q（思ったもの %q。空は既定なし）", seen.idDefault, c.idDefault)
			}
			st := readInitConfig(t, r.root).Story
			if st == nil || st.Adapter != c.adapter || st.Repository != c.repository || !reflect.DeepEqual(st.Command, c.command) || st.IDPattern != c.wantPattern {
				t.Errorf("[story]: %+v（思ったもの adapter %q、repository %q、command %v、id_pattern %q）", st, c.adapter, c.repository, c.command, c.wantPattern)
			}
		})
	}
}

// CMD-11: 言語のあるなしの 16 通りで、言語を選ぶ問いに 4 つの言語が出て、見つかった言語が選ばれた状態から始まり、
// そのまま進めると、見つかった言語だけのパターンを聞かれ、その [[annotation]] が書かれる
func Test言語のあるなしの16通りで見つかった言語が選ばれた状態から始まりそのまま進めるとそれが書かれる(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	for mask := 0; mask < 1<<len(initLangs); mask++ {
		var files, names, want []string
		for i, l := range initLangs {
			if mask&(1<<i) != 0 {
				files = append(files, l.file)
				names = append(names, l.name)
				want = append(want, l.adapter+" "+l.pattern)
			}
		}
		label := strings.Join(names, "と")
		if label == "" {
			label = "なし"
		}
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			r := newInitRepo(t, "", false, files...)
			s := startInit(t, bin, r.root, r.g.env)
			seen := s.run(initPlan{idPattern: "#[0-9]+"})
			if code := s.wait(); code != 0 {
				t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
			}
			if !reflect.DeepEqual(seen.langOptions, langNames) {
				t.Errorf("言語を選ぶ問いの選択肢: %v（思ったもの %v）", seen.langOptions, langNames)
			}
			sorted := func(xs []string) []string { ys := append([]string(nil), xs...); sort.Strings(ys); return ys }
			if !reflect.DeepEqual(sorted(seen.langSelected), sorted(names)) {
				t.Errorf("はじめから選ばれていた言語: %v（思ったもの %v）", seen.langSelected, names)
			}
			if !reflect.DeepEqual(sorted(seen.langsAsked), sorted(names)) {
				t.Errorf("パターンを聞かれた言語: %v（思ったもの %v）", seen.langsAsked, names)
			}
			for _, l := range initLangs {
				if p, ok := seen.patterns[l.name]; ok && p != l.pattern {
					t.Errorf("%s のパターンの既定: %q（思ったもの %q）", l.name, p, l.pattern)
				}
			}
			sort.Strings(want)
			if got := annotationsOf(readInitConfig(t, r.root)); !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
				t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
			}
		})
	}
}

// CMD-11: 見つからなかった言語を選ぶと書かれ、選びを外すと書かれない
func Test言語を選ぶ問いで見つからなかった言語を選ぶと書かれ選びを外すと書かれない(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	r := newInitRepo(t, "", false, "e2e/a.txtar", "db/schema.sql")
	s := startInit(t, bin, r.root, r.g.env)
	// シナリオと SQL が見つかる。Go を選び、シナリオの選びを外す
	seen := s.run(initPlan{idPattern: "#[0-9]+", toggle: []string{"Go", "シナリオ"}})
	if code := s.wait(); code != 0 {
		t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
	}
	if got := append([]string(nil), seen.langsAsked...); !reflect.DeepEqual(sortedCopy(got), []string{"Go", "SQL"}) {
		t.Errorf("パターンを聞かれた言語: %v（思ったもの Go と SQL）", got)
	}
	if got, want := annotationsOf(readInitConfig(t, r.root)), []string{"go **/*_test.go", "sql **/*.sql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
	}
}

func sortedCopy(xs []string) []string {
	ys := append([]string(nil), xs...)
	sort.Strings(ys)
	return ys
}

// newEmptyInitRepo は、git init しただけの（コミットもファイルもない）リポジトリを作る。
func newEmptyInitRepo(t *testing.T) *initRepo {
	t.Helper()
	root := t.TempDir()
	r := &initRepo{t: t, root: root, g: newRVGit(t, root)}
	r.g.run("init", "-q", "-b", "main")
	return r
}

// CMD-11: 初期化した直後のリポジトリでも、言語を選んで [[annotation]] を書ける。1 つも選ばなければ、案内に
// [[annotation]] を書くことが足される（外部のコマンドのアダプタのことは、いつも出る）
func Test初期化した直後のリポジトリでも言語を選べ1つも選ばなければ案内にannotationを書くことが出る(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	annotationLines := func(g string) int {
		n := 0
		for _, l := range strings.Split(g, "\n") {
			if strings.Contains(l, "[[annotation]]") {
				n++
			}
		}
		return n
	}
	var withLangs, without string
	t.Run("GoとTLA+を選ぶ", func(t *testing.T) {
		r := newEmptyInitRepo(t)
		s := startInit(t, bin, r.root, r.g.env)
		seen := s.run(initPlan{idPattern: "#[0-9]+", toggle: []string{"Go", "TLA+"}})
		if code := s.wait(); code != 0 {
			t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
		}
		if seen.baseDefault != "main" {
			t.Errorf("ベースブランチの既定: %q（思ったもの main。今のブランチ）", seen.baseDefault)
		}
		if len(seen.langSelected) != 0 {
			t.Errorf("ファイルがないのに、はじめから選ばれていた言語がある: %v", seen.langSelected)
		}
		if got, want := annotationsOf(readInitConfig(t, r.root)), []string{"go **/*_test.go", "tla **/*.tla"}; !reflect.DeepEqual(got, want) {
			t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
		}
		if c := readInitConfig(t, r.root); c.Gui == nil || c.Gui.Base != "main" {
			t.Errorf("[gui] の base: %+v", c.Gui)
		}
		withLangs = s.guidance()
	})
	t.Run("1つも選ばない", func(t *testing.T) {
		r := newEmptyInitRepo(t)
		s := startInit(t, bin, r.root, r.g.env)
		s.run(initPlan{idPattern: "#[0-9]+"})
		if code := s.wait(); code != 0 {
			t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
		}
		if got := annotationsOf(readInitConfig(t, r.root)); len(got) != 0 {
			t.Errorf("言語を選んでいないのに [[annotation]] がある: %v", got)
		}
		without = s.guidance()
	})
	if !strings.Contains(withLangs, "外部のコマンド") || !strings.Contains(without, "外部のコマンド") {
		t.Errorf("案内に、外部のコマンドのアダプタのことが出ていない\n--- 選んだとき\n%s\n--- 選ばないとき\n%s", withLangs, without)
	}
	if annotationLines(without) <= annotationLines(withLangs) {
		t.Errorf("1 つも選ばなかったときの案内に、[[annotation]] を書くことが足されていない\n--- 選んだとき\n%s\n--- 選ばないとき\n%s", withLangs, without)
	}
}

// CMD-11: 答えを変えると、その値で書かれる（ベースブランチ、リポジトリ、ID の書き方、読むファイルのパターン。コマンドは
// ストーリーの置き場所の選択肢のテストで変える）
func Test答えを変えるとその値で書かれる(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	r := newInitRepo(t, "https://github.com/owner/app.git", true, "internal/x/x_test.go", "e2e/a.txtar")
	r.g.run("branch", "develop")
	s := startInit(t, bin, r.root, r.g.env)
	s.run(initPlan{
		base: "develop", detail: "other/repo", idPattern: "[A-Z]+-[0-9]+",
		patterns: map[string]string{"Go": "internal/**/*_test.go", "シナリオ": "e2e/**/*.txtar"},
	})
	if code := s.wait(); code != 0 {
		t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
	}
	c := readInitConfig(t, r.root)
	if c.Gui == nil || c.Gui.Base != "develop" {
		t.Errorf("[gui] の base: %+v", c.Gui)
	}
	if c.Story == nil || c.Story.Repository != "other/repo" || c.Story.IDPattern != "[A-Z]+-[0-9]+" {
		t.Errorf("[story]: %+v", c.Story)
	}
	if got, want := annotationsOf(c), []string{"go internal/**/*_test.go", "txtar e2e/**/*.txtar"}; !reflect.DeepEqual(got, want) {
		t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
	}
}

// CMD-11: 形に合わない答えは、理由が出て同じ問いにとどまる（選択肢は矢印キーで選ぶので、選択肢にない番号は書けない）
func Test形に合わない答えは理由が出て同じ問いにとどまる(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	t.Run("既定のないIDの書き方の空と正規表現の誤りと空の文字列に当たる正規表現", func(t *testing.T) {
		t.Parallel()
		r := newInitRepo(t, "", false)
		s := startInit(t, bin, r.root, r.g.env)
		s.run(initPlan{
			story: "外部のコマンド", detail: "tools/story-adapter", idPattern: "PROJ-[0-9]+",
			invalid: map[string][]string{"id": {"", "[", "a*"}},
		})
		if code := s.wait(); code != 0 {
			t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
		}
		if c := readInitConfig(t, r.root); c.Story == nil || c.Story.IDPattern != "PROJ-[0-9]+" || !reflect.DeepEqual(c.Story.Command, []string{"tools/story-adapter"}) {
			t.Errorf("聞き直した後の答えで書かれていない: %+v", c.Story)
		}
	})
	t.Run("リポジトリの形", func(t *testing.T) {
		t.Parallel()
		r := newInitRepo(t, "https://github.com/owner/app.git", true)
		s := startInit(t, bin, r.root, r.g.env)
		s.run(initPlan{detail: "other/repo", invalid: map[string][]string{"detail": {"owner", "owner/", "/repo", "a/b/c"}}})
		if code := s.wait(); code != 0 {
			t.Fatalf("vanda init が %d で終わった\n%s", code, s.screen())
		}
		if c := readInitConfig(t, r.root); c.Story == nil || c.Story.Repository != "other/repo" {
			t.Errorf("聞き直した後の答えで書かれていない: %+v", c.Story)
		}
	})
}

// CMD-11: 作る前の確かめで n と答えると、何も作られない
func Test作る前の確かめでnと答えると何も作られない(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	r := newInitRepo(t, "https://github.com/owner/app.git", true, "e2e/a.txtar")
	s := startInit(t, bin, r.root, r.g.env)
	s.run(initPlan{confirm: "n"})
	if code := s.wait(); code != 1 {
		t.Errorf("n と答えたときの終了コードが %d（1 のはず）", code)
	}
	if r.exists(".vanda") {
		t.Errorf("n と答えたのに .vanda/ ができた")
	}
}

// CMD-11: 途中で入力を終える（Ctrl+D）と、何も作られない。問いごとに回す
func Test途中で入力を終えると何も作られない(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	// 問いの題に出る言葉。remote が GitHub なのでリポジトリの問いが出る。シナリオだけが見つかる
	steps := []string{"ベースブランチ", "ストーリーの置き場所", "リポジトリ", "ID", "言語", "読むファイル", "作りますか"}
	for i, step := range steps {
		t.Run(step+"で終える", func(t *testing.T) {
			t.Parallel()
			r := newInitRepo(t, "https://github.com/owner/app.git", true, "e2e/a.txtar")
			s := startInit(t, bin, r.root, r.g.env)
			for j := 0; j < i; j++ {
				s.waitField(steps[j], steps[j])
				s.press(keyEnter)
			}
			s.waitField(step, step)
			s.press(keyEOF)
			code := s.wait()
			if r.exists(".vanda") {
				t.Errorf("%s で入力を終えたのに .vanda/ ができた", step)
			}
			if code == 0 {
				t.Errorf("%s で入力を終えたのに、終了コードが 0", step)
			}
		})
	}
}

// CMD-11: 止める場合は、何も書かれず、理由が標準エラーに出て、終了コードが 1
func Test止める場合は何も書かず理由を標準エラーに出し終了コード1で終わる(t *testing.T) {
	t.Parallel()
	bin, _ := initSetup(t)
	expectStop := func(t *testing.T, r *initRepo, cmd *exec.Cmd, stderr *bytes.Buffer, wait func() int, keyword string, before map[string]string) {
		t.Helper()
		if code := wait(); code != 1 {
			t.Errorf("終了コードが %d（1 のはず）", code)
		}
		if !strings.Contains(stderr.String(), keyword) || !strings.Contains(stderr.String(), "。") {
			t.Errorf("標準エラーに理由（%s）が出ていない: %q", keyword, stderr.String())
		}
		if got := snapshotDir(t, r.root); !reflect.DeepEqual(got, before) {
			t.Errorf("止めたのに、ファイルが変わった\n前 %v\n後 %v", before, got)
		}
	}

	// 端末でない。標準入力と標準出力の片方か両方が端末でない、の 3 通り
	for _, c := range []struct {
		name          string
		stdin, stdout bool // 端末か
	}{{"標準入力が端末でない", false, true}, {"標準出力が端末でない", true, false}, {"どちらも端末でない", false, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newInitRepo(t, "https://github.com/owner/app.git", true)
			before := snapshotDir(t, r.root)
			ptmx, tty, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer ptmx.Close()
			defer tty.Close()
			go func() { _, _ = bytes.NewBuffer(nil).ReadFrom(ptmx) }()
			cmd := exec.Command(bin, "init")
			cmd.Dir, cmd.Env = r.root, r.g.env
			var stdout, stderr bytes.Buffer
			cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(nil), &stdout, &stderr
			if c.stdin {
				cmd.Stdin = tty
			}
			if c.stdout {
				cmd.Stdout = tty
			}
			err = cmd.Start()
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			wait := func() int {
				select {
				case err := <-done:
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						return exitErr.ExitCode()
					}
					if err != nil {
						t.Fatal(err)
					}
					return 0
				case <-time.After(15 * time.Second):
					_ = cmd.Process.Kill()
					t.Fatalf("端末でないのに vanda init が止まらない（問いかけて待っている）")
					return -1
				}
			}
			expectStop(t, r, cmd, &stderr, wait, "端末", before)
		})
	}

	runPty := func(t *testing.T, r *initRepo, dir string, keyword string) {
		t.Helper()
		before := snapshotDir(t, r.root)
		s := startInit(t, bin, dir, r.g.env)
		expectStop(t, r, s.cmd, &s.stderr, s.wait, keyword, before)
	}
	t.Run("gitの作業ツリーの中でない", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		r := &initRepo{t: t, root: root, g: newRVGit(t, root)}
		runPty(t, r, root, "git")
	})
	t.Run(".vanda_がディレクトリである", func(t *testing.T) {
		t.Parallel()
		r := newInitRepo(t, "https://github.com/owner/app.git", true)
		writeFile(t, r.root, ".vanda/config.toml", "version = \"0.0.1\"\n")
		runPty(t, r, r.root, ".vanda")
	})
	t.Run(".vanda_がファイルである", func(t *testing.T) {
		t.Parallel()
		r := newInitRepo(t, "https://github.com/owner/app.git", true)
		writeFile(t, r.root, ".vanda", "ファイル\n")
		runPty(t, r, r.root, ".vanda")
	})
}

// snapshotDir は、dir の下のファイル（.git を除く）の場所と中身を返す。
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == ".git" && info.IsDir() {
			return filepath.SkipDir
		}
		if info.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(fmt.Errorf("%s を読めない: %w", dir, err))
	}
	return out
}

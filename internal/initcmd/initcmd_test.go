package initcmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/TadahiroYamamura/vanda/internal/initcmd"
)

// syncBuffer は、プロンプトの出力を受ける。プロンプトを出す goroutine と、待つテストの goroutine の両方が触る
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type askResult struct {
	a   initcmd.Answers
	err error
}

// askSession は、擬似端末で Ask を動かす。端末に出たものはバッファに写し、質問の題が出るのを待ってから、キーを 1 回だけ送る
type askSession struct {
	t    *testing.T
	in   interface{ Write([]byte) (int, error) }
	out  *syncBuffer
	pos  int // ここまでの出力は、前の質問のもの
	done chan askResult
}

func startAsk(t *testing.T, d initcmd.Defaults) *askSession {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", "-b", "main", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})
	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 50, Cols: 120}); err != nil {
		t.Fatal(err)
	}
	s := &askSession{t: t, in: ptmx, out: &syncBuffer{}, done: make(chan askResult, 1)}
	// 質問は、vanda init と同じく端末に出す（端末でない出力では、質問の大きさが決まらず描かれない）
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				_, _ = s.out.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	term := initcmd.NewTerminal(tty, tty)
	go func() {
		a, err := initcmd.Ask(root, d, term)
		s.done <- askResult{a, err}
	}()
	return s
}

// answer は、題 title の質問が出るまで待ち、keys を送る。決まった時間だけ待たず、題が出たかを見に行く
func (s *askSession) answer(title, keys string) {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out := s.out.String()
		if i := strings.Index(out[s.pos:], title); i >= 0 {
			s.pos += i + len(title)
			break
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("質問 %q が 5 秒たっても出ない\n--- 出力\n%s", title, out)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := s.in.Write([]byte(keys)); err != nil {
		s.t.Fatalf("キーを送れない: %v", err)
	}
}

func (s *askSession) result() askResult {
	s.t.Helper()
	select {
	case r := <-s.done:
		return r
	case <-time.After(5 * time.Second):
		s.t.Fatalf("Ask が 5 秒たっても終わらない\n--- 出力\n%s", s.out.String())
		return askResult{}
	}
}

const (
	enter = "\r"
	down  = "\x1b[B"
)

// ID の書き方の入力欄に書いたものが、そのまま答えになれば、入力欄は空から始まっている（既定の答えがあれば、その後ろに書き足される）
//
// @vanda:evidence init-no-store-id-pattern
func TestAskはストーリーストアにカスタムアダプタか使わないを選ぶとIDの書き方の質問を空の入力欄から始める(t *testing.T) {
	const typed = "ABC-[0-9]+"
	cases := []struct {
		choice string
		keys   string // ストーリーストアの質問で送るキー（既定は GitHub Issues）
		store  string
		empty  bool
	}{
		{"カスタムアダプタ", down + enter, "user", true},
		{"使わない", down + down + enter, "none", true},
		{"GitHub Issues", enter, "github", false},
	}
	for _, c := range cases {
		t.Run(c.choice, func(t *testing.T) {
			s := startAsk(t, initcmd.Defaults{Base: "main", Store: "github", GitHubRepo: "o/r"})
			s.answer("ベースブランチ", enter)
			s.answer("ストーリーストア", c.keys)
			switch c.choice {
			case "カスタムアダプタ":
				s.answer("アダプタの名前", "jira"+enter)
				s.answer("アダプタに渡す引数", enter)
			case "GitHub Issues":
				s.answer("GitHub のリポジトリ", enter)
			}
			s.answer("ストーリーの ID の書き方", typed+enter)
			s.answer("アノテーションを読む言語", enter)
			r := s.result()
			if r.err != nil {
				t.Fatalf("Ask が誤りを返した: %v\n--- 出力\n%s", r.err, s.out.String())
			}
			if r.a.Store != c.store {
				t.Fatalf("選んだストーリーストアが %q（%q のはず）", r.a.Store, c.store)
			}
			if got := r.a.IDPattern == typed; got != c.empty {
				t.Errorf("ID の書き方の答え %q（入力欄が空から始まるのは %v のはず）", r.a.IDPattern, c.empty)
			}
		})
	}
}

// @vanda:evidence init-write-failure-cleans
func TestWriteはファイルを書く途中で失敗したら作りかけの_vandaを消して誤りを返す(t *testing.T) {
	cases := []struct {
		name   string
		files  [][2]string
		failed string // 書けないファイル
	}{
		// .vanda/ と同じ場所にファイルは書けない（ディレクトリ）
		{"最初のファイルで", [][2]string{{".vanda", "x"}, {".vanda/config.toml", "version = \"0.1.0\"\n"}}, ".vanda"},
		// 前に書いたファイルの下には書けない
		{"途中のファイルで", [][2]string{{".vanda/config.toml", "version = \"0.1.0\"\n"}, {".vanda/config.toml/x", "y"}, {".vanda/promises.toml", ""}}, ".vanda/config.toml/x"},
		{"最後のファイルで", [][2]string{{".vanda/config.toml", "version = \"0.1.0\"\n"}, {".vanda/promises.toml", ""}, {".vanda/promises.toml/x", "y"}}, ".vanda/promises.toml/x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("前からある\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := initcmd.Write(root, c.files)
			if err == nil || !strings.Contains(err.Error(), c.failed) {
				t.Errorf("書けなかった %s を示す誤りを返さない: %v", c.failed, err)
			}
			if _, serr := os.Lstat(filepath.Join(root, ".vanda")); !os.IsNotExist(serr) {
				t.Errorf("作りかけの .vanda/ が残った: %v", serr)
			}
			if b, rerr := os.ReadFile(filepath.Join(root, "README.md")); rerr != nil || string(b) != "前からある\n" {
				t.Errorf(".vanda/ の外のファイルが変わった: %q %v", b, rerr)
			}
		})
	}
}

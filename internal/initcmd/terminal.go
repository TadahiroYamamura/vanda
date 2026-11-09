package initcmd

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/x/term"
)

// Terminal は、質問の間の端末。標準入力は 1 つの goroutine で読み続け、今出している質問のプログラムにだけ渡す。
//
// huh の質問は、質問ごとに bubbletea のプログラムを作って動かす。プログラムに標準入力そのものを渡すと、
// 終わったプログラムの入力を読む部分が残って次の質問への答えを持っていくことがあり、
// Expect のように質問のプロンプトが出てから答えを送っても、2 問目より後の答えが丸ごと消えて止まった。
// そのため、プログラムには、質問が終わったら読み取りを打ち切れる入力を渡す。
// 入力が端末のファイルでないと bubbletea は端末を raw にしないので、raw にするのと元に戻すのはこちらで行う。
// 出力の処理（改行を CR LF に直す）は raw にせずに残す
type Terminal struct {
	in     *os.File
	out    io.Writer
	chunks chan chunk

	mu   sync.Mutex
	rest []byte // 終わったプログラムに渡さなかった分。次のプログラムに渡す
	err  error  // 標準入力を読めなくなった理由
}

type chunk struct {
	b   []byte
	err error
}

// NewTerminal は、in（端末の標準入力）を読み始め、質問のプロンプトを out に出す Terminal を作る。
func NewTerminal(in *os.File, out io.Writer) *Terminal {
	t := &Terminal{in: in, out: out, chunks: make(chan chunk)}
	go t.pump()
	return t
}

func (t *Terminal) pump() {
	for {
		buf := make([]byte, 4096)
		n, err := t.in.Read(buf)
		if n > 0 {
			t.chunks <- chunk{b: buf[:n]}
		}
		if err != nil {
			t.chunks <- chunk{err: err}
			return
		}
	}
}

// session は、1 つの質問のプログラムに渡す入力。done を閉じた後は、何も読まずに io.EOF を返す
type session struct {
	t    *Terminal
	done chan struct{}
}

func (s *session) Read(b []byte) (int, error) {
	t := s.t
	t.mu.Lock()
	if len(t.rest) > 0 {
		n := copy(b, t.rest)
		t.rest = t.rest[n:]
		t.mu.Unlock()
		return n, nil
	}
	if t.err != nil {
		err := t.err
		t.mu.Unlock()
		return 0, err
	}
	t.mu.Unlock()

	select {
	case <-s.done:
		return 0, io.EOF
	case c := <-t.chunks:
		t.mu.Lock()
		defer t.mu.Unlock()
		if c.err != nil {
			t.err = c.err
			return 0, c.err
		}
		select {
		case <-s.done:
			// 受け取ったときには質問が終わっていた。次の質問に渡す
			t.rest = append(t.rest, c.b...)
			return 0, io.EOF
		default:
		}
		n := copy(b, c.b)
		t.rest = append(t.rest, c.b[n:]...)
		return n, nil
	}
}

// with は、端末を raw にして、f に質問のプログラムへの入力を渡す。f が終わったら、入力を打ち切り、端末を元に戻す。
func (t *Terminal) with(f func(in io.Reader) error) error {
	state, err := term.MakeRaw(t.in.Fd())
	if err != nil {
		return fmt.Errorf("端末を、質問に答えられる状態にできません（%v）。", err)
	}
	// 入力が端末のファイルでないとき、bubbletea は端末が改行を CR LF に直す前提で出力するので、出力の処理は戻す
	if err := keepOutputProcessing(t.in.Fd()); err != nil {
		_ = term.Restore(t.in.Fd(), state)
		return fmt.Errorf("端末を、質問に答えられる状態にできません（%v）。", err)
	}
	s := &session{t: t, done: make(chan struct{})}
	runErr := f(s)
	close(s.done)
	if err := term.Restore(t.in.Fd(), state); err != nil {
		return fmt.Errorf("端末を元の状態に戻せませんでした（%v）。stty sane で戻してください。", err)
	}
	return runErr
}

// Package repo は、git のリポジトリのルートを探し、git のコマンドを呼ぶ。
package repo

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/diag"
)

// Root は、dir を含む git のリポジトリのルートを返す。
func Root(dir string) (string, *diag.Error) {
	out, err := Git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", &diag.Error{What: "git のリポジトリの外で動かしています（" + err.Error() + "）", Fix: "vanda は、.vanda/ のある git のリポジトリの中で動かしてください"}
	}
	return filepath.FromSlash(strings.TrimSpace(out)), nil
}

type gitError struct{ msg string }

func (e gitError) Error() string { return e.msg }

// Git は、dir で git を動かし、標準出力を返す。失敗したら、標準エラーの中身をエラーにする。
func Git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", gitError{"git " + strings.Join(args, " ") + ": " + msg}
	}
	return stdout.String(), nil
}

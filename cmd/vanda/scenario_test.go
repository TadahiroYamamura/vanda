package main_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// シナリオは、ビルドした vanda の実行ファイルを外から動かして確かめる。
// testscript.RunMain で main を直に呼ぶ形にしないのは、実装の内部の関数（main の入口の形）に頼らずに書くため。
func TestScenario(t *testing.T) {
	bin := buildVanda(t)
	binDir := filepath.Dir(bin)

	current, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	newer, older, err := neighborVersions(current)
	if err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "scenario"),
		Setup: func(env *testscript.Env) error {
			env.Setenv("PATH", binDir+string(os.PathListSeparator)+env.Getenv("PATH"))
			env.Setenv("VANDA_VERSION", current)
			env.Setenv("VANDA_VERSION_NEWER", newer)
			env.Setenv("VANDA_VERSION_OLDER", older)
			// 動かしているマシンの git の設定に結果が左右されないよう、system と global の設定を読ませない。
			env.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			// 作業のディレクトリの上にたまたま git のリポジトリがあっても、リポジトリの外として扱わせる。
			env.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(env.WorkDir))
			env.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			env.Setenv("GIT_AUTHOR_NAME", "テスト担当")
			env.Setenv("GIT_AUTHOR_EMAIL", "tester@example.com")
			env.Setenv("GIT_COMMITTER_NAME", "テスト担当")
			env.Setenv("GIT_COMMITTER_EMAIL", "tester@example.com")
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"expand": cmdExpand,
			"order":  cmdOrder,
			"agree":  cmdAgree,
		},
	})
}

// buildVanda は、vanda をビルドし、実行ファイルの場所を返す。テストごとにビルドするが、2 度目からは Go のビルドのキャッシュが効く。
func buildVanda(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vanda")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("vanda をビルドできない: %v\n%s", err, out)
	}
	return bin
}

var semverPattern = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

// `vanda version` の出力の形は計画に書かれていないので、出力の中の最初の版の形の文字列を、動いている vanda の版とみなす。
func runningVersion(bin string) (string, error) {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return "", fmt.Errorf("vanda version が失敗した: %w", err)
	}
	v := semverPattern.FindString(string(out))
	if v == "" {
		return "", fmt.Errorf("vanda version の出力に版が見つからない: %q", out)
	}
	return v, nil
}

// 動いている版より新しい版と古い版を作る。v を付けるかどうかは、動いている版の書き方に合わせる。
func neighborVersions(current string) (newer, older string, err error) {
	m := semverPattern.FindStringSubmatch(current)
	if m == nil {
		return "", "", fmt.Errorf("版の形ではない: %q", current)
	}
	prefix := ""
	if strings.HasPrefix(current, "v") {
		prefix = "v"
	}
	var n [3]int
	for i := range n {
		n[i], err = strconv.Atoi(m[i+1])
		if err != nil {
			return "", "", fmt.Errorf("版の数を読めない: %q: %w", current, err)
		}
	}
	newer = fmt.Sprintf("%s%d.0.0", prefix, n[0]+1)
	switch {
	case n[2] > 0:
		older = fmt.Sprintf("%s%d.%d.%d", prefix, n[0], n[1], n[2]-1)
	case n[1] > 0:
		older = fmt.Sprintf("%s%d.%d.0", prefix, n[0], n[1]-1)
	case n[0] > 0:
		older = fmt.Sprintf("%s%d.0.0", prefix, n[0]-1)
	default:
		return "", "", fmt.Errorf("動いている版 %q より古い版を作れない", current)
	}
	return newer, older, nil
}

// expand <ファイル>...
// ファイルの中の ${VANDA_VERSION} などを、環境の変数の値に置き換える。
// txtar のファイルの中では変数が展開されないため、動いている vanda の版を設定のファイルに書くのに使う。
func cmdExpand(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("expand は ! を付けて使えない")
	}
	if len(args) == 0 {
		ts.Fatalf("使い方: expand <ファイル>...")
	}
	for _, name := range args {
		path := ts.MkAbs(name)
		data, err := os.ReadFile(path)
		ts.Check(err)
		ts.Check(os.WriteFile(path, []byte(os.Expand(string(data), ts.Getenv)), 0o644))
	}
}

// order <前に出る正規表現> <後に出る正規表現>
// 直前のコマンドの標準出力（vanda check が審査の結果と誤りを出す所）で、前の正規表現に当たる所が、後の正規表現に当たる所より先に出ることを確かめる。
// エラーは最初の 1 件で止めずに全部を集めて出す決まりなので、両方が出ていることも求める。
func cmdOrder(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("order は ! を付けて使えない")
	}
	if len(args) != 2 {
		ts.Fatalf("使い方: order <前に出る正規表現> <後に出る正規表現>")
	}
	stdout := ts.ReadFile("stdout")
	first, err := regexp.Compile(args[0])
	ts.Check(err)
	second, err := regexp.Compile(args[1])
	ts.Check(err)
	fi := first.FindStringIndex(stdout)
	if fi == nil {
		ts.Fatalf("標準出力に %q が出ていない", args[0])
	}
	si := second.FindStringIndex(stdout)
	if si == nil {
		ts.Fatalf("標準出力に %q が出ていない", args[1])
	}
	if si[0] < fi[0] {
		ts.Fatalf("標準出力で %q が %q より先に出ている", args[1], args[0])
	}
}

// agree <合意の記録のファイル> <ストーリーの ID> <claim|term|feature> <ID> <指紋の元の JSON のファイル>
// agree <合意の記録のファイル> <ストーリーの ID> file <ファイルの場所>
// 合意の記録に 1 件を書き足す。
// 約束・用語・機能の指紋は、テストの中で手で書いた JSON（キーを名前の順に並べ、空白なし）の SHA-256 から作る。
// 約束リストの TOML を読んで JSON を作らないのは、読み込みを実装と同じ作りにすると、実装と同じ誤りに気づけないため。
func cmdAgree(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("agree は ! を付けて使えない")
	}
	if len(args) < 4 {
		ts.Fatalf("使い方: agree <合意の記録のファイル> <ストーリーの ID> <claim|term|feature|file> <ID かファイルの場所> [<JSON のファイル>]")
	}
	record, story, kind, key := args[0], args[1], args[2], args[3]
	var fingerprint, path string
	switch kind {
	case "claim", "term", "feature":
		if len(args) != 5 {
			ts.Fatalf("%s の合意には、指紋の元の JSON のファイルが要る", kind)
		}
		data, err := os.ReadFile(ts.MkAbs(args[4]))
		ts.Check(err)
		data = bytes.TrimRight(data, "\n")
		var compact bytes.Buffer
		if err := json.Compact(&compact, data); err != nil {
			ts.Fatalf("%s は JSON として読めない: %v", args[4], err)
		}
		if !bytes.Equal(compact.Bytes(), data) {
			ts.Fatalf("%s に空白がある。指紋の元は空白なしの JSON で書く", args[4])
		}
		fingerprint = sha256Hex(data)
	case "file":
		if len(args) != 4 {
			ts.Fatalf("file の合意には、ファイルの場所だけを渡す")
		}
		data, err := os.ReadFile(ts.MkAbs(key))
		ts.Check(err)
		path = key
		fingerprint = sha256Hex(data)
		key = fingerprint
	default:
		ts.Fatalf("対象の種類 %q は claim・term・feature・file のどれでもない", kind)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n[[agreement]]\nkind = %q\nkey = %q\nfingerprint = %q\n", kind, key, fingerprint)
	if path != "" {
		fmt.Fprintf(&b, "path = %q\n", path)
	}
	fmt.Fprintf(&b, "agreed_at = 2026-10-31T12:00:00+09:00\nstory = %q\n", story)
	fmt.Fprintf(&b, "\n[agreement.user]\nname = %q\nemail = %q\n", ts.Getenv("GIT_AUTHOR_NAME"), ts.Getenv("GIT_AUTHOR_EMAIL"))

	abs := ts.MkAbs(record)
	ts.Check(os.MkdirAll(filepath.Dir(abs), 0o755))
	f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	ts.Check(err)
	_, err = f.WriteString(b.String())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	ts.Check(err)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

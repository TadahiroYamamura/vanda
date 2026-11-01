// Package initcmd は、vanda init（CMD-11）。端末で問いかけ、答えで .vanda/ を作る。
package initcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/check"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/story"
	"github.com/TadahiroYamamura/vanda/internal/tomlwrite"
	"github.com/TadahiroYamamura/vanda/internal/version"
)

// ErrEOF は、問いかけの途中で入力が終わった（Ctrl+D）ため、何も作らなかったことを表す。
var ErrEOF = errors.New("入力が終わったため、何も作りませんでした。")

// ErrDeclined は、作るかの確かめに n と答えたため、何も作らなかったことを表す。
var ErrDeclined = errors.New("作らないと答えたため、何も作りませんでした。")

// Language は、アノテーションを読む言語。
type Language struct {
	Title   string // 問いかけに出す名前
	Suffix  string // 見つけ方（git が追っているファイルの名前の終わり）
	Pattern string // 既定のパターン
	Adapter string
}

// Languages は、vanda の中に持つアノテーションの読み取りのアダプタで扱う言語（docs/plans/init-impl.md の表の順）。
var Languages = []Language{
	{Title: "Go のテスト（*_test.go）", Suffix: "_test.go", Pattern: "**/*_test.go", Adapter: "go"},
	{Title: "シナリオ（*.txtar）", Suffix: ".txtar", Pattern: "**/*.txtar", Adapter: "txtar"},
	{Title: "TLA+（*.tla）", Suffix: ".tla", Pattern: "**/*.tla", Adapter: "tla"},
	{Title: "SQL（*.sql）", Suffix: ".sql", Pattern: "**/*.sql", Adapter: "sql"},
}

// Defaults は、リポジトリから読み取った、問いかけの既定の答え。
type Defaults struct {
	Base       string // 空なら既定なし
	GitHubRepo string // remote origin が GitHub のときの <owner>/<repo>。そうでなければ空
	Languages  []Language
}

// Detect は、リポジトリから既定の答えを読み取る。
func Detect(root string) (Defaults, error) {
	var d Defaults
	if out, err := repo.Git(root, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"); err == nil && strings.TrimSpace(out) != "" {
		d.Base = strings.TrimSpace(out)
	} else if out, err := repo.Git(root, "branch", "--show-current"); err == nil {
		d.Base = strings.TrimSpace(out)
	}
	if out, err := repo.Git(root, "remote", "get-url", "origin"); err == nil {
		d.GitHubRepo = GitHubRepository(strings.TrimSpace(out))
	}
	files, err := repo.Git(root, "ls-files", "-z")
	if err != nil {
		return d, fmt.Errorf("git が追っているファイルの一覧を読めません（%v）", err)
	}
	names := strings.Split(files, "\x00")
	for _, l := range Languages {
		for _, f := range names {
			if strings.HasSuffix(f, l.Suffix) {
				d.Languages = append(d.Languages, l)
				break
			}
		}
	}
	return d, nil
}

// GitHubRepository は、remote の URL が GitHub なら <owner>/<repo> を返す。https・ssh（scp の形と ssh:// の形）を読み、
// ssh の設定の別名のホスト（github.com_xxx など、github.com で始まるもの）も GitHub とみなす。GitHub でなければ空。
func GitHubRepository(url string) string {
	var host, path string
	switch {
	case strings.Contains(url, "://"):
		rest := url[strings.Index(url, "://")+3:]
		if i := strings.Index(rest, "@"); i >= 0 && i < strings.Index(rest+"/", "/") {
			rest = rest[i+1:]
		}
		host, path, _ = strings.Cut(rest, "/")
		host, _, _ = strings.Cut(host, ":")
	case strings.Contains(url, ":"):
		hostPart, p, _ := strings.Cut(url, ":")
		if i := strings.LastIndex(hostPart, "@"); i >= 0 {
			hostPart = hostPart[i+1:]
		}
		host, path = hostPart, p
	default:
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(host), "github.com") {
		return ""
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// Answers は、問いかけの答え。
type Answers struct {
	Base        string
	Store       string // github・command・none
	Repository  string
	Command     []string
	IDPattern   string
	Annotations []Language // Pattern は答えのもの
}

// prompter は、1 行ずつ読む問いかけ。
type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

// ask は、問いを出して 1 行を読む。空なら既定を返す。入力が終わったら ErrEOF を返す。
func (p *prompter) ask(question, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", question)
	}
	line, err := p.in.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		fmt.Fprintln(p.out)
		return "", ErrEOF
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// askValid は、check が誤りを返すあいだ、理由を出して同じ問いを聞き直す。
func (p *prompter) askValid(question, def string, check func(string) error) (string, error) {
	for {
		a, err := p.ask(question, def)
		if err != nil {
			return "", err
		}
		if err := check(a); err != nil {
			fmt.Fprintln(p.out, "  "+err.Error())
			continue
		}
		return a, nil
	}
}

func (p *prompter) yesNo(question string, def bool) (bool, error) {
	d := "n"
	if def {
		d = "y"
	}
	a, err := p.askValid(question+"（y/n）", d, func(s string) error {
		switch strings.ToLower(s) {
		case "y", "yes", "n", "no":
			return nil
		}
		return errors.New("y か n で答えてください。")
	})
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(strings.ToLower(a), "y"), nil
}

type storeChoice struct {
	key, title, defaultIDPattern string
}

// storeChoices は、ストーリーの置き場所の選択肢。vanda の中に持つアダプタの一覧から作り、外部のコマンドと使わないを足す
func storeChoices() []storeChoice {
	var out []storeChoice
	for _, a := range story.Builtins {
		out = append(out, storeChoice{key: a.Name, title: a.Title, defaultIDPattern: a.DefaultIDPattern})
	}
	return append(out, storeChoice{key: "command", title: "外部のコマンド"}, storeChoice{key: "none", title: "使わない"})
}

// Ask は、問いかけて答えを集める。
func Ask(root string, d Defaults, in *bufio.Reader, out io.Writer) (Answers, error) {
	p := &prompter{in: in, out: out}
	var a Answers
	var err error

	a.Base, err = p.askValid("ベースブランチ", d.Base, func(s string) error {
		if s == "" {
			return errors.New("ベースブランチを入れてください。")
		}
		if _, err := repo.Git(root, "rev-parse", "--verify", "-q", s+"^{commit}"); err != nil {
			return fmt.Errorf("%s というブランチ（コミット）が見つかりません。あるブランチの名前を入れてください。", s)
		}
		return nil
	})
	if err != nil {
		return a, err
	}

	choices := storeChoices()
	def := len(choices) // 使わない
	if d.GitHubRepo != "" {
		def = 1
	}
	fmt.Fprintln(out, "ストーリーの置き場所を、番号で選んでください。")
	for i, c := range choices {
		fmt.Fprintf(out, "  %d. %s\n", i+1, c.title)
	}
	n, err := p.askValid("番号", strconv.Itoa(def), func(s string) error {
		i, err := strconv.Atoi(s)
		if err != nil || i < 1 || i > len(choices) {
			return fmt.Errorf("1 から %d の番号を入れてください。", len(choices))
		}
		return nil
	})
	if err != nil {
		return a, err
	}
	i, _ := strconv.Atoi(n)
	choice := choices[i-1]
	a.Store = choice.key

	switch choice.key {
	case "github":
		a.Repository, err = p.askValid("GitHub のリポジトリ", d.GitHubRepo, func(s string) error {
			if GitHubRepository("https://github.com/"+s) != s || strings.ContainsAny(s, " \t") {
				return errors.New("<owner>/<repo> の形（例: TadahiroYamamura/vanda）で入れてください。")
			}
			return nil
		})
	case "command":
		var line string
		line, err = p.askValid("外部のコマンドと引数（空白で区切る）", "", func(s string) error {
			if len(strings.Fields(s)) == 0 {
				return errors.New("コマンドを入れてください。")
			}
			return nil
		})
		a.Command = strings.Fields(line)
	}
	if err != nil {
		return a, err
	}

	a.IDPattern, err = p.askValid("ストーリーの ID の書き方（正規表現）", choice.defaultIDPattern, func(s string) error {
		if s == "" {
			return errors.New("ストーリーの ID の書き方を入れてください（例: #[0-9]+）。")
		}
		re, err := regexp.Compile(s)
		if err != nil {
			return fmt.Errorf("正規表現として読めません（%v）。Go の正規表現（RE2）の書き方で入れてください。", err)
		}
		if re.MatchString("") {
			return errors.New("空の文字列に当たります。ストーリーの ID 全体に当たる正規表現を入れてください。")
		}
		return nil
	})
	if err != nil {
		return a, err
	}

	if len(d.Languages) == 0 {
		fmt.Fprintln(out, "アノテーションを読む言語のファイルは、見つかりませんでした。")
	}
	for _, l := range d.Languages {
		yes, err := p.yesNo(l.Title+"のアノテーションを読みますか", true)
		if err != nil {
			return a, err
		}
		if !yes {
			continue
		}
		pat, err := p.askValid("  読むファイルのパターン", l.Pattern, func(s string) error {
			if err := glob.Validate(s); err != nil {
				return errors.New("パターンの書き方が誤っています。リポジトリの直下からの / 区切りの場所で入れてください（例: " + l.Pattern + "）。")
			}
			return nil
		})
		if err != nil {
			return a, err
		}
		l.Pattern = pat
		a.Annotations = append(a.Annotations, l)
	}
	return a, nil
}

// Confirm は、作るかを聞く。
func Confirm(in *bufio.Reader, out io.Writer) (bool, error) {
	p := &prompter{in: in, out: out}
	return p.yesNo("作りますか", true)
}

// ConfigText は、.vanda/config.toml の中身。値だけを書き、コメントは入れない。
func ConfigText(a Answers) string {
	var b strings.Builder
	fmt.Fprintf(&b, "version = %s\n", tomlwrite.String(version.Current))
	for _, l := range a.Annotations {
		fmt.Fprintf(&b, "\n[[annotation]]\npaths = [%s]\nadapter = %s\n", tomlwrite.String(l.Pattern), tomlwrite.String(l.Adapter))
	}
	b.WriteString("\n[story]\n")
	switch a.Store {
	case "command":
		parts := make([]string, len(a.Command))
		for i, c := range a.Command {
			parts[i] = tomlwrite.String(c)
		}
		fmt.Fprintf(&b, "command = [%s]\n", strings.Join(parts, ", "))
	case "none":
	default:
		fmt.Fprintf(&b, "adapter = %s\nrepository = %s\n", tomlwrite.String(a.Store), tomlwrite.String(a.Repository))
	}
	fmt.Fprintf(&b, "id_pattern = %s\n", tomlwrite.String(a.IDPattern))
	fmt.Fprintf(&b, "\n[gui]\nbase = %s\n", tomlwrite.String(a.Base))
	return b.String()
}

// ChecksText は、.vanda/checks/ci.toml の中身。今ある審査を全部、審査の種類の順に並べる。
// 1 つのストーリーで変わったものを見られる審査は、範囲を story にする
func ChecksText() string {
	var b strings.Builder
	for i, n := range check.Names {
		scope := check.ScopeAll
		for _, s := range n.Scopes {
			if s == check.ScopeStory {
				scope = check.ScopeStory
			}
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[[check]]\nname = %s\nscope = %s\n", tomlwrite.String(n.Name), tomlwrite.String(scope))
	}
	return b.String()
}

// Files は、作るファイルの場所（リポジトリの直下から）と中身。
func Files(a Answers) [][2]string {
	return [][2]string{
		{config.File, ConfigText(a)},
		{config.DefaultPromises, ""},
		{config.DefaultAgreements, ""},
		{".vanda/checks/ci.toml", ChecksText()},
	}
}

// Write は、.vanda/ を作り、ファイルを書く。途中で失敗したら、作った .vanda/ を消す。
func Write(root string, files [][2]string) error {
	dir := filepath.Join(root, ".vanda")
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf(".vanda/ を作れません（%v）。", err)
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f[0]))
		err := os.MkdirAll(filepath.Dir(p), 0o755)
		if err == nil {
			err = os.WriteFile(p, []byte(f[1]), 0o644)
		}
		if err != nil {
			if rerr := os.RemoveAll(dir); rerr != nil {
				return fmt.Errorf("%s を書けず（%v）、作りかけの .vanda/ を消すこともできませんでした（%v）。.vanda/ を手で消してください。", f[0], err, rerr)
			}
			return fmt.Errorf("%s を書けませんでした（%v）。作りかけの .vanda/ は消しました。", f[0], err)
		}
	}
	return nil
}

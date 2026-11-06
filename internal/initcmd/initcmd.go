// Package initcmd は、vanda init（CMD-11）。端末で問いかけ、答えで .vanda/ を作る。
package initcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/annotation"
	"github.com/TadahiroYamamura/vanda/internal/check"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/story"
	"github.com/TadahiroYamamura/vanda/internal/tomlwrite"
	"github.com/TadahiroYamamura/vanda/internal/version"

	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"
)

// ErrDeclined は、作るかの確かめに n と答えたため、何も作らなかったことを表す。
var ErrDeclined = errors.New("作らないと答えたため、何も作りませんでした。")

// Language は、アノテーションを読む言語（組込みのアダプタ）と、読むファイルのパターン。
// Detect が返すものでは Patterns はアダプタの既定、Answers の中では答えのパターン（書いた順）。
type Language struct {
	annotation.AdapterInfo
	Patterns []string
}

// Defaults は、リポジトリから読み取った、問いかけの既定の答え。
type Defaults struct {
	// Base は、ベースブランチ。origin/HEAD が指すブランチ（例: origin/main）、なければ今のブランチの名前。
	// コミットのないリポジトリでも、今のブランチの名前（例: main）が入る。どちらも読めなければ空
	Base string
	// Store は、ストーリーの置き場所。remote origin が GitHub なら "github"、そうでなければ（origin がないときも）"none"
	Store string
	// GitHubRepo は、remote origin が GitHub のときの <owner>/<repo>。そうでなければ空
	GitHubRepo string
	// Languages は、git が追っているファイルに見つかった言語。annotation.Builtins の順で、Pattern はアダプタの既定。
	// 見つからなければ空
	Languages []Language
}

// Detect は、root（git のリポジトリの直下のディレクトリ）から、問いかけの既定の答えを読み取る。
// 問いかけはしない。git が追っているファイルの一覧を読めないときだけ、エラーを返す。
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
	d.Store = "none"
	if d.GitHubRepo != "" {
		d.Store = "github"
	}
	files, err := repo.Git(root, "ls-files", "-z")
	if err != nil {
		return d, fmt.Errorf("git が追っているファイルの一覧を読めません（%v）", err)
	}
	names := strings.Split(files, "\x00")
	for _, l := range annotation.Builtins {
	found:
		for _, f := range names {
			for _, suffix := range l.Suffixes {
				if strings.HasSuffix(f, suffix) {
					d.Languages = append(d.Languages, Language{AdapterInfo: l, Patterns: l.DefaultPatterns})
					break found
				}
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

// Answers は、問いかけの答え。Files に渡すと、作るファイルの中身になる。
type Answers struct {
	// Base は、ベースブランチ（config.toml の [gui] base）
	Base string
	// Store は、ストーリーの置き場所。"github"（GitHub Issues）・"user"（カスタムアダプタ）・"none"（使わない）
	Store string
	// Repository は、Store が "github" のときの <owner>/<repo>。ほかの置き場所では使わない
	Repository string
	// Adapter は、Store が "user" のときの、アダプタの名前（.vanda/adapters/ の下に置く名前）。ほかの置き場所では使わない
	Adapter string
	// Args は、Store が "user" のときの、アダプタに渡す引数。空なら args を書かない。ほかの置き場所では使わない
	Args []string
	// IDPattern は、ストーリーの ID の書き方（Go の正規表現）。置き場所によらず書く
	IDPattern string
	// Annotations は、選んだ言語と、言語ごとの読むファイルのパターン。並べた順に [[annotation]] を書く。
	// 空なら [[annotation]] を書かない
	Annotations []Language
}

// ErrInterrupted は、問いかけの途中で中断した（Ctrl+C・Ctrl+D）ため、何も作らなかったことを表す。
var ErrInterrupted = errors.New("中断したため、何も作りませんでした。")

type storeChoice struct {
	key, title, defaultIDPattern string
}

// storeChoices は、ストーリーの置き場所の選択肢。組込みのアダプタの一覧から作り、カスタムアダプタと使わないを足す
func storeChoices() []storeChoice {
	var out []storeChoice
	for _, a := range story.Builtins {
		out = append(out, storeChoice{key: a.Name, title: a.Title, defaultIDPattern: a.DefaultIDPattern})
	}
	return append(out, storeChoice{key: "user", title: "カスタムアダプタ"}, storeChoice{key: "none", title: "使わない"})
}

// asker は、huh の問いを 1 つずつ出す。問いの答えで次の問いが変わるので、まとめた 1 つの形でなく、問いごとに出す
type asker struct {
	t    *Terminal
	keys *huh.KeyMap
}

func newAsker(t *Terminal) *asker {
	keys := huh.NewDefaultKeyMap()
	// Ctrl+D（入力を終える）でも、Ctrl+C と同じく中断する
	keys.Quit = key.NewBinding(key.WithKeys("ctrl+c", "ctrl+d"))
	return &asker{t: t, keys: keys}
}

func (a *asker) run(fields ...huh.Field) error {
	err := a.t.with(func(in io.Reader) error {
		return huh.NewForm(huh.NewGroup(fields...)).WithInput(in).WithOutput(a.t.out).WithKeyMap(a.keys).WithShowHelp(true).Run()
	})
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrInterrupted
	}
	if err != nil {
		return fmt.Errorf("問いかけを出せませんでした（%v）。", err)
	}
	return nil
}

// input は、既定の答えを入れた状態から始める入力。check は、形に合わない答えのときに、その場で理由を出す
func (a *asker) input(title, def string, check func(string) error) (string, error) {
	v := def
	err := a.run(huh.NewInput().Title(title).Value(&v).Validate(func(s string) error { return check(strings.TrimSpace(s)) }))
	return strings.TrimSpace(v), err
}

// Ask は、d を既定の答えにして、t で問いかけ、答えを集める。
func Ask(root string, d Defaults, t *Terminal) (Answers, error) {
	q := newAsker(t)
	var a Answers
	var err error

	current := ""
	if b, err := repo.Git(root, "branch", "--show-current"); err == nil {
		current = strings.TrimSpace(b)
	}
	a.Base, err = q.input("ベースブランチ", d.Base, func(s string) error {
		if s == "" {
			return errors.New("ベースブランチを入れてください。")
		}
		// 初期化した直後でコミットがないリポジトリでは、今のブランチはまだ git の中にないので、名前が同じなら受け付ける
		if s == current {
			return nil
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
	choice := choices[len(choices)-1] // 使わない
	for _, c := range choices {
		if c.key == d.Store {
			choice = c
		}
	}
	var opts []huh.Option[storeChoice]
	for _, c := range choices {
		opts = append(opts, huh.NewOption(c.title, c))
	}
	if err := q.run(huh.NewSelect[storeChoice]().Title("ストーリーの置き場所").Options(opts...).Value(&choice)); err != nil {
		return a, err
	}
	a.Store = choice.key

	switch choice.key {
	case "github":
		a.Repository, err = q.input("GitHub のリポジトリ", d.GitHubRepo, func(s string) error {
			if GitHubRepository("https://github.com/"+s) != s || strings.ContainsAny(s, " \t") {
				return errors.New("<owner>/<repo> の形（例: TadahiroYamamura/vanda）で入れてください。")
			}
			return nil
		})
	case "user":
		a.Adapter, err = q.input("アダプタの名前（.vanda/adapters/ の下に置く名前）", "", func(s string) error {
			if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\ \t") {
				return errors.New("アダプタの名前を入れてください（空白・/・\\ を含まない名前。. と .. は使えません）。")
			}
			return nil
		})
		if err != nil {
			return a, err
		}
		var line string
		line, err = q.input("アダプタに渡す引数（空白で区切る。なくてよい）", "", func(string) error { return nil })
		a.Args = strings.Fields(line)
	}
	if err != nil {
		return a, err
	}

	a.IDPattern, err = q.input("ストーリーの ID の書き方（正規表現）", choice.defaultIDPattern, func(s string) error {
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

	// 読む言語は、組込みのアダプタを全部並べて選ばせる（初期化した直後でファイルがないリポジトリでも選べるように）。
	// リポジトリで見つかった言語は、選んだ状態から始める
	found := map[string]bool{}
	for _, l := range d.Languages {
		found[l.Name] = true
	}
	var langOpts []huh.Option[string]
	for _, l := range annotation.Builtins {
		langOpts = append(langOpts, huh.NewOption(l.Title, l.Name).Selected(found[l.Name]))
	}
	var picked []string
	if err := q.run(huh.NewMultiSelect[string]().Title("アノテーションを読む言語（スペースで選ぶ）").Options(langOpts...).Value(&picked)); err != nil {
		return a, err
	}
	for _, l := range annotation.Builtins {
		if !contains(picked, l.Name) {
			continue
		}
		example := strings.Join(l.DefaultPatterns, " ")
		line, err := q.input(l.Title+"の、アノテーションを読むファイル（空白で区切って複数書ける）", example, func(s string) error {
			pats := strings.Fields(s)
			bad := len(pats) == 0
			for _, p := range pats {
				bad = bad || glob.Validate(p) != nil
			}
			if bad {
				return errors.New("パターンの書き方が誤っています。リポジトリの直下からの / 区切りの場所で、複数なら空白で区切って入れてください（例: " + example + "）。")
			}
			return nil
		})
		if err != nil {
			return a, err
		}
		a.Annotations = append(a.Annotations, Language{AdapterInfo: l, Patterns: strings.Fields(line)})
	}
	return a, nil
}

// Confirm は、t で、作るかを聞く。
func Confirm(t *Terminal) (bool, error) {
	q := newAsker(t)
	yes := true
	err := q.run(huh.NewConfirm().Title("この内容で .vanda/ を作りますか").Affirmative("作る").Negative("作らない").Value(&yes))
	return yes, err
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ConfigText は、.vanda/config.toml の中身。値だけを書き、コメントは入れない。
func ConfigText(a Answers) string {
	var b strings.Builder
	fmt.Fprintf(&b, "version = %s\n", tomlwrite.String(version.Current))
	for _, l := range a.Annotations {
		types := make([]string, len(l.DefaultTypes))
		for i, t := range l.DefaultTypes {
			types[i] = tomlwrite.String(t)
		}
		paths := make([]string, len(l.Patterns))
		for i, p := range l.Patterns {
			paths[i] = tomlwrite.String(p)
		}
		fmt.Fprintf(&b, "\n[[annotation]]\npaths = [%s]\nadapter = %s\ntypes = [%s]\n", strings.Join(paths, ", "), tomlwrite.String(l.Name), strings.Join(types, ", "))
	}
	b.WriteString("\n[story]\n")
	switch a.Store {
	case "user":
		fmt.Fprintf(&b, "adapter = %s\n", tomlwrite.String(a.Adapter))
		if len(a.Args) > 0 {
			parts := make([]string, len(a.Args))
			for i, c := range a.Args {
				parts[i] = tomlwrite.String(c)
			}
			fmt.Fprintf(&b, "args = [%s]\n", strings.Join(parts, ", "))
		}
	case "none":
	default:
		fmt.Fprintf(&b, "adapter = %s\nrepository = %s\n", tomlwrite.String(a.Store), tomlwrite.String(a.Repository))
	}
	fmt.Fprintf(&b, "id_pattern = %s\n", tomlwrite.String(a.IDPattern))
	fmt.Fprintf(&b, "\n[gui]\nbase = %s\n", tomlwrite.String(a.Base))
	return b.String()
}

// ChecksText は、.vanda/checks/ci.toml（ブランチで動かす審査の設定）の中身。今ある審査を全部、審査の種類の順に並べる。
// 1 つのストーリーで変わったものを見られる審査は、範囲を story にする
func ChecksText() string {
	return checksText(true)
}

// MainChecksText は、.vanda/checks/main.toml（ベースブランチで動かす審査の設定）の中身。
// 範囲 story だけで行える審査（story-origin）を除き、ほかの審査を全部、範囲 all で並べる。
// ベースブランチへの push のように Story: のトレーラーのないコミットでは、範囲 story の審査が必ず止まるため
func MainChecksText() string {
	return checksText(false)
}

func checksText(story bool) string {
	var b strings.Builder
	for _, n := range check.Names {
		scope := ""
		for _, s := range n.Scopes {
			if s == check.ScopeAll && scope == "" {
				scope = check.ScopeAll
			}
			if s == check.ScopeStory && story {
				scope = check.ScopeStory
			}
		}
		if scope == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[[check]]\nname = %s\nscope = %s\n", tomlwrite.String(n.Name), tomlwrite.String(scope))
	}
	return b.String()
}

// CIChecksFile と MainChecksFile は、vanda init が作る審査の設定の場所。CIChecksFile はブランチで、MainChecksFile はベースブランチで動かす
const (
	CIChecksFile   = ".vanda/checks/ci.toml"
	MainChecksFile = ".vanda/checks/main.toml"
)

// Files は、答え a から、作る 5 つのファイルを作る。問いかけも、ファイルの書き込みもしない。
// 戻り値の要素は {場所, 中身}。場所はリポジトリの直下からの / 区切り（例: .vanda/config.toml）。
// 順は .vanda/config.toml・.vanda/promises.toml・.vanda/agreements.toml・.vanda/checks/ci.toml・.vanda/checks/main.toml で、
// promises.toml と agreements.toml の中身は空。
func Files(a Answers) [][2]string {
	return [][2]string{
		{config.File, ConfigText(a)},
		{config.DefaultPromises, ""},
		{config.DefaultAgreements, ""},
		{CIChecksFile, ChecksText()},
		{MainChecksFile, MainChecksText()},
	}
}

// Write は、.vanda/ を作り、ファイルを書く。途中で失敗したら、作った .vanda/ を消す。書き終えたら、.vanda/ の中が書いたものとちょうど同じかを確かめる。
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
	if Hooks.AfterWrite != nil {
		Hooks.AfterWrite(root)
	}
	return checkCreated(root, files)
}

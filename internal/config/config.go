// Package config は、.vanda/config.toml を読む。
package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
	"github.com/TadahiroYamamura/vanda/internal/version"
)

const File = ".vanda/config.toml"

const (
	DefaultPromises   = ".vanda/promises.toml"
	DefaultAgreements = ".vanda/agreements.toml"
	DefaultIDPattern  = "#[0-9]+"
)

// 組込みのアノテーションの読み取りのアダプタ
var BuiltinAdapters = []string{"go", "txtar", "tla", "sql"}

// EvidenceKinds は、[[annotation]] の types に書けるエビデンスのタイプ（書ける順）と、そのタイプのエビデンスとして
// 揃っているとするアノテーションの種類。human-review・monitoring はアノテーションで確かめず、proof はまだ扱えないので、書けない
var EvidenceKinds = []struct{ Type, Kind string }{
	{"model-check", "claim"},
	{"cross-check", "evidence"},
	{"random-check", "evidence"},
	{"scenario-test", "evidence"},
	{"unit-test", "evidence"},
	{"interface-test", "evidence"},
	{"concurrency-test", "evidence"},
	{"guardrail", "evidence"},
	{"sensor", "sensor"},
}

// EvidenceKind は、types に書けるタイプ typ のエビデンスとして揃っているとするアノテーションの種類を返す。書けないタイプなら空。
func EvidenceKind(typ string) string {
	for _, e := range EvidenceKinds {
		if e.Type == typ {
			return e.Kind
		}
	}
	return ""
}

type Config struct {
	Version    string
	Promises   []string
	Agreements []string
	// 設定に promises・agreements を書かず、既定のファイルを使うか。既定のファイルは、なくても空とみなす
	PromisesDefault   bool
	AgreementsDefault bool
	Layers            []Layer
	Annotations       []AnnotationRule
	Guardrails        []GuardrailRule
	IDPattern         *regexp.Regexp
	// IDFull は、ストーリーの ID 全体に当たるかを確かめる正規表現（id_pattern の前後を固定したもの）
	IDFull *regexp.Regexp
	Story  StoryStore
	// GUIBase は、画面を開いたときのベースブランチ。書いていなければ空
	GUIBase string
}

// StoryStore は、ストーリーの置き場所のアダプタ。Adapter が空で User が nil なら、置き場所につながない。
type StoryStore struct {
	// Adapter は、組込みのアダプタの名前（github）。.vanda/adapters/ のアダプタを使うなら空
	Adapter    string
	Repository string
	APIURL     string
	User       *UserAdapter
}

// Name は、設定の [story] の adapter に書いた名前。置き場所につながないなら空。
func (s StoryStore) Name() string {
	if s.User != nil {
		return s.User.Name
	}
	return s.Adapter
}

// AdaptersDir は、組込みのほかのアダプタを置くディレクトリ（リポジトリの直下から）
const AdaptersDir = ".vanda/adapters"

// UserAdapter は、.vanda/adapters/ の直下に置いたアダプタ。
type UserAdapter struct {
	Name string
	// Path は、実行するファイルの、リポジトリの直下からの場所。ファイルならそのもの、ディレクトリなら直下の run
	Path string
	Args []string
}

const DefaultGitHubAPI = "https://api.github.com"

type Layer struct {
	ID          string
	Name        string
	Description string
}

// AnnotationRule は、読むファイルのパターンと、使うアダプタ。Adapter が空で User が nil なら、アダプタが決まっていない。
type AnnotationRule struct {
	Paths []string
	// Adapter は、組込みのアダプタの名前（go・txtar・tla・sql）。.vanda/adapters/ のアダプタを使うなら空
	Adapter string
	User    *UserAdapter
	// Types は、このパターンで読んだアノテーションが満たすエビデンスのタイプ
	Types []string
	Line  int
}

type GuardrailRule struct {
	Adapter string
	Paths   []string
}

// IsID は、ID が英小文字で始まり、英小文字・数字をハイフンでつないだ形かを返す。
func IsID(s string) bool {
	if s == "" || !(s[0] >= 'a' && s[0] <= 'z') || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// Load は、root の下の .vanda/config.toml を読み、確かめる。エラーがあれば、すべてを集めて返す。
func Load(root string) (*Config, diag.Errors) {
	cfg, _, errs := load(&loader{root: root})
	return cfg, errs
}

// LoadAllowingMissingAdapters は、Load と同じく読み、確かめる。ただし、名前のアダプタが組込みにも .vanda/adapters/ にもないことは
// 誤りにせず、その名前を missing に返す（.vanda/ を作った直後で、アダプタをまだ置いていないときに使う）。
func LoadAllowingMissingAdapters(root string) (cfg *Config, missing []string, errs diag.Errors) {
	return load(&loader{root: root, allowMissing: true})
}

type loader struct {
	root         string
	allowMissing bool
	missing      []string
}

func load(l *loader) (*Config, []string, diag.Errors) {
	var errs []diag.Error
	doc := tomlread.Load(l.root, File, &errs)
	if doc == nil {
		return nil, nil, errs
	}
	cfg := parse(l, doc.Top())
	if len(errs) > 0 {
		diag.Sort(errs)
		return nil, nil, errs
	}
	return cfg, l.missing, nil
}

func parse(l *loader, top *tomlread.Table) *Config {
	cfg := &Config{}
	if v, ok := top.Str("version", true); ok {
		cfg.Version = v
		checkVersion(top, v)
	}
	cfg.PromisesDefault = !top.Has("promises")
	cfg.AgreementsDefault = !top.Has("agreements")
	cfg.Promises = patterns(top, "promises", DefaultPromises)
	cfg.Agreements = patterns(top, "agreements", DefaultAgreements)

	layers, _ := top.Tables("layer", false)
	seen := map[string]bool{}
	for _, t := range layers {
		var l Layer
		l.ID, _ = t.Str("id", true)
		if l.ID != "" {
			t.Label = `layer "` + l.ID + `"`
			if !IsID(l.ID) {
				t.ErrAt("id", "レイヤーの ID "+quote(l.ID)+" が決まった形ではありません", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: model、db）にしてください")
			} else if seen[l.ID] {
				t.ErrAt("id", "レイヤーの ID "+quote(l.ID)+" がほかのレイヤーと重なっています", "別の ID にしてください")
			}
			seen[l.ID] = true
		}
		l.Name, _ = t.Str("name", true)
		l.Description, _ = t.Str("description", true)
		t.Unknown("id, name, description")
		cfg.Layers = append(cfg.Layers, l)
	}

	rules, _ := top.Tables("annotation", false)
	for _, t := range rules {
		r := AnnotationRule{Line: t.Line("")}
		r.Paths = patterns(t, "paths", "")
		r.Adapter, r.User = adapterOf(l, t, BuiltinAdapters, "adapter を "+strings.Join(BuiltinAdapters, "・")+" のどれかにするか")
		r.Types = evidenceTypes(t)
		t.Unknown("paths, adapter, args, types")
		cfg.Annotations = append(cfg.Annotations, r)
	}

	guards, _ := top.Tables("guardrail", false)
	for _, t := range guards {
		var g GuardrailRule
		if a, ok := t.Str("adapter", true); ok {
			if a != "file" {
				t.ErrAt("adapter", "ガードレールの定義の取り出しのアダプタ "+quote(a)+" は組込みにありません", `adapter = "file" にしてください（今は、ファイルを読むだけのアダプタしかありません）`)
			}
			g.Adapter = a
		}
		g.Paths = patterns(t, "paths", "")
		t.Unknown("adapter, paths")
		cfg.Guardrails = append(cfg.Guardrails, g)
	}

	if re, err := regexp.Compile(DefaultIDPattern); err != nil {
		top.ErrAt("", "vanda の中の、ストーリーの ID の既定の書き方を読めません（"+err.Error()+"）", "vanda の不具合です。開発者に知らせてください")
	} else {
		cfg.IDPattern = re
	}
	if story, ok := top.Table("story", false); ok {
		if p, ok := story.Str("id_pattern", false); ok {
			re, err := regexp.Compile(p)
			if err != nil {
				story.ErrAt("id_pattern", "id_pattern を正規表現として読めません（"+err.Error()+"）", "Go の正規表現（RE2）の書き方で直してください")
			} else if re.MatchString("") {
				story.ErrAt("id_pattern", "id_pattern が空の文字列に当たります", "ストーリーの ID 全体に当たる正規表現（例: #[0-9]+）にしてください")
			} else {
				cfg.IDPattern = re
			}
		}
		parseStoryStore(l, story, &cfg.Story)
		story.Unknown("id_pattern, adapter, args, repository, api_url")
	}
	if gui, ok := top.Table("gui", false); ok {
		cfg.GUIBase, _ = gui.Str("base", false)
		gui.Unknown("base")
	}

	if cfg.IDPattern != nil {
		if re, err := regexp.Compile(`^(?:` + cfg.IDPattern.String() + `)$`); err != nil {
			top.ErrAt("", "ストーリーの ID の書き方を、ID 全体に当たる形にできません（"+err.Error()+"）", "id_pattern を直してください")
		} else {
			cfg.IDFull = re
		}
	}
	top.Unknown("version, promises, agreements, layer, annotation, guardrail, story, gui")
	return cfg
}

// isRepository は、<owner>/<repo> の形（英数字と _ . - の名前を / で 1 つつないだもの）かを返す。
func isRepository(s string) bool {
	owner, name, ok := strings.Cut(s, "/")
	valid := func(p string) bool {
		if p == "" {
			return false
		}
		for _, c := range p {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-') {
				return false
			}
		}
		return true
	}
	return ok && valid(owner) && valid(name)
}

// evidenceTypes は、[[annotation]] の types を読む。
func evidenceTypes(t *tomlread.Table) []string {
	types, ok := t.Strs("types", true)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for _, typ := range types {
		switch {
		case EvidenceKind(typ) == "":
			names := make([]string, len(EvidenceKinds))
			for i, e := range EvidenceKinds {
				names[i] = e.Type
			}
			why := "決まったタイプではありません"
			switch typ {
			case "human-review", "monitoring":
				why = "アノテーションで確かめないタイプなので、書けません"
			case "proof":
				why = "まだ扱えないタイプなので、書けません"
			}
			t.ErrAt("types", "types の "+quote(typ)+" は、"+why, "types には "+strings.Join(names, "・")+" のどれかを書いてください")
		case seen[typ]:
			t.ErrAt("types", "types に "+quote(typ)+" が 2 つ以上あります", "1 つにしてください")
		}
		seen[typ] = true
	}
	return types
}

// adapterOf は、表の adapter と args を読み、使うアダプタを決める。.vanda/adapters/ に同じ名前のアダプタがあれば、
// 組込みのアダプタ（builtins）より先に使う。fix は、名前のアダプタがどこにもないときの直し方の前半。
func adapterOf(l *loader, t *tomlread.Table, builtins []string, fix string) (string, *UserAdapter) {
	name, hasAdapter := t.Str("adapter", false)
	args, hasArgs := t.Strs("args", false)
	if !hasAdapter {
		if hasArgs {
			t.ErrAt("args", "args は、.vanda/adapters/ のアダプタのときだけ書けます", "adapter に .vanda/adapters/ のアダプタの名前を書くか、args を消してください")
		}
		return "", nil
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		t.ErrAt("adapter", "アダプタの名前 "+quote(name)+" が誤っています（空の名前、/ か \\ を含む名前、. と .. は使えません）", "組込みのアダプタの名前か、.vanda/adapters/ の直下のファイルかディレクトリの名前を書いてください")
		return "", nil
	}
	rel := AdaptersDir + "/" + name
	info, err := os.Stat(filepath.Join(l.root, filepath.FromSlash(rel)))
	switch {
	case err == nil:
		if info.IsDir() {
			rel += "/run"
		}
		return "", &UserAdapter{Name: name, Path: rel, Args: args}
	case !errors.Is(err, os.ErrNotExist):
		t.ErrAt("adapter", rel+" を確かめられません（"+err.Error()+"）", rel+" を読めるようにしてください")
		return "", nil
	}
	if !contains(builtins, name) {
		if l.allowMissing {
			l.missing = append(l.missing, name)
			return "", &UserAdapter{Name: name, Path: rel, Args: args}
		}
		t.ErrAt("adapter", "アダプタ "+quote(name)+" は、組込みにも "+AdaptersDir+"/ にもありません", fix+"、"+rel+" にアダプタを置いてください")
		return "", nil
	}
	if hasArgs {
		t.ErrAt("args", "args は、.vanda/adapters/ のアダプタのときだけ書けます（"+quote(name)+" は組込みのアダプタです）", "args を消してください")
	}
	return name, nil
}

// AdapterError は、.vanda/adapters/ のアダプタを動かせない理由（What）と直し方（Fix）。どちらも「。」で終えない。
type AdapterError struct {
	What, Fix string
}

func (e *AdapterError) Error() string { return e.What + "。" + e.Fix + "。" }

// Check は、.vanda/adapters/ のアダプタがあって実行できるかを確かめる。
func (u *UserAdapter) Check(root string) *AdapterError {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(u.Path)))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return &AdapterError{u.Label() + "がありません", "ディレクトリのアダプタは、直下に実行するファイル run を置いてください"}
	case err != nil:
		return &AdapterError{u.Label() + "を確かめられません（" + err.Error() + "）", u.Path + " を読めるようにしてください"}
	case !info.Mode().IsRegular():
		return &AdapterError{u.Label() + "が、ふつうのファイルではありません", u.Path + " に実行するファイルを置いてください"}
	case info.Mode().Perm()&0o111 == 0:
		return &AdapterError{u.Label() + "に、実行の権限がありません", "chmod +x " + u.Path + " で権限を付けてください"}
	}
	return nil
}

// Label は、誤りの文で .vanda/adapters/ のアダプタを指す言い方。
func (u *UserAdapter) Label() string {
	return "アダプタ " + u.Name + "（" + u.Path + "）"
}

// Cmd は、.vanda/adapters/ のアダプタを、リポジトリの根を作業のディレクトリにして動かすコマンドを作る。
func (u *UserAdapter) Cmd(ctx context.Context, root string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, filepath.Join(root, filepath.FromSlash(u.Path)), u.Args...)
	cmd.Dir = root
	return cmd
}

// StartError は、Cmd を動かせなかった（動いた後に失敗したのではない）なら、理由と直し方を返す。そうでなければ nil。
func (u *UserAdapter) StartError(root string, err error) *AdapterError {
	if cerr := u.Check(root); cerr != nil {
		return cerr
	}
	if errors.Is(err, syscall.ENOEXEC) {
		return &AdapterError{u.Label() + "を実行できません（" + err.Error() + "）", "1 行目に、動かし方（例: #!/usr/bin/env python3）を書いてください"}
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return &AdapterError{u.Label() + "を実行できません（" + err.Error() + "）", u.Path + " を、動かせるファイルにしてください"}
	}
	return nil
}

func parseStoryStore(l *loader, t *tomlread.Table, st *StoryStore) {
	repository, hasRepo := t.Str("repository", false)
	apiURL, hasAPI := t.Str("api_url", false)
	st.Adapter, st.User = adapterOf(l, t, []string{"github"}, `adapter = "github" にするか`)
	if st.Adapter == "github" {
		if !hasRepo {
			t.ErrAt("", `adapter = "github" なのに、repository がありません`, `repository = "<owner>/<repo>" を書いてください`)
		} else if !isRepository(repository) {
			t.ErrAt("repository", "repository "+quote(repository)+" が <owner>/<repo> の形ではありません", `repository = "TadahiroYamamura/vanda" のように書いてください`)
		}
		st.Repository = repository
		st.APIURL = DefaultGitHubAPI
		if hasAPI {
			if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
				t.ErrAt("api_url", "api_url "+quote(apiURL)+" が、http:// か https:// で始まる URL ではありません", `api_url = "https://api.github.com" のように書いてください`)
			}
			st.APIURL = strings.TrimSuffix(apiURL, "/")
		}
		return
	}
	if hasRepo {
		t.ErrAt("repository", "repository は、組込みの github のアダプタを使うときだけ書けます", `adapter = "github" を書くか、repository を消してください（.vanda/adapters/ に github という名前のアダプタがあれば、そちらを使います）`)
	}
	if hasAPI {
		t.ErrAt("api_url", "api_url は、組込みの github のアダプタを使うときだけ書けます", `adapter = "github" を書くか、api_url を消してください（.vanda/adapters/ に github という名前のアダプタがあれば、そちらを使います）`)
	}
}

func checkVersion(top *tomlread.Table, v string) {
	want, err := version.Parse(v)
	if err != nil {
		top.ErrAt("version", "version "+quote(v)+" が major.minor.patch の形ではありません", `version = "`+version.Current+`" のように書いてください`)
		return
	}
	cur, err := version.Parse(version.Current)
	if err != nil {
		top.ErrAt("version", "動いている vanda の版 "+quote(version.Current)+" を読めません", "vanda の不具合です。開発者に知らせてください")
		return
	}
	switch c := version.Compare(cur, want); {
	case c < 0:
		top.ErrAt("version", "動いている vanda（"+version.Current+"）が、設定の版（"+v+"）より古いです", "vanda を "+v+" に上げてください")
	case c > 0:
		top.ErrAt("version", "動いている vanda（"+version.Current+"）が、設定の版（"+v+"）より新しいです", "vanda の移行のコマンドで、.vanda/ を "+version.Current+" の形に書き換えてください")
	}
}

func patterns(t *tomlread.Table, key, def string) []string {
	ps, ok := t.Strs(key, def == "")
	if !ok {
		if def == "" {
			return nil
		}
		return []string{def}
	}
	if len(ps) == 0 {
		t.ErrAt(key, key+" にパターンがありません", key+` = ["…"] のように、1 つ以上書いてください`)
	}
	for _, p := range ps {
		if err := glob.Validate(p); err != nil {
			t.ErrAt(key, "パターン "+quote(p)+" の書き方が誤っています", "リポジトリの直下からの / 区切りの場所で書いてください（* と ? と [] と、何段のディレクトリにも当たる ** が使えます）")
		}
	}
	return ps
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func quote(s string) string { return `"` + s + `"` }

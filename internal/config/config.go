// Package config は、.vanda/config.toml を読む。
package config

import (
	"regexp"
	"strings"

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

// 中に持つアノテーションの読み取りのアダプタ
var BuiltinAdapters = []string{"go", "txtar", "tla", "sql"}

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

// StoryStore は、ストーリーの置き場所のアダプタ。Adapter と Command のどちらも空なら、置き場所につながない。
type StoryStore struct {
	Adapter    string
	Repository string
	APIURL     string
	Command    []string
}

const DefaultGitHubAPI = "https://api.github.com"

type Layer struct {
	ID          string
	Name        string
	Description string
}

// AnnotationRule は、読むファイルのパターンと、使うアダプタ。Adapter と Command のどちらも空なら、アダプタが決まっていない。
type AnnotationRule struct {
	Paths   []string
	Adapter string
	Command []string
	Line    int
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
	var errs []diag.Error
	doc := tomlread.Load(root, File, &errs)
	if doc == nil {
		return nil, errs
	}
	cfg := parse(doc.Top())
	if len(errs) > 0 {
		diag.Sort(errs)
		return nil, errs
	}
	return cfg, nil
}

func parse(top *tomlread.Table) *Config {
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
		adapter, hasAdapter := t.Str("adapter", false)
		command, hasCommand := t.Strs("command", false)
		switch {
		case hasAdapter && hasCommand:
			t.ErrAt("command", "adapter と command の両方が書かれています", "vanda の中のアダプタを使うなら adapter だけを、外部のコマンドを使うなら command だけを書いてください")
		case hasAdapter:
			if !contains(BuiltinAdapters, adapter) {
				t.ErrAt("adapter", "アダプタ "+quote(adapter)+" は vanda の中にありません", "adapter を "+strings.Join(BuiltinAdapters, "・")+" のどれかにするか、外部のコマンドを command = [\"…\"] で書いてください")
			}
			r.Adapter = adapter
		case hasCommand:
			if len(command) == 0 || command[0] == "" {
				t.ErrAt("command", "command に、実行するコマンドがありません", `command = ["コマンド", "引数"] のように書いてください`)
			}
			r.Command = command
		}
		t.Unknown("paths, adapter, command")
		cfg.Annotations = append(cfg.Annotations, r)
	}

	guards, _ := top.Tables("guardrail", false)
	for _, t := range guards {
		var g GuardrailRule
		if a, ok := t.Str("adapter", true); ok {
			if a != "file" {
				t.ErrAt("adapter", "ガードレールの定義の取り出しのアダプタ "+quote(a)+" は vanda の中にありません", `adapter = "file" にしてください（今は、ファイルを読むだけのアダプタしかありません）`)
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
		parseStoryStore(story, &cfg.Story)
		story.Unknown("id_pattern, adapter, repository, api_url, command")
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

func parseStoryStore(t *tomlread.Table, st *StoryStore) {
	adapter, hasAdapter := t.Str("adapter", false)
	command, hasCommand := t.Strs("command", false)
	repository, hasRepo := t.Str("repository", false)
	apiURL, hasAPI := t.Str("api_url", false)
	switch {
	case hasAdapter && hasCommand:
		t.ErrAt("command", "[story] に adapter と command の両方が書かれています", "vanda の中のアダプタを使うなら adapter だけを、外部のコマンドを使うなら command だけを書いてください")
	case hasAdapter:
		if adapter != "github" {
			t.ErrAt("adapter", "ストーリーの置き場所のアダプタ "+quote(adapter)+" は vanda の中にありません", `adapter = "github" にするか、外部のコマンドを command = ["…"] で書いてください`)
		}
		st.Adapter = adapter
	case hasCommand:
		if len(command) == 0 || command[0] == "" {
			t.ErrAt("command", "[story] の command に、実行するコマンドがありません", `command = ["コマンド", "引数"] のように書いてください`)
		}
		st.Command = command
	}
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
		t.ErrAt("repository", `repository は、adapter = "github" のときだけ書けます`, `adapter = "github" を書くか、repository を消してください`)
	}
	if hasAPI {
		t.ErrAt("api_url", `api_url は、adapter = "github" のときだけ書けます`, `adapter = "github" を書くか、api_url を消してください`)
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

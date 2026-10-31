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
}

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
				t.ErrAt("id", "レイヤーの ID "+quote(l.ID)+" が決まった形でない", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: model、db）にする")
			} else if seen[l.ID] {
				t.ErrAt("id", "レイヤーの ID "+quote(l.ID)+" がほかのレイヤーと重なっている", "別の ID にする")
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
			t.ErrAt("command", "adapter と command の両方を書いている", "中に持つアダプタを使うなら adapter だけを、外部のコマンドを使うなら command だけを書く")
		case hasAdapter:
			if !contains(BuiltinAdapters, adapter) {
				t.ErrAt("adapter", "アダプタ "+quote(adapter)+" は vanda の中にない", "adapter を "+strings.Join(BuiltinAdapters, "・")+" のどれかにするか、外部のコマンドを command = [\"…\"] で書く")
			}
			r.Adapter = adapter
		case hasCommand:
			if len(command) == 0 || command[0] == "" {
				t.ErrAt("command", "command に実行するコマンドがない", `command = ["コマンド", "引数"] のように書く`)
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
				t.ErrAt("adapter", "ガードレールの定義の取り出しのアダプタ "+quote(a)+" は vanda の中にない", `adapter = "file" にする（今はファイルを読むだけのアダプタしかない）`)
			}
			g.Adapter = a
		}
		g.Paths = patterns(t, "paths", "")
		t.Unknown("adapter, paths")
		cfg.Guardrails = append(cfg.Guardrails, g)
	}

	if re, err := regexp.Compile(DefaultIDPattern); err != nil {
		top.ErrAt("", "vanda の中のストーリーの ID の既定の書き方が読めない（"+err.Error()+"）", "vanda の不具合なので、知らせる")
	} else {
		cfg.IDPattern = re
	}
	if story, ok := top.Table("story", false); ok {
		if p, ok := story.Str("id_pattern", false); ok {
			re, err := regexp.Compile(p)
			if err != nil {
				story.ErrAt("id_pattern", "id_pattern が正規表現として読めない（"+err.Error()+"）", "Go の正規表現（RE2）の書き方で直す")
			} else if re.MatchString("") {
				story.ErrAt("id_pattern", "id_pattern が空の文字列に当たる", "ストーリーの ID 全体に当たる正規表現（例: #[0-9]+）にする")
			} else {
				cfg.IDPattern = re
			}
		}
		story.Unknown("id_pattern")
	}

	if cfg.IDPattern != nil {
		if re, err := regexp.Compile(`^(?:` + cfg.IDPattern.String() + `)$`); err != nil {
			top.ErrAt("", "ストーリーの ID の書き方を、ID 全体に当たる形にできない（"+err.Error()+"）", "id_pattern を直す")
		} else {
			cfg.IDFull = re
		}
	}
	top.Unknown("version, promises, agreements, layer, annotation, guardrail, story")
	return cfg
}

func checkVersion(top *tomlread.Table, v string) {
	want, err := version.Parse(v)
	if err != nil {
		top.ErrAt("version", "version "+quote(v)+" が major.minor.patch の形でない", `version = "`+version.Current+`" のように書く`)
		return
	}
	cur, err := version.Parse(version.Current)
	if err != nil {
		top.ErrAt("version", "動いている vanda の版 "+quote(version.Current)+" が読めない", "vanda の不具合なので、知らせる")
		return
	}
	switch c := version.Compare(cur, want); {
	case c < 0:
		top.ErrAt("version", "動いている vanda（"+version.Current+"）が、設定の版（"+v+"）より古い", "vanda を "+v+" に上げる")
	case c > 0:
		top.ErrAt("version", "動いている vanda（"+version.Current+"）が、設定の版（"+v+"）より新しい", "vanda の移行のコマンドで .vanda/ を "+version.Current+" の形に書き換える")
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
		t.ErrAt(key, key+" にパターンがない", key+` = ["…"] のように、1 つ以上書く`)
	}
	for _, p := range ps {
		if err := glob.Validate(p); err != nil {
			t.ErrAt(key, "パターン "+quote(p)+" の書き方が誤っている", "リポジトリの直下からの / 区切りの場所で書く（* と ? と [] と、何段のディレクトリにも当たる ** が使える）")
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

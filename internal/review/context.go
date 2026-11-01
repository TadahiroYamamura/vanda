package review

import (
	"sort"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/version"
)

type Context struct {
	Version     string     `json:"version"`
	Branch      string     `json:"branch"`
	Head        string     `json:"head"`
	DefaultBase string     `json:"defaultBase"`
	Bases       []string   `json:"bases"`
	User        *User      `json:"user"`
	Stories     []StoryRef `json:"stories"`
	Layers      []LayerRef `json:"layers"`
	// StoryStore は、設定の [story] の adapter に書いたアダプタの名前。設定になければ空
	StoryStore string `json:"storyStore"`
}

type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type StoryRef struct {
	ID       string `json:"id"`
	InBranch bool   `json:"inBranch"`
}

// GitUser は、git の user.name と user.email を返す。どちらかがなければ nil。
func GitUser(root string) *User {
	name, err1 := repo.Git(root, "config", "user.name")
	email, err2 := repo.Git(root, "config", "user.email")
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	if err1 != nil || err2 != nil || name == "" || email == "" {
		return nil
	}
	return &User{Name: name, Email: email}
}

// LoadContext は、今の設定と、画面で選べるもの（ベースブランチ、ストーリー）を返す。
func LoadContext(root string, cfg *config.Config) (*Context, *Error) {
	c := &Context{Version: version.Current, DefaultBase: cfg.GUIBase, Bases: []string{}, Stories: []StoryRef{}, Layers: []LayerRef{}, User: GitUser(root)}
	c.StoryStore = cfg.Story.Name()
	for _, l := range cfg.Layers {
		c.Layers = append(c.Layers, LayerRef{ID: l.ID, Name: l.Name})
	}
	if b, err := repo.Git(root, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		c.Branch = strings.TrimSpace(b)
	}
	if h, err := repo.Git(root, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		c.Head = strings.TrimSpace(h)
	}
	refs, err := repo.Git(root, "for-each-ref", "--format=%(refname:short)%00%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, internalErr(err)
	}
	for _, line := range strings.Split(refs, "\n") {
		name, symref, _ := strings.Cut(line, "\x00")
		// origin/HEAD のような、ほかのブランチを指すだけの名前は除く
		if name == "" || symref != "" {
			continue
		}
		c.Bases = append(c.Bases, name)
	}
	sort.Strings(c.Bases)

	h := newHistory(root, cfg)
	commits, err := h.loadCommits()
	if err != nil {
		return nil, internalErr(err)
	}
	inBranch := map[string]bool{}
	if c.DefaultBase != "" && c.Head != "" {
		if mb, err := repo.Git(root, "merge-base", c.DefaultBase, "HEAD"); err == nil {
			ids, err := storyIDsIn(root, cfg, strings.TrimSpace(mb)+"..HEAD")
			if err != nil {
				return nil, internalErr(err)
			}
			for _, id := range ids {
				inBranch[id] = true
			}
		}
	}
	seen := map[string]bool{}
	for i := len(commits) - 1; i >= 0; i-- {
		for _, id := range commits[i].Stories {
			if !seen[id] {
				seen[id] = true
				c.Stories = append(c.Stories, StoryRef{ID: id, InBranch: inBranch[id]})
			}
		}
	}
	return c, nil
}

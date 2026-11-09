package repo

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

// Tree は、1 つのコミットのファイルの一覧と中身。中身は、要るときに読む。
type Tree struct {
	root  string
	Rev   string // コミットのハッシュ
	Files []string
	oid   map[string]string
	mu    sync.Mutex // cache を守る。1 つのコミットの中身を、同時に読むことがあるため
	cache map[string][]byte
}

// HeadTree は、HEAD のコミットを読む。まだコミットがなければ、ファイルのない中身を返す
// （コミットしていないものは読まないので、ないのと同じ。ないファイルの誤りで、何をコミットすればよいかを知らせるため）。
func HeadTree(root string) (*Tree, error) {
	h, err := Git(root, "rev-parse", "--verify", "-q", "HEAD^{commit}")
	if err != nil {
		// HEAD がブランチを指しているのにコミットを読めないのは、まだ 1 つもコミットしていないブランチ
		if _, uerr := Git(root, "symbolic-ref", "-q", "HEAD"); uerr == nil {
			return &Tree{root: root, oid: map[string]string{}, cache: map[string][]byte{}}, nil
		}
		return nil, fmt.Errorf("HEAD のコミットを読めません（%v）。", err)
	}
	return TreeAt(root, strings.TrimSpace(h))
}

// TreeAt は、コミット rev を読む。HEAD が途中で動いても同じ中身を読むよう、rev にはハッシュを渡す。
func TreeAt(root, rev string) (*Tree, error) {
	out, err := gitByID(root, rev, "ls-tree", "-r", "-z", "--full-tree", rev)
	if err != nil {
		return nil, fmt.Errorf("コミット %s のファイルの一覧を読めません（%v）。", rev, err)
	}
	t := &Tree{root: root, Rev: rev, oid: map[string]string{}, cache: map[string][]byte{}}
	for _, ent := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(ent, "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		// サブモジュール（commit）は、ファイルとして読めないので数えない
		if len(f) != 3 || f[1] != "blob" {
			continue
		}
		t.Files = append(t.Files, path)
		t.oid[path] = f[2]
	}
	sort.Strings(t.Files)
	return t, nil
}

// Has は、コミットに path のファイルがあるかを返す。
func (t *Tree) Has(path string) bool {
	_, ok := t.oid[path]
	return ok
}

// Read は、コミットの path のファイルの中身を返す。なければ fs.ErrNotExist を包んだエラーを返す。
func (t *Tree) Read(path string) ([]byte, error) {
	t.mu.Lock()
	b, ok := t.cache[path]
	t.mu.Unlock()
	if ok {
		return b, nil
	}
	oid, ok := t.oid[path]
	if !ok {
		return nil, fmt.Errorf("コミット %s に %s がありません: %w", short(t.Rev), path, fs.ErrNotExist)
	}
	out, err := gitByID(t.root, oid, "cat-file", "blob", oid)
	if err != nil {
		return nil, err
	}
	b = []byte(out)
	t.mu.Lock()
	t.cache[path] = b
	t.mu.Unlock()
	return b, nil
}

func short(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

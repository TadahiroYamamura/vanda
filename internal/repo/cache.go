package repo

import (
	"strings"
	"sync"
)

// cacheLimit は、覚えておく git の出力の大きさの上限（バイト）。
// 覚えるものは履歴とともに増え続け、vanda serve は長く動くので、上限を設けて古く使ったものから捨てる。
// vanda のリポジトリ（648 コミット、約束リストが 180 KB）で一覧と由来と合意の履歴を開いて覚えるのは、約 31 MB（1694 件）
const cacheLimit = 256 << 20

// cache は、中身が変わらない git の出力（コミット・ツリー・ブロブの ID で決まるもの）を覚えておく。
// 同時に読み書きしてよい。
var cache = newLRU(cacheLimit)

type lru struct {
	mu    sync.Mutex
	limit int
	size  int
	// head から tail へ、新しく使った順につなぐ
	head, tail *lruEntry
	items      map[string]*lruEntry
}

type lruEntry struct {
	key, val   string
	prev, next *lruEntry
}

func newLRU(limit int) *lru {
	return &lru{limit: limit, items: map[string]*lruEntry{}}
}

func (c *lru) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.unlink(e)
	c.pushFront(e)
	return e.val, true
}

func (c *lru) put(key, val string) {
	n := len(key) + len(val)
	if n > c.limit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return
	}
	e := &lruEntry{key: key, val: val}
	c.items[key] = e
	c.pushFront(e)
	c.size += n
	for c.size > c.limit {
		old := c.tail
		c.unlink(old)
		delete(c.items, old.key)
		c.size -= len(old.key) + len(old.val)
	}
}

func (c *lru) pushFront(e *lruEntry) {
	e.prev, e.next = nil, c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *lru) unlink(e *lruEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

// gitByID は、id で中身の決まる git の出力を、覚えたものがあれば使い、なければ git を動かして覚える。
// HEAD やブランチの名前は指す先が変わるので、id が完全な ID の形でなければ覚えない。
func gitByID(dir, id string, args ...string) (string, error) {
	if !isFullID(id) {
		return Git(dir, args...)
	}
	key := dir + "\x00" + strings.Join(args, "\x00")
	if out, ok := cache.get(key); ok {
		return out, nil
	}
	out, err := Git(dir, args...)
	if err != nil {
		return "", err
	}
	cache.put(key, out)
	return out, nil
}

// isFullID は、s が、省略していない SHA-1（40 桁）か SHA-256（64 桁）のオブジェクトの ID の形かを返す。
// 省略した ID は、後から同じ頭のオブジェクトが増えると指す先が決まらなくなるので、覚えない。
func isFullID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// ChangedFiles は、コミット commit が 1 つ目の親から変えたファイルの場所を、NUL で区切って返す（最初のコミットなら、すべてのファイル）。
func ChangedFiles(dir, commit string) (string, error) {
	return gitByID(dir, commit, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root", commit)
}

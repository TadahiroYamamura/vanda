// Package server は、vanda serve が起動する vanda サーバー。API と画面のファイルを出す。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/review"
	"github.com/TadahiroYamamura/vanda/internal/story"
)

type Server struct {
	Root string
	// Cfg は、起動したときに読んだ設定。要求ごとには、設定を読み直して使う（config を参照）
	Cfg *config.Config
	// UI は、画面のファイル。index.html がなければ、画面を作っていないと知らせる
	UI fs.FS
	// mu は、合意（ファイルを書いてコミットする）のあいだに、一覧が書きかけの中身を読まないようにする
	mu sync.RWMutex
}

// Handler は、API（/api/ の下）と画面のファイルを出す http.Handler を返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/context", s.read(s.context))
	mux.HandleFunc("GET /api/review", s.read(s.review))
	mux.HandleFunc("POST /api/agree", s.agree)
	mux.HandleFunc("POST /api/revoke", s.revoke)
	mux.HandleFunc("GET /api/claims/{id}", s.read(s.claim))
	mux.HandleFunc("GET /api/claims/{id}/at/{commit}", s.read(s.claimAt))
	mux.HandleFunc("GET /api/file", s.read(s.file))
	mux.HandleFunc("GET /api/story", s.story)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "この API はありません。", "docs/formats.md の「GUI の API」を確かめてください。")
	})
	mux.Handle("/", s.ui())
	return localOnly(mux)
}

// config は、要求ごとに設定（.vanda/config.toml）を読み直す。起動した後にブランチを切り替えたり、設定を書き換えたりしても、
// 画面を開き直すだけで今の設定が効くようにするため（起動したときの設定を使い続けると、たとえばレイヤーの名前が古いままになる）。
// 読めなければ、誤りを返して false を返す
func (s *Server) config(w http.ResponseWriter) (*config.Config, bool) {
	tree, err := repo.HeadTree(s.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "git の状態を確かめてください。")
		return nil, false
	}
	cfg, errs := config.LoadCommitted(s.Root, tree)
	if errs != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("設定（%s）を読めません（%d 件。最初の 1 件: %s）。", config.File, len(errs), errs[0].Error()), "vanda doctor で誤りを確かめて、直してください。")
		return nil, false
	}
	return cfg, true
}

func (s *Server) read(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		h(w, r)
	}
}

func (s *Server) context(w http.ResponseWriter, _ *http.Request) {
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	c, err := review.LoadContext(s.Root, cfg)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) review(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	res, err := review.Build(review.Options{Root: s.Root, Cfg: cfg, View: q.Get("view"), Base: q.Get("base"), Story: q.Get("story")})
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	d, err := review.Claim(review.Options{Root: s.Root, Cfg: cfg}, r.PathValue("id"))
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) claimAt(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	d, err := review.ClaimAtCommit(review.Options{Root: s.Root, Cfg: cfg}, r.PathValue("id"), r.PathValue("commit"))
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	text, err := review.File(review.Options{Root: s.Root, Cfg: cfg}, p)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": p, "text": text})
}

// story は、置き場所を読むだけでリポジトリを変えないので、合意と並べて動かしてよい（ロックを取らない）
func (s *Server) story(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	st, err := story.Read(s.Root, cfg, r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, err.Status, err.Message, err.Fix)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func writeReviewError(w http.ResponseWriter, e *review.Error) {
	writeError(w, e.Status, e.Message, e.Fix)
}

// ui は、画面のファイルを出す。ないパスは index.html を返す（画面の中の場所を URL に持つため）。
func (s *Server) ui() http.Handler {
	files := http.FileServer(http.FS(s.UI))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(s.UI, "index.html"); err != nil {
			http.Error(w, "画面のファイルが vanda に埋め込まれていません。make build で画面を作ってから、vanda をビルドし直してください。", http.StatusServiceUnavailable)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(s.UI, p); errors.Is(err, fs.ErrNotExist) {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		files.ServeHTTP(w, r)
	})
}

// localOnly は、Host が 127.0.0.1:<ポート>・localhost:<ポート> でない要求と、ほかのサイトから送られた書き込みの要求を断る。
// 127.0.0.1 で待ち受けるだけでは、ブラウザで開いたほかのサイトの頁から、DNS の付け替えや、フォームの送信で、
// 合意（コミットを作る）を送らせることができてしまうため。
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host, listeningPort(r)) {
			writeError(w, http.StatusForbidden, "Host "+r.Host+" からの要求は受け付けません。", "http://127.0.0.1:<ポート>/ で開いてください。")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin != "" && origin != "http://"+r.Host {
				writeError(w, http.StatusForbidden, "ほかのサイト（"+origin+"）からの書き込みの要求は受け付けません。", "vanda の画面から操作してください。")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalHost(hostport, port string) bool {
	host, p, err := net.SplitHostPort(hostport)
	if err != nil || port == "" || p != port {
		return false
	}
	return host == "127.0.0.1" || host == "localhost"
}

// listeningPort は、要求を受けた接続の、待ち受けているポートを返す。分からなければ空を返す（isLocalHost は断る）。
// Handler を作るときにポートを渡さず接続から読むのは、--port 0 では待ち受けを始めるまでポートが決まらないため。
func listeningPort(r *http.Request) string {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		log.Printf("要求 %s %s を受けた接続の、待ち受けているポートが分かりません。断ります。", r.Method, r.URL.Path)
		return ""
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		log.Printf("待ち受けている場所 %s からポートを読めません（%v）。断ります。", addr.String(), err)
		return ""
	}
	return port
}

// apiError は、API の誤りの形。
type apiError struct {
	Error struct {
		Message string `json:"message"`
		Fix     string `json:"fix,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message, fix string) {
	var e apiError
	e.Error.Message = message
	e.Error.Fix = fix
	writeJSON(w, status, e)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("API の応答を書けませんでした: %v", err)
	}
}

func (s *Server) agree(w http.ResponseWriter, r *http.Request) {
	var req review.AgreeRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "合意の要求の JSON を読めません（"+err.Error()+"）。", "docs/formats.md の POST /api/agree の形で送ってください。")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	res, err := review.Agree(review.Options{Root: s.Root, Cfg: cfg}, req, time.Now())
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	var req review.RevokeRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "取り消しの要求の JSON を読めません（"+err.Error()+"）。", "docs/formats.md の POST /api/revoke の形で送ってください。")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.config(w)
	if !ok {
		return
	}
	res, err := review.Revoke(review.Options{Root: s.Root, Cfg: cfg}, req)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

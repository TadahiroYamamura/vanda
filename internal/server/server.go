// Package server は、vanda serve の HTTP のサーバー。API と画面のファイルを出す。
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/config"
)

type Server struct {
	Root string
	Cfg  *config.Config
	// UI は、画面のファイル。index.html がなければ、画面を作っていないと知らせる
	UI fs.FS
}

// Handler は、API（/api/ の下）と画面のファイルを出す http.Handler を返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "この API はない", "")
	})
	mux.Handle("/", s.ui())
	return localOnly(mux)
}

// ui は、画面のファイルを出す。ないパスは index.html を返す（画面の中の場所を URL に持つため）。
func (s *Server) ui() http.Handler {
	files := http.FileServer(http.FS(s.UI))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(s.UI, "index.html"); err != nil {
			http.Error(w, "画面のファイルが vanda に埋め込まれていない。make build で画面を作ってから、vanda をビルドし直す", http.StatusServiceUnavailable)
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

// localOnly は、Host が 127.0.0.1・localhost でない要求と、ほかのサイトから送られた書き込みの要求を断る。
// 127.0.0.1 で待ち受けるだけでは、ブラウザで開いたほかのサイトの頁から、DNS の付け替えや、フォームの送信で、
// 合意（コミットを作る）を送らせることができてしまうため。
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host) {
			writeError(w, http.StatusForbidden, "Host "+r.Host+" からの要求は受けない", "http://127.0.0.1:<ポート>/ で開く")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin != "" && origin != "http://"+r.Host {
				writeError(w, http.StatusForbidden, "ほかのサイト（"+origin+"）からの書き込みの要求は受けない", "vanda の画面から操作する")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	return host == "127.0.0.1" || host == "localhost"
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
		log.Printf("API の応答を書けなかった: %v", err)
	}
}

package server_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TadahiroYamamura/vanda/internal/server"
)

// @vanda:evidence serve-rejects-foreign-host
func TestHostが待ち受けているポートの127_0_0_1かlocalhostでない要求は403で断る(t *testing.T) {
	// 45123 で待ち受けている接続に来た要求として送る。ない API（/api/nope）は、Host を通れば 404 になる
	listening := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 45123}
	cases := []struct {
		host string
		want int
	}{
		{"127.0.0.1:45123", http.StatusNotFound},
		{"localhost:45123", http.StatusNotFound},
		// 待ち受けているポートと違うポート
		{"127.0.0.1:45124", http.StatusForbidden},
		{"localhost:45124", http.StatusForbidden},
		{"127.0.0.1:80", http.StatusForbidden},
		// ポートのない Host
		{"127.0.0.1", http.StatusForbidden},
		{"localhost", http.StatusForbidden},
		{"127.0.0.1:", http.StatusForbidden},
		// ほかのホストの名前
		{"example.com:45123", http.StatusForbidden},
		{"example.com", http.StatusForbidden},
		{"localhost.example.com:45123", http.StatusForbidden},
		{"127.0.0.1.example.com:45123", http.StatusForbidden},
		{"127.0.0.2:45123", http.StatusForbidden},
		{"[::1]:45123", http.StatusForbidden},
		{"", http.StatusForbidden},
	}
	h := (&server.Server{}).Handler()
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/nope", nil)
		req.Host = c.host
		req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, listening))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("45123 で待ち受けているとき、Host が %q の要求に %d を返した（%d を返す）", c.host, rec.Code, c.want)
		}
	}
}

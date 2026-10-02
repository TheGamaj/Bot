// Package miniapp serves the Gamaj Bot Mini App (is.0.0.1): a single-page
// Telegram Web App themed after the Gamaj Panel (black & white). It renders
// the buyer's plan list, services and subscription links using the same
// Gamaj API the bot uses.
package miniapp

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed index.html
var indexHTML string

// Version is the Gamaj Bot Mini App release version.
const Version = "is.0.0.1"

// Handler serves the Mini App page on /miniapp.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/miniapp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(indexHTML))
	})
	mux.HandleFunc("/miniapp/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/miniapp/") != "" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/miniapp", http.StatusMovedPermanently)
	})
	return mux
}

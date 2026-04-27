package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Register mounts the dashboard routes on the given mux.
// The dashboard is served at /dashboard/ and static assets at /dashboard/static/.
func Register(mux *http.ServeMux) {
	// Serve the SPA index.html for the dashboard root and all sub-routes
	mux.HandleFunc("GET /dashboard", redirectToDashboard)
	mux.HandleFunc("GET /dashboard/{path...}", serveSPA)

	// Serve static assets (CSS, JS)
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
}

func redirectToDashboard(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
}

func serveSPA(w http.ResponseWriter, r *http.Request) {
	// Always serve index.html for SPA client-side routing
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Dashboard not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

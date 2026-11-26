package server

import (
	"embed"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/gorilla/mux"
	"performancereport/handlers"
	"performancereport/storage"
)

// Frontend holds the compiled Vue app.
// In development (no embed) it proxies to Vite dev server.
var Frontend embed.FS

func Start(port string, frontendFS embed.FS) {
	Frontend = frontendFS
	storage.EnsureDir(storage.DataPath())

	r := mux.NewRouter()
	registerRoutes(r, frontendFS)

	r.Use(corsMiddleware)
	r.Use(loggingMiddleware)

	addr := "127.0.0.1:" + port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to bind to %s: %v", addr, err)
	}

	url := "http://" + addr
	log.Printf("PerformanceReport running at %s", url)

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

	if err := http.Serve(listener, r); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func registerRoutes(r *mux.Router, frontendFS embed.FS) {
	api := r.PathPrefix("/api").Subrouter()

	// Profile
	api.HandleFunc("/profile", handlers.GetProfile).Methods("GET")
	api.HandleFunc("/profile", handlers.UpdateProfile).Methods("PUT", "POST")

	// Tasks
	api.HandleFunc("/tasks", handlers.GetAllTasks).Methods("GET")
	api.HandleFunc("/tasks/{year:[0-9]{4}}/{month:[0-9]{2}}", handlers.GetTasks).Methods("GET")
	api.HandleFunc("/tasks", handlers.CreateTask).Methods("POST")
	api.HandleFunc("/tasks/{id}", handlers.UpdateTask).Methods("PUT")
	api.HandleFunc("/tasks/{id}", handlers.DeleteTask).Methods("DELETE")
	api.HandleFunc("/tasks/bulk", handlers.BulkImportTasks).Methods("POST")
	api.HandleFunc("/stats", handlers.GetTaskStats).Methods("GET")

	// Full Month Entry
	api.HandleFunc("/fullmonth/{year:[0-9]{4}}/{month:[0-9]{2}}", handlers.GetFullMonthDraft).Methods("GET")
	api.HandleFunc("/fullmonth/{year:[0-9]{4}}/{month:[0-9]{2}}", handlers.SaveFullMonthDraft).Methods("PUT", "POST")
	api.HandleFunc("/fullmonth/parse", handlers.ParseMonthlyText).Methods("POST")

	// Reports
	api.HandleFunc("/reports", handlers.ListReports).Methods("GET")
	api.HandleFunc("/reports/history", handlers.GetMonthHistory).Methods("GET")
	api.HandleFunc("/reports/reminders", handlers.GetMonthlyReminders).Methods("GET")
	api.HandleFunc("/reports/generate", handlers.GenerateReport).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}", handlers.GetReport).Methods("GET")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}", handlers.UpdateReport).Methods("PUT")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/status", handlers.GetMonthStatus).Methods("GET")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/remind-later", handlers.RemindLaterReport).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/complete", handlers.CompleteReport).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/missing-summary", handlers.GetMissingTaskSummary).Methods("GET")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/export/pdf", handlers.ExportPDFDownload).Methods("GET")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/export/docx", handlers.ExportDocx).Methods("GET")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/preview", handlers.GetReportHTML).Methods("GET")

	// Document Editor (Word-like editor & Microsoft Word working copy)
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor", handlers.GetEditorDocument).Methods("GET")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor/status", handlers.GetEditorStatus).Methods("GET")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor/draft", handlers.SaveEditorDraft).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor/save", handlers.SaveEditorChanges).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor/discard", handlers.DiscardEditorChanges).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor/open-word", handlers.OpenInMicrosoftWord).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor/sync-word", handlers.SyncFromMicrosoftWord).Methods("POST")
	api.HandleFunc("/reports/{year:[0-9]{4}}/{month:[0-9]{2}}/editor/clear", handlers.ClearEditedOnRegenerate).Methods("POST")

	// Templates
	api.HandleFunc("/templates", handlers.GetTemplates).Methods("GET")
	api.HandleFunc("/templates/import", handlers.ImportTemplate).Methods("POST")
	api.HandleFunc("/templates/{id}", handlers.UpdateTemplate).Methods("PUT")
	api.HandleFunc("/templates/{id}", handlers.DeleteTemplate).Methods("DELETE")
	api.HandleFunc("/templates/{id}/duplicate", handlers.DuplicateTemplate).Methods("POST")
	api.HandleFunc("/templates/{id}/default", handlers.SetDefaultTemplate).Methods("POST")
	api.HandleFunc("/templates/{id}/source", handlers.GetTemplateSource).Methods("GET")
	api.HandleFunc("/templates/{id}/assets/{filename}", handlers.GetTemplateAsset).Methods("GET")
	api.HandleFunc("/templates/{id}/preview", handlers.GetTemplatePreviewHTML).Methods("GET")

	// Settings
	api.HandleFunc("/settings", handlers.GetSettings).Methods("GET")
	api.HandleFunc("/settings", handlers.UpdateSettings).Methods("PUT", "POST")

	// Backup
	api.HandleFunc("/backup", handlers.CreateBackup).Methods("POST")
	api.HandleFunc("/backup", handlers.ListBackups).Methods("GET")
	api.HandleFunc("/data-folder", handlers.OpenDataFolder).Methods("GET")

	// AI
	api.HandleFunc("/ai/improve", handlers.ImproveWithAI).Methods("POST")
	api.HandleFunc("/ai/test", handlers.TestAIConnection).Methods("POST")
	api.HandleFunc("/ai/creds", handlers.GetAICreds).Methods("GET")
	api.HandleFunc("/ai/creds", handlers.SaveAICreds).Methods("POST")

	// GitLab
	api.HandleFunc("/gitlab/commits", handlers.GetGitLabCommits).Methods("GET")

	// Serve compiled frontend — all non-API routes serve index.html (SPA)
	subFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		// No embedded frontend — development mode
		r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.ServeFile(w, req, "frontend/dist/index.html")
		})
		return
	}
	staticFS := http.FileServer(http.FS(subFS))
	r.PathPrefix("/assets/").Handler(staticFS)
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// SPA fallback: try to serve static file, else index.html
		f, err := subFS.Open(req.URL.Path[1:])
		if err == nil {
			f.Close()
			staticFS.ServeHTTP(w, req)
			return
		}
		indexFile, err2 := subFS.Open("index.html")
		if err2 != nil {
			http.NotFound(w, req)
			return
		}
		indexFile.Close()
		// Rewrite to index.html
		req.URL.Path = "/"
		staticFS.ServeHTTP(w, req)
	})
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
}

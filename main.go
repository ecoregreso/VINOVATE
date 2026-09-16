package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed templates/*.html static/* catalog.json
var assets embed.FS

type App struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Size        string   `json:"size"`
	Android     string   `json:"android"`
	Package     string   `json:"package"`
	Price       string   `json:"price"`
	Free        bool     `json:"free"`
	WebURL      string   `json:"web_url"`
	APK         string   `json:"apk"`
	SHA256      string   `json:"sha256"`
	Permissions []string `json:"permissions"`
	Featured    bool     `json:"featured"`
}

type PageData struct {
	Title       string
	Description string
	Apps        []App
	App         *App
	Query       string
	Category    string
}

type Store struct {
	apps      []App
	templates *template.Template
}

func main() {
	store, err := newStore()
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", store.home)
	mux.HandleFunc("/apps/", store.appDetail)
	mux.HandleFunc("/download/", store.download)
	mux.HandleFunc("/health", health)

	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/favicon.svg" {
			http.NotFound(w, r)
			return
		}
		b, err := assets.ReadFile("static/vinovate-mark.svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write(b)
	})

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("VINOVATE running at http://localhost:%s", port)
	log.Fatal(server.ListenAndServe())
}

func newStore() (*Store, error) {
	catalogBytes, err := assets.ReadFile("catalog.json")
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var apps []App
	if err := json.Unmarshal(catalogBytes, &apps); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	funcs := template.FuncMap{
		"initial": func(s string) string {
			s = strings.TrimSpace(s)
			if s == "" {
				return "V"
			}
			return strings.ToUpper(string([]rune(s)[0]))
		},
	}

	t, err := template.New("base.html").Funcs(funcs).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	return &Store{apps: apps, templates: t}, nil
}

func (s *Store) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	filtered := make([]App, 0, len(s.apps))

	for _, app := range s.apps {
		if category != "" && !strings.EqualFold(category, app.Category) {
			continue
		}
		haystack := strings.ToLower(app.Name + " " + app.Description + " " + app.Category)
		if query != "" && !strings.Contains(haystack, strings.ToLower(query)) {
			continue
		}
		filtered = append(filtered, app)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Featured != filtered[j].Featured {
			return filtered[i].Featured
		}
		return filtered[i].Name < filtered[j].Name
	})

	s.render(w, "home.html", PageData{
		Title:       "VINOVATE — Apps for the consumer, by the consumer.",
		Description: "Discover independent apps, games and AI tools directly from the people who build them.",
		Apps:        filtered,
		Query:       query,
		Category:    category,
	})
}

func (s *Store) appDetail(w http.ResponseWriter, r *http.Request) {
	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/apps/"), "/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	for i := range s.apps {
		if s.apps[i].Slug == slug {
			s.render(w, "app.html", PageData{
				Title:       s.apps[i].Name + " — VINOVATE",
				Description: s.apps[i].Description,
				App:         &s.apps[i],
			})
			return
		}
	}

	http.NotFound(w, r)
}

func (s *Store) download(w http.ResponseWriter, r *http.Request) {
	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/download/"), "/")
	var selected *App
	for i := range s.apps {
		if s.apps[i].Slug == slug {
			selected = &s.apps[i]
			break
		}
	}
	if selected == nil {
		http.NotFound(w, r)
		return
	}

	// Paid releases fail closed until Stripe entitlement verification is wired.
	if !selected.Free {
		http.Error(w, "Purchase entitlement required", http.StatusPaymentRequired)
		return
	}

	if strings.TrimSpace(selected.APK) == "" {
		http.NotFound(w, r)
		return
	}

	base, err := filepath.Abs("private-apks")
	if err != nil {
		http.Error(w, "storage unavailable", http.StatusInternalServerError)
		return
	}
	target, err := filepath.Abs(filepath.Join(base, filepath.Base(selected.APK)))
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(target, base+string(os.PathSeparator)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	f, err := os.Open(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "download unavailable", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		http.Error(w, "verification failed", http.StatusInternalServerError)
		return
	}
	actual := hex.EncodeToString(h.Sum(nil))
	expected := strings.TrimSpace(strings.ToLower(selected.SHA256))
	if expected != "" && expected != "pending-release" && actual != expected {
		http.Error(w, "release hash mismatch", http.StatusInternalServerError)
		return
	}
	if _, err := f.Seek(0, 0); err != nil {
		http.Error(w, "download failed", http.StatusInternalServerError)
		return
	}

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "download failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(selected.APK)))
	w.Header().Set("X-Release-SHA256", actual)
	http.ServeContent(w, r, filepath.Base(selected.APK), info.ModTime(), f)
}

func (s *Store) render(w http.ResponseWriter, page string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, page, data); err != nil {
		log.Printf("template %s: %v", page, err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/health" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

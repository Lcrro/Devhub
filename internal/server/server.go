package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lcrro/devhub/internal/capture"
	"github.com/Lcrro/devhub/internal/discovery"
	"github.com/Lcrro/devhub/internal/model"
	processmgr "github.com/Lcrro/devhub/internal/process"
	"github.com/Lcrro/devhub/internal/store"
)

//go:embed web/*
var webFS embed.FS

type App struct {
	store     *store.Store
	scanner   *discovery.Scanner
	capturer  *capture.Capturer
	processes *processmgr.Manager
	scanMu    sync.Mutex
	captureMu sync.Mutex
	captures  map[string]bool
}

func New(st *store.Store) *App {
	return &App{store: st, scanner: discovery.New(), capturer: capture.New(), processes: processmgr.New(), captures: map[string]bool{}}
}

func (a *App) Start(ctx context.Context) {
	go a.scanLoop(ctx)
}

func (a *App) scanLoop(ctx context.Context) {
	_ = a.scanOnce(ctx)
	for {
		settings := a.store.Snapshot().Settings
		delay := time.Duration(settings.ScanIntervalSeconds) * time.Second
		if delay < 2*time.Second {
			delay = 5 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			_ = a.scanOnce(ctx)
		}
	}
}

func (a *App) scanOnce(ctx context.Context) error {
	if !a.scanMu.TryLock() {
		return nil
	}
	defer a.scanMu.Unlock()
	projects, err := a.scanner.Scan(ctx)
	if err != nil {
		return err
	}
	state := a.store.Snapshot()
	settings := state.Settings
	seen := map[string]bool{}
	for _, p := range projects {
		if state.Ignored[p.ID] {
			continue
		}
		existing, ok := state.Projects[p.ID]
		if !ok {
			for existingID, candidate := range state.Projects {
				if sameProject(candidate, &p) {
					p.ID = existingID
					existing = candidate
					ok = true
					break
				}
			}
		}
		if state.Ignored[p.ID] {
			continue
		}
		if !ok && !settings.AutoRemember {
			continue
		}
		if ok {
			p.Managed = existing.Managed
			if existing.Name != "" {
				p.Name = existing.Name
			}
			if existing.StartCommand != "" {
				p.StartCommand = existing.StartCommand
			}
		}
		if err := a.store.Upsert(p); err != nil {
			log.Printf("store discovered service: %v", err)
		}
		seen[p.ID] = true
		current, _ := a.store.Get(p.ID)
		if current != nil && settings.AutoCapture && current.URL != "" && captureDue(current) {
			a.captureAsync(current.ID, current.URL)
		}
	}
	latest := a.store.Snapshot()
	for id, p := range latest.Projects {
		if seen[id] {
			continue
		}
		if p.Status == "running" && p.AutoDiscovered {
			_ = a.store.Update(id, func(x *model.Project) { x.Status = "stopped"; x.PID = 0 })
		}
	}
	return nil
}

func captureDue(p *model.Project) bool {
	if p.CoverFile == "" || p.LastCaptured.IsZero() {
		return true
	}
	return time.Since(p.LastCaptured) > 24*time.Hour
}

func (a *App) captureAsync(id, targetURL string) {
	a.captureMu.Lock()
	if a.captures[id] {
		a.captureMu.Unlock()
		return
	}
	a.captures[id] = true
	a.captureMu.Unlock()
	go func() {
		defer func() { a.captureMu.Lock(); delete(a.captures, id); a.captureMu.Unlock() }()
		name := id + ".png"
		output := filepath.Join(a.store.CoversDir(), name)
		if err := a.capturer.Capture(context.Background(), targetURL, output); err != nil {
			log.Printf("capture %s: %v", id, err)
			return
		}
		_ = a.store.Update(id, func(p *model.Project) { p.CoverFile = name; p.LastCaptured = time.Now() })
	}()
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/services", a.handleServices)
	mux.HandleFunc("/api/scan", a.handleScan)
	mux.HandleFunc("/api/projects", a.handleProjects)
	mux.HandleFunc("/api/projects/", a.handleProjectAction)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/covers/", a.handleCover)
	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	mux.Handle("/", securityHeaders(static))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	state := a.store.Snapshot()
	list := make([]model.Project, 0, len(state.Projects))
	for _, p := range state.Projects {
		cp := *p
		if cp.CoverFile != "" {
			cp.CoverURL = "/covers/" + cp.CoverFile + "?v=" + strconv.FormatInt(cp.LastCaptured.Unix(), 10)
		}
		list = append(list, cp)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Status != list[j].Status {
			return list[i].Status == "running"
		}
		if list[i].Managed != list[j].Managed {
			return list[i].Managed
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]any{"projects": list, "settings": state.Settings, "dataDir": a.store.DataDir()})
}

func (a *App) handleScan(w http.ResponseWriter, r *http.Request) {
	if !mutationAllowed(w, r) {
		return
	}
	if err := a.scanOnce(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !mutationAllowed(w, r) {
		return
	}
	var in struct{ Name, Root, Command, URL string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Root = strings.TrimSpace(in.Root)
	in.Command = strings.TrimSpace(in.Command)
	in.URL = strings.TrimSpace(in.URL)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	id := model.StableID(nonEmpty(in.Root, in.Name), in.Command)
	p := model.Project{ID: id, Name: in.Name, Root: in.Root, WorkingDir: in.Root, StartCommand: in.Command, URL: in.URL, Status: "stopped", Managed: true, CreatedAt: time.Now()}
	if u, err := neturl.Parse(in.URL); err == nil {
		if port, err := strconv.Atoi(u.Port()); err == nil {
			p.Port = port
		}
	}
	if err := a.store.Upsert(p); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (a *App) handleProjectAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id, action := parts[0], parts[1]
	p, ok := a.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("project not found"))
		return
	}
	if action == "logs" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{"logs": a.processes.Logs(id)})
		return
	}
	if !mutationAllowed(w, r) {
		return
	}
	switch action {
	case "start":
		if !p.Managed {
			writeError(w, http.StatusForbidden, fmt.Errorf("keep the discovered project before starting it"))
			return
		}
		if p.Status == "running" && p.PID > 0 {
			writeError(w, http.StatusConflict, fmt.Errorf("service is already running"))
			return
		}
		pid, err := a.processes.Start(id, p.StartCommand, nonEmpty(p.WorkingDir, p.Root))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		_ = a.store.Update(id, func(x *model.Project) {
			x.PID = pid
			x.Status = "starting"
			x.Managed = true
			x.LastStarted = time.Now()
		})
		time.AfterFunc(1400*time.Millisecond, func() { _ = a.scanOnce(context.Background()) })
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "pid": pid})
	case "stop":
		if !p.Managed {
			writeError(w, http.StatusForbidden, fmt.Errorf("keep the discovered project before stopping it"))
			return
		}
		if err := a.processes.Stop(id, p.PID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		_ = a.store.Update(id, func(x *model.Project) { x.Status = "stopped"; x.PID = 0 })
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "manage":
		_ = a.store.Update(id, func(x *model.Project) { x.Managed = true })
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "capture":
		if p.URL == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("service has no detected web URL"))
			return
		}
		a.captureAsync(id, p.URL)
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
	case "forget":
		var err error
		if p.AutoDiscovered {
			err = a.store.Ignore(id)
		} else {
			err = a.store.Delete(id)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !mutationAllowed(w, r) {
		return
	}
	var settings model.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.store.UpdateSettings(settings); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *App) handleCover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	name := filepath.Base(strings.TrimPrefix(r.URL.Path, "/covers/"))
	if name == "." || name == "" || !strings.HasSuffix(strings.ToLower(name), ".png") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(a.store.CoversDir(), name)
	data, err := os.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(".png"))
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(data)
}

func mutationAllowed(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return false
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, fmt.Errorf("application/json required"))
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := neturl.Parse(origin)
		if err != nil || !sameLocalHost(u.Host, r.Host) {
			writeError(w, http.StatusForbidden, fmt.Errorf("cross-origin mutation blocked"))
			return false
		}
	}
	return true
}

func sameLocalHost(a, b string) bool {
	ah, ap, _ := net.SplitHostPort(a)
	bh, bp, _ := net.SplitHostPort(b)
	if ah == "" {
		ah = a
	}
	if bh == "" {
		bh = b
	}
	local := func(h string) bool {
		h = strings.Trim(h, "[]")
		return h == "127.0.0.1" || h == "localhost" || h == "::1"
	}
	return local(ah) && local(bh) && (ap == "" || bp == "" || ap == bp)
}

func sameProject(a, b *model.Project) bool {
	if a == nil || b == nil {
		return false
	}
	clean := func(v string) string {
		if strings.TrimSpace(v) == "" {
			return ""
		}
		return filepath.Clean(v)
	}
	aDirs := []string{clean(a.WorkingDir), clean(a.Root)}
	bDirs := []string{clean(b.WorkingDir), clean(b.Root)}
	for _, left := range aDirs {
		if left == "" {
			continue
		}
		for _, right := range bDirs {
			if right != "" && left == right {
				return true
			}
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}
func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

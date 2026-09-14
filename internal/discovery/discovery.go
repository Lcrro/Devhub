package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Lcrro/devhub/internal/model"
)

type Listener struct {
	Host    string
	Port    int
	PID     int
	Process string
}

type Scanner struct {
	client *http.Client
}

func New() *Scanner {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	return &Scanner{client: &http.Client{Timeout: 700 * time.Millisecond, Transport: tr}}
}

func (s *Scanner) Scan(ctx context.Context) ([]model.Project, error) {
	listeners, err := listListeners(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	projects := make([]model.Project, 0, len(listeners))
	for _, l := range listeners {
		key := fmt.Sprintf("%d:%d", l.PID, l.Port)
		if seen[key] || l.PID <= 0 || l.Port <= 0 {
			continue
		}
		seen[key] = true
		p := s.inspect(ctx, l)
		if p.Confidence >= 55 {
			projects = append(projects, p)
		}
	}
	return projects, nil
}

func (s *Scanner) inspect(ctx context.Context, l Listener) model.Project {
	cwd := processCWD(ctx, l.PID)
	cmdline := processCommand(ctx, l.PID)
	root := findProjectRoot(cwd)
	name, framework, startCommand := inferProject(root, cwd, cmdline)
	proc := strings.ToLower(l.Process)
	if name == "" {
		if cwd != "" {
			name = filepath.Base(cwd)
		} else {
			name = fmt.Sprintf("%s-%d", nonEmpty(l.Process, "service"), l.Port)
		}
	}

	score := 0
	if isLoopbackHost(l.Host) {
		score += 10
	}
	if root != "" {
		score += 25
	}
	if isDevRuntime(proc, cmdline) {
		score += 15
	}
	if framework != "" {
		score += 20
	}
	if isSystemService(proc) {
		score -= 80
	}

	url, ok := s.probe(ctx, l.Port)
	if ok {
		score += 30
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	identity := cwd
	if identity == "" {
		identity = root
	}
	if identity == "" {
		identity = fmt.Sprintf("%s:%d", proc, l.Port)
	}
	id := model.StableID(identity, nonEmpty(framework, proc))
	now := time.Now()
	return model.Project{
		ID: id, Name: name, Root: root, WorkingDir: cwd, StartCommand: startCommand,
		Framework: framework, Runtime: l.Process, URL: url, Host: l.Host, Port: l.Port,
		PID: l.PID, Status: "running", AutoDiscovered: true, Confidence: score,
		LastSeen: now,
	}
}

func (s *Scanner) probe(ctx context.Context, port int) (string, bool) {
	for _, scheme := range []string{"http", "https"} {
		url := fmt.Sprintf("%s://127.0.0.1:%d", scheme, port)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "DevHub/0.1")
		resp, err := s.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return url, true
		}
	}
	return "", false
}

func isLoopbackHost(host string) bool {
	h := strings.Trim(host, "[]")
	return h == "127.0.0.1" || h == "::1" || h == "localhost" || h == "0.0.0.0" || h == "::" || h == "*"
}

func isDevRuntime(proc, cmdline string) bool {
	text := strings.ToLower(proc + " " + cmdline)
	hints := []string{"node", "bun", "deno", "python", "uvicorn", "gunicorn", "go-build", "air", "cargo", "ruby", "rails", "php", "dotnet", "java"}
	for _, h := range hints {
		if strings.Contains(text, h) {
			return true
		}
	}
	return false
}

func isSystemService(proc string) bool {
	blocked := []string{"postgres", "postmaster", "redis-server", "mysqld", "mariadbd", "mongod", "memcached", "sshd", "cupsd"}
	for _, b := range blocked {
		if strings.Contains(proc, b) {
			return true
		}
	}
	return false
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func findProjectRoot(cwd string) string {
	if cwd == "" {
		return ""
	}
	dir := filepath.Clean(cwd)
	markers := []string{"package.json", "pyproject.toml", "Cargo.toml", "go.mod", "composer.json", "Gemfile", ".git"}
	for i := 0; i < 10; i++ {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

type packageJSON struct {
	Name         string            `json:"name"`
	Scripts      map[string]string `json:"scripts"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
}

func inferProject(root, cwd, cmdline string) (string, string, string) {
	if root == "" {
		return "", inferFrameworkFromCommand(cmdline), ""
	}
	name := filepath.Base(root)
	framework := inferFrameworkFromCommand(cmdline)
	start := ""
	pkgPath := filepath.Join(root, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg packageJSON
		if json.Unmarshal(data, &pkg) == nil {
			if strings.TrimSpace(pkg.Name) != "" {
				name = pkg.Name
			}
			all := map[string]string{}
			for k, v := range pkg.Dependencies {
				all[k] = v
			}
			for k, v := range pkg.DevDeps {
				all[k] = v
			}
			switch {
			case hasKey(all, "next"):
				framework = "Next.js"
			case hasKey(all, "vite"):
				framework = "Vite"
			case hasKey(all, "nuxt"):
				framework = "Nuxt"
			case hasKey(all, "astro"):
				framework = "Astro"
			case hasKey(all, "@sveltejs/kit"):
				framework = "SvelteKit"
			case hasKey(all, "react-scripts"):
				framework = "Create React App"
			}
			manager := packageManager(root)
			if _, ok := pkg.Scripts["dev"]; ok {
				start = manager + " run dev"
			}
			if start == "" {
				if _, ok := pkg.Scripts["start"]; ok {
					start = manager + " run start"
				}
			}
		}
	}
	if framework == "" {
		switch {
		case fileExists(filepath.Join(root, "pyproject.toml")):
			framework = "Python"
		case fileExists(filepath.Join(root, "Cargo.toml")):
			framework = "Rust"
		case fileExists(filepath.Join(root, "go.mod")):
			framework = "Go"
		case fileExists(filepath.Join(root, "Gemfile")):
			framework = "Ruby"
		}
	}
	return name, framework, start
}

func inferFrameworkFromCommand(cmd string) string {
	text := strings.ToLower(cmd)
	cases := []struct{ needle, name string }{
		{"next", "Next.js"}, {"vite", "Vite"}, {"nuxt", "Nuxt"}, {"astro", "Astro"},
		{"svelte", "SvelteKit"}, {"uvicorn", "FastAPI"}, {"flask", "Flask"},
		{"manage.py runserver", "Django"}, {"rails server", "Rails"},
	}
	for _, c := range cases {
		if strings.Contains(text, c.needle) {
			return c.name
		}
	}
	return ""
}

func packageManager(root string) string {
	switch {
	case fileExists(filepath.Join(root, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(root, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(root, "bun.lockb")), fileExists(filepath.Join(root, "bun.lock")):
		return "bun"
	default:
		return "npm"
	}
}

func fileExists(path string) bool                 { _, err := os.Stat(path); return err == nil }
func hasKey(m map[string]string, key string) bool { _, ok := m[key]; return ok }

var pidRE = regexp.MustCompile(`pid=(\d+)`)

func listListeners(ctx context.Context) ([]Listener, error) {
	switch runtime.GOOS {
	case "linux":
		return listLinux(ctx)
	case "darwin":
		return listDarwin(ctx)
	case "windows":
		return listWindows(ctx)
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func listLinux(ctx context.Context) ([]Listener, error) {
	out, err := exec.CommandContext(ctx, "ss", "-ltnpH").CombinedOutput()
	if err != nil {
		return listWithLsof(ctx)
	}
	var result []Listener
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		host, port := parseEndpoint(f[3])
		if port == 0 {
			continue
		}
		m := pidRE.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		pid, _ := strconv.Atoi(m[1])
		proc := processName(pid)
		result = append(result, Listener{Host: host, Port: port, PID: pid, Process: proc})
	}
	return result, nil
}

func listDarwin(ctx context.Context) ([]Listener, error) {
	return listWithLsof(ctx)
}

func listWithLsof(ctx context.Context) ([]Listener, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN").CombinedOutput()
	if err != nil {
		if len(strings.TrimSpace(string(out))) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("lsof: %w", err)
	}
	var result []Listener
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 9 {
			continue
		}
		pid, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		endpoint := f[len(f)-1]
		if endpoint == "(LISTEN)" && len(f) > 1 {
			endpoint = f[len(f)-2]
		}
		host, port := parseEndpoint(endpoint)
		if port == 0 {
			continue
		}
		result = append(result, Listener{Host: host, Port: port, PID: pid, Process: f[0]})
	}
	return result, nil
}

func listWindows(ctx context.Context) ([]Listener, error) {
	script := `Get-NetTCPConnection -State Listen | ForEach-Object { $p=Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue; Write-Output ($_.LocalAddress + "` + "`t" + `" + $_.LocalPort + "` + "`t" + `" + $_.OwningProcess + "` + "`t" + `" + $p.ProcessName) }`
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("powershell: %w", err)
	}
	var result []Listener
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) < 4 {
			continue
		}
		port, _ := strconv.Atoi(f[1])
		pid, _ := strconv.Atoi(f[2])
		if port == 0 || pid == 0 {
			continue
		}
		result = append(result, Listener{Host: f[0], Port: port, PID: pid, Process: f[3]})
	}
	return result, nil
}

func parseEndpoint(v string) (string, int) {
	v = strings.TrimSpace(v)
	idx := strings.LastIndex(v, ":")
	if idx < 0 {
		return "", 0
	}
	host := strings.Trim(v[:idx], "[]")
	port, _ := strconv.Atoi(v[idx+1:])
	return host, port
}

func processName(pid int) string {
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return "process"
}

func processCWD(ctx context.Context, pid int) string {
	switch runtime.GOOS {
	case "linux":
		v, _ := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
		return v
	case "darwin":
		out, err := exec.CommandContext(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n") {
				return strings.TrimPrefix(line, "n")
			}
		}
	}
	return ""
}

func processCommand(ctx context.Context, pid int) string {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err == nil {
			return strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
		}
	case "darwin":
		out, _ := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
		return strings.TrimSpace(string(out))
	case "windows":
		script := fmt.Sprintf(`(Get-CimInstance Win32_Process -Filter "ProcessId=%d").CommandLine`, pid)
		out, _ := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script).Output()
		return strings.TrimSpace(string(out))
	}
	return ""
}

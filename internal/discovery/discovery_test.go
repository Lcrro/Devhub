package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferViteProject(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"demo-ui","scripts":{"dev":"vite"},"devDependencies":{"vite":"^5"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte("lockfileVersion: 9"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, framework, command := inferProject(dir, dir, "node vite")
	if name != "demo-ui" || framework != "Vite" || command != "pnpm run dev" {
		t.Fatalf("got name=%q framework=%q command=%q", name, framework, command)
	}
}

func TestParseEndpoint(t *testing.T) {
	host, port := parseEndpoint("[::1]:5173")
	if host != "::1" || port != 5173 {
		t.Fatalf("got %q %d", host, port)
	}
}

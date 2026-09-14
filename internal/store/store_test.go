package store

import (
	"path/filepath"
	"testing"

	"github.com/Lcrro/devhub/internal/model"
)

func TestStorePersistsProject(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := model.Project{ID: "abc", Name: "demo", Status: "running", Port: 3000}
	if err := s.Upsert(p); err != nil {
		t.Fatal(err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("abc")
	if !ok || got.Name != "demo" || got.Port != 3000 {
		t.Fatalf("unexpected persisted project: %#v", got)
	}
	if filepath.Base(s2.CoversDir()) != "covers" {
		t.Fatalf("unexpected covers dir: %s", s2.CoversDir())
	}
}

func TestIgnorePersists(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := model.Project{ID: "ignored", Name: "noise", AutoDiscovered: true}
	if err := s.Upsert(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Ignore("ignored"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("ignored"); ok {
		t.Fatal("ignored project should be removed")
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.IsIgnored("ignored") {
		t.Fatal("ignore state did not persist")
	}
}

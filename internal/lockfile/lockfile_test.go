package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndSort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	l, err := Load(path)
	if err != nil || len(l.Entries) != 0 {
		t.Fatalf("lock inexistente deveria vir vazio: %v", err)
	}
	l.Put(Entry{Pack: "b", Agent: "cursor", Module: "skills", Files: []File{{Path: "/z"}, {Path: "/a"}}})
	l.Put(Entry{Pack: "a", Agent: "claude", Module: "skills"})
	l.Put(Entry{Pack: "b", Agent: "cursor", Module: "skills", Version: "2", Files: []File{{Path: "/z"}, {Path: "/a"}}})
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Pack != "a" || got.Entries[1].Version != "2" || got.Entries[1].Files[0].Path != "/a" {
		t.Fatalf("conteúdo inesperado: %+v", got.Entries)
	}
	if owners := got.Owners("/z"); len(owners) != 1 {
		t.Fatalf("owners = %v", owners)
	}
	got.Delete("a", "claude", "skills")
	if len(got.Entries) != 1 {
		t.Fatal("Delete falhou")
	}
}

func TestRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"version": 99, "entries": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("esperava erro de versão")
	}
}

func TestUserPath(t *testing.T) {
	if got := UserPath("/h", ""); got != "/h/.config/atlana/atlana.lock" {
		t.Fatal(got)
	}
	if got := UserPath("/h", "/x"); got != "/x/atlana/atlana.lock" {
		t.Fatal(got)
	}
}

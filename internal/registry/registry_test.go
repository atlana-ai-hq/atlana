package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddSaveLoadResolve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlana", FileName)
	r, err := Load(path)
	if err != nil || len(r.Projects) != 0 {
		t.Fatal(r, err)
	}
	if err := r.Add(Project{Name: "packs-private", Origin: "github:me/atlana-packs-private", Visibility: "private"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(Project{Name: "atlana", Origin: "github:atlana-ai-hq/atlana"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Index(string(raw), "name: atlana\n") > strings.Index(string(raw), "name: packs-private") || !strings.Contains(string(raw), "visibility: public") {
		t.Fatalf("ordem ou padrão:\n%s", raw)
	}
	r2, err := Load(path)
	if err != nil || len(r2.Projects) != 2 {
		t.Fatal(r2, err)
	}
	o, ok, err := r2.Resolve("packs-private//packs/work@main")
	if !ok || err != nil || o.String() != "github:me/atlana-packs-private//packs/work@main" {
		t.Fatalf("%v %v %v", o, ok, err)
	}
	if _, ok, _ := r2.Resolve("outro//x"); ok {
		t.Fatal("nome fora do registry")
	}
	if !r2.Remove("atlana") || r2.Remove("atlana") || len(r2.Projects) != 1 {
		t.Fatal("remove")
	}
}

func TestValidation(t *testing.T) {
	r, _ := Load(filepath.Join(t.TempDir(), "x"))
	for _, p := range []Project{
		{Name: "Bad_Name", Origin: "github:a/b"},
		{Name: "x", Origin: "gitlab:a/b"},
		{Name: "x", Origin: "github:a/b//packs"},
		{Name: "x", Origin: "github:a/b", Visibility: "secret"},
	} {
		if err := r.Add(p); err == nil {
			t.Errorf("%+v deveria ser inválido", p)
		}
	}
	if len(r.Projects) != 0 {
		t.Fatal("Add inválido não pode mudar o registry")
	}
	path := filepath.Join(t.TempDir(), FileName)
	os.WriteFile(path, []byte("apiVersion: atlana/v1\nkind: HubRegistry\nprojects:\n  - name: a\n    origin: github:a/b\n    visibility: public\n    token: abc\n"), 0o644)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("campo desconhecido (ex.: token) deveria falhar: %v", err)
	}
}

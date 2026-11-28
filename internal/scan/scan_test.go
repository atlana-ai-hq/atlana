package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadNames(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, NamesFile), "# empresas\nAcmeCorp\n\n  projeto-x  \n")
	names, err := LoadNames(dir, func(k string) string {
		if k == EnvVar {
			return "Initech, ,Globex"
		}
		return ""
	})
	if err != nil || len(names) != 4 || names[1] != "projeto-x" || names[3] != "Globex" {
		t.Fatalf("%q %v", names, err)
	}
	if n, err := LoadNames(t.TempDir(), nil); err != nil || len(n) != 0 {
		t.Fatal("sem arquivo nem variável = lista vazia")
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "README.md"), "linha ok\nfeito na acmecorp em 2025\nAcmeCorporation não conta\n")
	write(t, filepath.Join(root, "docs", "a.md"), "(Initech)\n")
	write(t, filepath.Join(root, ".git", "config"), "AcmeCorp")
	write(t, filepath.Join(root, "img.bin"), "AcmeCorp\x00\x01")
	write(t, filepath.Join(root, "packs", "work", "atlana.yaml"), "apiVersion: atlana/v1\nkind: Pack\nname: work\nversion: 0.1.0\ndescription: d\nvisibility: private\n")

	got, err := Scan(root, []string{"AcmeCorp", "Initech"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Finding{
		{File: "README.md", Line: 2, Index: 0, Name: "AcmeCorp", Kind: "nome"},
		{File: filepath.Join("docs", "a.md"), Line: 1, Index: 1, Name: "Initech", Kind: "nome"},
		{File: filepath.Join("packs", "work", "atlana.yaml"), Line: 1, Index: -1, Kind: "pack-privado"},
	}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %+v, esperado %+v", i, got[i], want[i])
		}
	}
}

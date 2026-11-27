package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	o, err := Parse("github:atlana-ai-hq/atlana//packs/hello@v0.1.0")
	if err != nil || o.Owner != "atlana-ai-hq" || o.Repo != "atlana" || o.Path != "packs/hello" || o.Ref != "v0.1.0" {
		t.Fatalf("%+v %v", o, err)
	}
	if o.String() != "github:atlana-ai-hq/atlana//packs/hello@v0.1.0" || o.RepoID() != "github:atlana-ai-hq/atlana" {
		t.Fatal(o.String())
	}
	if o, _ := Parse("github:a/b.git"); o.Repo != "b" || o.Path != "" || o.Ref != "" {
		t.Fatalf("%+v", o)
	}
	for _, bad := range []string{"github:a", "gitlab:a/b", "github:a/b//../x", "github:a/b@-x", "github:a/b@a..b", "github:a/b c"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q deveria ser inválido", bad)
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// remote cria um repo com packs/demo e devolve a URL file:// e o primeiro commit.
func remote(t *testing.T) (string, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git não está instalado")
	}
	work := t.TempDir()
	git(t, work, "init", "--quiet", "--initial-branch=main")
	os.MkdirAll(filepath.Join(work, "packs", "demo"), 0o755)
	os.WriteFile(filepath.Join(work, "packs", "demo", "v.txt"), []byte("1"), 0o644)
	git(t, work, "add", ".")
	git(t, work, "commit", "--quiet", "-m", "v1")
	git(t, work, "tag", "v1")
	first := git(t, work, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(work, "packs", "demo", "v.txt"), []byte("2"), 0o644)
	git(t, work, "commit", "--quiet", "-am", "v2")
	bare := filepath.Join(t.TempDir(), "repo.git")
	cmd := exec.Command("git", "clone", "--quiet", "--bare", work, bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	return "file://" + bare, first, work
}

func TestFetchRefsAndCache(t *testing.T) {
	url, first, work := remote(t)
	f := Fetcher{CacheDir: t.TempDir(), URLs: func(string, string) []string {
		return []string{"file:///nao/existe.git", url} // a primeira falha, a segunda serve
	}}
	read := func(o Origin) (string, Result) {
		t.Helper()
		r, err := f.Fetch(o)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(r.Dir, "v.txt"))
		return string(b), r
	}
	o, _ := Parse("github:me/repo//packs/demo")
	if v, r := read(o); v != "2" || len(r.Commit) != 40 {
		t.Fatalf("branch padrão: %s %+v", v, r)
	}
	o.Ref = "v1"
	if v, r := read(o); v != "1" || r.Commit != first || r.Pinned(o) != "github:me/repo//packs/demo@"+first {
		t.Fatalf("tag: %s %+v", v, r)
	}
	o.Ref = first[:10]
	if v, _ := read(o); v != "1" {
		t.Fatal("commit curto")
	}

	// Novo commit no remoto: o próximo fetch pega.
	os.WriteFile(filepath.Join(work, "packs", "demo", "v.txt"), []byte("3"), 0o644)
	git(t, work, "commit", "--quiet", "-am", "v3")
	git(t, work, "push", "--quiet", strings.TrimPrefix(url, "file://"), "main")
	o.Ref = "main"
	if v, _ := read(o); v != "3" {
		t.Fatalf("fetch não atualizou: %s", v)
	}

	o.Ref = "nope"
	if _, err := f.Fetch(o); err == nil || !strings.Contains(err.Error(), `ref "nope" não existe`) {
		t.Fatalf("ref inexistente: %v", err)
	}
	o.Ref, o.Path = "", "packs/outro"
	if _, err := f.Fetch(o); err == nil || !strings.Contains(err.Error(), "não existe no commit") {
		t.Fatalf("caminho inexistente: %v", err)
	}
}

func TestFetchAndAccessFailures(t *testing.T) {
	url, _, _ := remote(t)
	ok := Fetcher{CacheDir: t.TempDir(), URLs: func(string, string) []string { return []string{url} }}
	o, _ := Parse("github:me/repo")
	if err := ok.Access(o); err != nil {
		t.Fatal(err)
	}
	bad := Fetcher{CacheDir: t.TempDir(), URLs: func(string, string) []string { return []string{"file:///nao/existe.git"} }}
	if err := bad.Access(o); err == nil || !strings.Contains(err.Error(), "sem acesso") {
		t.Fatalf("Access: %v", err)
	}
	if _, err := bad.Fetch(o); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("Fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bad.CacheDir, "github", "me", "repo")); !os.IsNotExist(err) {
		t.Fatal("clone que falhou não pode deixar lixo no cache")
	}
}

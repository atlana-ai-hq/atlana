package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atlana-ai-hq/atlana/internal/lockfile"
	"github.com/atlana-ai-hq/atlana/internal/source"
)

// fakeGitHub cria um repo bare com packs/hello e faz github:<owner>/<repo>
// apontar para ele; o repo "private" não existe (simula falta de acesso).
func fakeGitHub(t *testing.T, app *App) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git não está instalado")
	}
	work := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(work, "init", "--quiet", "--initial-branch=main")
	if err := exec.Command("cp", "-R", helloPack, filepath.Join(work, "hello")).Run(); err != nil {
		t.Fatal(err)
	}
	run(work, "add", ".")
	run(work, "commit", "--quiet", "-m", "hello")
	bare := filepath.Join(t.TempDir(), "packs.git")
	run(work, "clone", "--quiet", "--bare", work, bare)
	app.Fetcher = &source.Fetcher{CacheDir: t.TempDir(), URLs: func(owner, repo string) []string {
		if repo == "packs" {
			return []string{"file://" + bare}
		}
		return []string{"file:///nao/existe/" + repo + ".git"}
	}}
}

func TestInstallFromGitHubOrigin(t *testing.T) {
	app, out, errb := newApp(t)
	fakeGitHub(t, app)
	if code := app.Run([]string{"install", "github:me/packs//hello", "--agent", "claude", "--yes"}); code != 0 {
		t.Fatalf("code=%d\n%s%s", code, out, errb)
	}
	if !strings.Contains(out.String(), "Origem: github:me/packs//hello @ ") {
		t.Fatalf("faltou a origem:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(app.Env.Home, ".claude", "skills", "conventional-commit-message", "SKILL.md")); err != nil {
		t.Fatal("skill não instalada")
	}
	lock, _ := lockfile.Load(filepath.Join(app.Env.Home, ".config", "atlana", "atlana.lock"))
	e, _ := lock.Find("hello", "claude", "skills")
	if !strings.HasPrefix(e.Source, "github:me/packs//hello@") || len(strings.TrimPrefix(e.Source, "github:me/packs//hello@")) != 40 {
		t.Fatalf("o lock precisa da origem com o commit exato: %q", e.Source)
	}

	if code := app.Run([]string{"install", "github:me/packs//hello@nope", "--agent", "claude", "--yes"}); code != 1 || !strings.Contains(errb.String(), `ref "nope" não existe`) {
		t.Fatalf("ref inexistente: code=%d %s", code, errb)
	}
}

func TestRegistryCommands(t *testing.T) {
	app, out, errb := newApp(t)
	fakeGitHub(t, app)
	run := func(args ...string) int {
		t.Helper()
		out.Reset()
		errb.Reset()
		return app.Run(args)
	}
	if run("registry", "list") != 0 || !strings.Contains(out.String(), "Registry vazio") {
		t.Fatalf("vazio: %s", out)
	}
	if run("registry", "add", "mine", "github:me/packs", "--private") != 0 {
		t.Fatal(errb.String())
	}
	if run("registry", "add", "work", "github:me/private", "--private") != 0 || run("registry", "add", "atlana", "github:atlana-ai-hq/atlana") != 0 {
		t.Fatal(errb.String())
	}
	if run("registry", "add", "bad", "github:me/x//path") != 2 {
		t.Fatal("origem com caminho deveria ser recusada")
	}

	run("registry", "list")
	got := out.String()
	if !strings.Contains(got, "mine") || !strings.Contains(got, "atlana") || strings.Contains(got, "work") || !strings.Contains(got, "1 projeto(s) privado(s) escondido(s)") {
		t.Fatalf("list deveria esconder o privado sem acesso:\n%s", got)
	}
	run("registry", "list", "--all")
	if !strings.Contains(out.String(), "sem acesso") || !strings.Contains(out.String(), "work") {
		t.Fatalf("--all:\n%s", out)
	}

	if code := run("install", "mine//hello", "--agent", "cursor", "--yes"); code != 0 {
		t.Fatalf("install pelo nome: %d %s %s", code, out, errb)
	}
	if code := run("install", "naoexiste//x", "--agent", "cursor", "--yes"); code != 1 || !strings.Contains(errb.String(), "registry list") {
		t.Fatalf("nome desconhecido: %d %s", code, errb)
	}
	if run("registry", "remove", "mine") != 0 || run("registry", "remove", "mine") != 1 {
		t.Fatal("remove")
	}
	if raw, _ := os.ReadFile(filepath.Join(app.Env.Home, ".config", "atlana", "hub.yaml")); strings.Contains(string(raw), "mine") || !strings.Contains(string(raw), "kind: HubRegistry") {
		t.Fatalf("hub.yaml:\n%s", raw)
	}
}

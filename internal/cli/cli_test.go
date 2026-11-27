package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atlana-ai-hq/atlana/internal/adapter"
)

var helloPack, _ = filepath.Abs(filepath.Join("..", "..", "packs", "hello"))

func newApp(t *testing.T, bins ...string) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	have := map[string]bool{}
	for _, b := range bins {
		have[b] = true
	}
	var out, errb bytes.Buffer
	return &App{
		Stdout: &out, Stderr: &errb, Stdin: strings.NewReader(""),
		Env: adapter.Env{
			Home: t.TempDir(), WorkDir: t.TempDir(),
			LookPath: func(b string) (string, error) {
				if have[b] {
					return "/usr/bin/" + b, nil
				}
				return "", errors.New("x")
			},
			Getenv: func(string) string { return "" },
		},
		Now: func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

func TestVersion(t *testing.T) {
	app, out, _ := newApp(t)
	if code := app.Run([]string{"version"}); code != 0 || !strings.Contains(out.String(), "atlana/v1alpha1") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestUnknownCommand(t *testing.T) {
	app, _, _ := newApp(t)
	if code := app.Run([]string{"nope"}); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestDoctor(t *testing.T) {
	app, out, _ := newApp(t, "cursor-agent")
	if code := app.Run([]string{"doctor"}); code != 0 {
		t.Fatal(code)
	}
	for _, want := range []string{"Cursor", "Claude Code", "Codex CLI", "Gemini CLI", "Grok Bot", "não (só exporta)", "/usr/bin/cursor-agent"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor sem %q:\n%s", want, out)
		}
	}
}

func TestValidateRepoPacks(t *testing.T) {
	app, out, errb := newApp(t)
	if code := app.Run([]string{"validate", filepath.Dir(helloPack)}); code != 0 {
		t.Fatalf("code=%d\n%s%s", code, out, errb)
	}
}

func TestValidateInvalid(t *testing.T) {
	app, out, _ := newApp(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "atlana.yaml"), []byte("apiVersion: atlana/v1alpha1\nkind: Pack\nname: X\n"), 0o644)
	if code := app.Run([]string{"validate", dir}); code != 1 || !strings.Contains(out.String(), "FALHA") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestInstallDryRunThenApplyThenUninstall(t *testing.T) {
	app, out, errb := newApp(t)
	skill := filepath.Join(app.Env.Home, ".cursor", "skills", "conventional-commit-message", "SKILL.md")

	// flags depois do posicional, como no README.
	code := app.Run([]string{"install", helloPack, "--only", "skills", "--agent", "cursor,claude", "--dry-run"})
	if code != 0 || !strings.Contains(out.String(), "--dry-run: nada foi alterado") {
		t.Fatalf("dry-run code=%d\n%s%s", code, out, errb)
	}
	if _, err := os.Stat(skill); !os.IsNotExist(err) {
		t.Fatal("dry-run escreveu no disco")
	}

	// Sem terminal e sem --yes: recusa.
	if code := app.Run([]string{"install", helloPack, "--agent", "cursor"}); code != 2 {
		t.Fatalf("sem --yes deveria falhar com 2, veio %d", code)
	}

	out.Reset()
	if code := app.Run([]string{"install", "--agent", "cursor,claude", "--yes", helloPack}); code != 0 {
		t.Fatalf("install code=%d\n%s%s", code, out, errb)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatal("skill não instalada")
	}

	out.Reset()
	app.Run([]string{"status"})
	if !strings.Contains(out.String(), "hello") || strings.Count(out.String(), " ok") != 2 {
		t.Fatalf("status inesperado:\n%s", out)
	}

	if code := app.Run([]string{"uninstall", "hello", "--yes"}); code != 0 {
		t.Fatalf("uninstall code=%d %s", code, errb)
	}
	if _, err := os.Stat(skill); !os.IsNotExist(err) {
		t.Fatal("uninstall não removeu")
	}
}

func TestInstallGrokPrintsExportDir(t *testing.T) {
	app, out, errb := newApp(t)
	root := filepath.Join(app.Env.Home, ".config", "atlana", "exports", "grok", "skills")
	if code := app.Run([]string{"install", helloPack, "--agent", "grok", "--yes"}); code != 0 {
		t.Fatalf("code=%d\n%s%s", code, out, errb)
	}
	if _, err := os.Stat(filepath.Join(root, "conventional-commit-message", "SKILL.md")); err != nil {
		t.Fatal("skill não exportada")
	}
	if !strings.Contains(out.String(), "skills exportadas em "+root) {
		t.Fatalf("faltou aviso de exportação:\n%s", out)
	}
}

func TestInteractiveConfirm(t *testing.T) {
	app, out, _ := newApp(t)
	app.Interactive = true
	app.Stdin = strings.NewReader("n\n")
	app.Run([]string{"install", helloPack, "--agent", "cursor"})
	if !strings.Contains(out.String(), "Cancelado") {
		t.Fatalf("esperava cancelamento:\n%s", out)
	}
	app.Stdin = strings.NewReader("s\n")
	if code := app.Run([]string{"install", helloPack, "--agent", "cursor"}); code != 0 {
		t.Fatal(code)
	}
	if _, err := os.Stat(filepath.Join(app.Env.Home, ".cursor", "skills", "conventional-commit-message", "SKILL.md")); err != nil {
		t.Fatal("confirmação 's' deveria instalar")
	}
}

func TestInstallNoAgentsDetected(t *testing.T) {
	app, _, errb := newApp(t)
	if code := app.Run([]string{"install", helloPack, "--dry-run"}); code != 1 || !strings.Contains(errb.String(), "nenhum agente detectado") {
		t.Fatalf("code=%d err=%s", code, errb)
	}
}

func TestInstallBadFlags(t *testing.T) {
	app, _, _ := newApp(t)
	for _, args := range [][]string{
		{"install"},
		{"install", helloPack, "--only", "plugins", "--agent", "cursor"},
		{"install", helloPack, "--agent", "vim"},
		{"install", helloPack, "--scope", "global", "--agent", "cursor"},
	} {
		if code := app.Run(args); code != 2 {
			t.Errorf("%v: code=%d, esperado 2", args, code)
		}
	}
}

func TestScanCommand(t *testing.T) {
	app, out, errb := newApp(t)
	root := app.Env.WorkDir
	os.WriteFile(filepath.Join(root, "README.md"), []byte("feito na AcmeCorp\n"), 0o644)
	if code := app.Run([]string{"scan"}); code != 0 || !strings.Contains(out.String(), "nenhum nome privado configurado") {
		t.Fatalf("sem lista: %d %s %s", code, out, errb)
	}
	cfg := filepath.Join(app.Env.Home, ".config", "atlana")
	os.MkdirAll(cfg, 0o755)
	os.WriteFile(filepath.Join(cfg, "private-names.txt"), []byte("AcmeCorp\n"), 0o644)
	out.Reset()
	if code := app.Run([]string{"scan", "."}); code != 1 || !strings.Contains(out.String(), "README.md:1: nome privado #1 da lista") || strings.Contains(out.String(), "AcmeCorp") {
		t.Fatalf("o relatório padrão não pode expor o nome: %d\n%s", code, out)
	}
	out.Reset()
	app.Run([]string{"scan", "--show"})
	if !strings.Contains(out.String(), `"AcmeCorp"`) {
		t.Fatalf("--show:\n%s", out)
	}
}

func TestRepoHasNothingPrivate(t *testing.T) {
	app, out, errb := newApp(t)
	repo, _ := filepath.Abs(filepath.Join("..", ".."))
	if code := app.Run([]string{"scan", repo}); code != 0 {
		t.Fatalf("o repo tem pack privado: %d\n%s%s", code, out, errb)
	}
}

func TestBuildCommand(t *testing.T) {
	app, out, errb := newApp(t)
	if code := app.Run([]string{"build", helloPack, "--target", "claude-plugin,agent-plugin"}); code != 0 {
		t.Fatalf("code=%d\n%s%s", code, out, errb)
	}
	for _, p := range []string{
		filepath.Join("dist", "claude-plugin", "hello", ".claude-plugin", "plugin.json"),
		filepath.Join("dist", "claude-plugin", "hello", "skills", "conventional-commit-message", "SKILL.md"),
		filepath.Join("dist", "agent-plugin", "hello", "plugin.json"),
	} {
		if _, err := os.Stat(filepath.Join(app.Env.WorkDir, p)); err != nil {
			t.Errorf("faltou %s", p)
		}
	}
	if code := app.Run([]string{"build", helloPack, "--target", "vscode"}); code != 2 {
		t.Fatalf("target inválido: %d", code)
	}
	if code := app.Run([]string{"build", helloPack}); code != 2 {
		t.Fatalf("sem --target: %d", code)
	}
}

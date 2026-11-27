package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

func fakeEnv(t *testing.T, bins ...string) Env {
	t.Helper()
	have := map[string]bool{}
	for _, b := range bins {
		have[b] = true
	}
	return Env{
		Home:    t.TempDir(),
		WorkDir: t.TempDir(),
		LookPath: func(b string) (string, error) {
			if have[b] {
				return "/usr/bin/" + b, nil
			}
			return "", errors.New("not found")
		},
		Getenv: func(string) string { return "" },
	}
}

func TestDetect(t *testing.T) {
	env := fakeEnv(t, "claude")
	if err := os.Mkdir(filepath.Join(env.Home, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"claude": true, "gemini": true, "cursor": false, "codex": false, "grok": false}
	for _, ad := range All() {
		if got := ad.Detect(env).Installed; got != want[ad.ID()] {
			t.Errorf("%s: Installed = %v, esperado %v", ad.ID(), got, want[ad.ID()])
		}
	}
}

func TestCodexHome(t *testing.T) {
	env := fakeEnv(t)
	custom := t.TempDir()
	env.Getenv = func(k string) string {
		if k == "CODEX_HOME" {
			return custom
		}
		return ""
	}
	ad, _ := Get("codex")
	if !ad.Detect(env).Installed {
		t.Fatal("deveria detectar via CODEX_HOME")
	}
}

func TestSkillsRoot(t *testing.T) {
	env := fakeEnv(t)
	cases := []struct {
		id    string
		scope Scope
		want  string
	}{
		{"cursor", ScopeUser, filepath.Join(env.Home, ".cursor", "skills")},
		{"cursor", ScopeProject, filepath.Join(env.WorkDir, ".cursor", "skills")},
		{"claude", ScopeUser, filepath.Join(env.Home, ".claude", "skills")},
		{"claude", ScopeProject, filepath.Join(env.WorkDir, ".claude", "skills")},
		{"gemini", ScopeUser, filepath.Join(env.Home, ".gemini", "skills")},
		{"gemini", ScopeProject, filepath.Join(env.WorkDir, ".gemini", "skills")},
		{"grok", ScopeUser, filepath.Join(env.Home, ".config", "atlana", "exports", "grok", "skills")},
		{"grok", ScopeProject, filepath.Join(env.WorkDir, ".atlana", "exports", "grok", "skills")},
	}
	for _, c := range cases {
		ad, err := Get(c.id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ad.SkillsRoot(c.scope, env)
		if err != nil || got != c.want {
			t.Errorf("%s/%s = %q, %v; esperado %q", c.id, c.scope, got, err, c.want)
		}
	}
	for _, id := range []string{"codex"} {
		ad, _ := Get(id)
		if ad.Supports(manifest.ModuleSkills) {
			t.Errorf("%s ainda não deveria suportar skills", id)
		}
		if _, err := ad.SkillsRoot(ScopeUser, env); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: esperava ErrUnsupported", id)
		}
	}
}

func TestGrok(t *testing.T) {
	ad, err := Get("grok")
	if err != nil {
		t.Fatal(err)
	}
	if !IsExporter(ad) || !ad.Supports(manifest.ModuleSkills) || ad.Supports(manifest.ModuleMCP) {
		t.Fatal("grok deveria só exportar skills")
	}
	for _, id := range []string{"cursor", "claude"} {
		if a, _ := Get(id); IsExporter(a) {
			t.Errorf("%s não deveria ser exportador", id)
		}
	}

	env := fakeEnv(t, "grok")
	if d := ad.Detect(env); !d.Installed || d.Evidence[0] != "binário /usr/bin/grok" {
		t.Errorf("detecção pelo binário falhou: %+v", d)
	}
	env = fakeEnv(t)
	if err := os.Mkdir(filepath.Join(env.Home, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !ad.Detect(env).Installed {
		t.Error("detecção por ~/.grok falhou")
	}

	xdg := t.TempDir()
	env.Getenv = func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return xdg
		}
		return ""
	}
	if got, _ := ad.SkillsRoot(ScopeUser, env); got != filepath.Join(xdg, "atlana", "exports", "grok", "skills") {
		t.Errorf("XDG_CONFIG_HOME ignorado: %s", got)
	}
	if _, err := ad.SkillsRoot(ScopeUser, Env{}); err == nil {
		t.Error("HOME vazio deveria dar erro")
	}
}

func TestGetUnknown(t *testing.T) {
	if _, err := Get("vim"); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestParseScope(t *testing.T) {
	if _, err := ParseScope("global"); err == nil {
		t.Fatal("esperava erro")
	}
}

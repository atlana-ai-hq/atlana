package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	want := map[string]bool{"claude": true, "gemini": true, "cursor": false, "codex": false, "grok": false, "crush": false, "opencode": false}
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
		{"codex", ScopeUser, filepath.Join(env.Home, ".agents", "skills")},
		{"codex", ScopeProject, filepath.Join(env.WorkDir, ".agents", "skills")},
		{"gemini", ScopeUser, filepath.Join(env.Home, ".gemini", "skills")},
		{"gemini", ScopeProject, filepath.Join(env.WorkDir, ".gemini", "skills")},
		{"grok", ScopeUser, filepath.Join(env.Home, ".config", "atlana", "exports", "grok", "skills")},
		{"grok", ScopeProject, filepath.Join(env.WorkDir, ".atlana", "exports", "grok", "skills")},
		{"crush", ScopeUser, filepath.Join(env.Home, ".config", "crush", "skills")},
		{"crush", ScopeProject, filepath.Join(env.WorkDir, ".crush", "skills")},
		{"opencode", ScopeUser, filepath.Join(env.Home, ".config", "opencode", "skills")},
		{"opencode", ScopeProject, filepath.Join(env.WorkDir, ".opencode", "skills")},
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

func TestCrushAndOpenCode(t *testing.T) {
	env := fakeEnv(t, "crush")
	xdg := t.TempDir()
	vars := map[string]string{"XDG_CONFIG_HOME": xdg}
	env.Getenv = func(k string) string { return vars[k] }

	cr, _ := Get("crush")
	if d := cr.Detect(env); !d.Installed {
		t.Error("crush pelo binário")
	}
	if got, _ := cr.SkillsRoot(ScopeUser, env); got != filepath.Join(xdg, "crush", "skills") {
		t.Errorf("XDG: %s", got)
	}
	vars["CRUSH_SKILLS_DIR"] = "/skills/crush"
	if got, _ := cr.SkillsRoot(ScopeUser, env); got != "/skills/crush" {
		t.Errorf("CRUSH_SKILLS_DIR: %s", got)
	}

	oc, _ := Get("opencode")
	if oc.Detect(env).Installed {
		t.Fatal("opencode não está instalado")
	}
	if err := os.Mkdir(filepath.Join(env.Home, ".opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !oc.Detect(env).Installed {
		t.Error("opencode por ~/.opencode (instalador oficial)")
	}
	if got, _ := oc.SkillsRoot(ScopeUser, env); got != filepath.Join(xdg, "opencode", "skills") {
		t.Errorf("opencode XDG: %s", got)
	}
	for _, m := range manifest.AllModules {
		if cr.Supports(m) != (m == manifest.ModuleSkills) {
			t.Errorf("crush: Supports(%s)", m)
		}
		want := m == manifest.ModuleSkills || m == manifest.ModuleMCP || m == manifest.ModuleAgents
		if oc.Supports(m) != want {
			t.Errorf("opencode: Supports(%s)", m)
		}
	}
}

func TestOpenCodeMCP(t *testing.T) {
	env := fakeEnv(t)
	ad, _ := Get("opencode")
	ma := ad.(MCPAdapter)
	cf, _ := ma.MCPConfig(ScopeUser, env)
	if cf.Path != filepath.Join(env.Home, ".config", "opencode", "opencode.json") || cf.Section != "mcp" {
		t.Fatalf("config: %+v", cf)
	}
	dir := filepath.Join(env.Home, ".config", "opencode")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte("{}"), 0o644)
	if cf, _ := ma.MCPConfig(ScopeUser, env); cf.Path != filepath.Join(dir, "opencode.jsonc") {
		t.Fatalf("jsonc existente tem prioridade: %s", cf.Path)
	}
	if cf, _ := ma.MCPConfig(ScopeProject, env); cf.Path != filepath.Join(env.WorkDir, "opencode.json") {
		t.Fatalf("project: %s", cf.Path)
	}

	local, err := ma.MCPServer(map[string]any{"command": "${PLUGIN_ROOT}/bin/srv", "args": []any{"--x"}, "env": map[string]any{"TOKEN": "${TOKEN}"}}, "/pack")
	if err != nil {
		t.Fatal(err)
	}
	cmd := local["command"].([]any)
	if local["type"] != "local" || cmd[0] != "/pack/bin/srv" || cmd[1] != "--x" || local["environment"].(map[string]any)["TOKEN"] != "{env:TOKEN}" {
		t.Fatalf("local: %v", local)
	}
	if _, ok := local["env"]; ok {
		t.Fatal("env vira environment")
	}
	remote, _ := ma.MCPServer(map[string]any{"type": "http", "url": "https://x/mcp", "headers": map[string]any{"Authorization": "Bearer ${T}"}}, "/pack")
	if remote["type"] != "remote" || remote["headers"].(map[string]any)["Authorization"] != "Bearer {env:T}" {
		t.Fatalf("remote: %v", remote)
	}
	if _, err := ma.MCPServer(map[string]any{"command": "x", "cwd": "./a"}, "/pack"); err == nil {
		t.Fatal("cwd não tem equivalente")
	}
}

func TestOpenCodeSubagent(t *testing.T) {
	env := fakeEnv(t)
	ad, _ := Get("opencode")
	aa := ad.(AgentsAdapter)
	if root, _ := aa.AgentsRoot(ScopeUser, env); root != filepath.Join(env.Home, ".config", "opencode", "agents") {
		t.Fatalf("user: %s", root)
	}
	if root, _ := aa.AgentsRoot(ScopeProject, env); root != filepath.Join(env.WorkDir, ".opencode", "agents") {
		t.Fatalf("project: %s", root)
	}
	file, b, notes, err := aa.Subagent(manifest.Subagent{Name: "rev", Description: "Revisa.", Tools: []string{"read", "search", "shell:readonly"}, Model: "fast", Body: "Revise.\n"})
	if err != nil || file != "rev.md" {
		t.Fatal(file, err)
	}
	got := string(b)
	for _, want := range []string{"description: Revisa.", "mode: subagent", "bash: ask", "edit: deny", "webfetch: deny", "Revise."} {
		if !strings.Contains(got, want) {
			t.Errorf("sem %q:\n%s", want, got)
		}
	}
	if len(notes) != 3 { // shell:readonly, mcp e model
		t.Errorf("notas: %v", notes)
	}
	_, b, _, _ = aa.Subagent(manifest.Subagent{Name: "livre", Description: "Tudo.", Model: "inherit", Body: "x"})
	if strings.Contains(string(b), "permission") {
		t.Errorf("sem tools = sem restrição:\n%s", b)
	}
}

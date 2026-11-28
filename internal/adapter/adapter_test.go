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
	// Só ~/.gemini: é do Antigravity e do Gemini CLI, não prova nenhum dos dois.
	want := map[string]bool{"claude": true}
	for _, ad := range All() {
		if got := ad.Detect(env).Installed; got != want[ad.ID()] {
			t.Errorf("%s: Installed = %v, esperado %v", ad.ID(), got, want[ad.ID()])
		}
	}
	os.WriteFile(filepath.Join(env.Home, ".gemini", "settings.json"), []byte("{}"), 0o644)
	os.Mkdir(filepath.Join(env.Home, ".gemini", "antigravity-cli"), 0o755)
	for _, id := range []string{"gemini", "antigravity"} {
		ad, _ := Get(id)
		if !ad.Detect(env).Installed {
			t.Errorf("%s deveria ser detectado", id)
		}
	}
	env2 := fakeEnv(t, "agy")
	if ad, _ := Get("antigravity"); !ad.Detect(env2).Installed {
		t.Error("antigravity pelo binário agy")
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
		if !oc.Supports(m) {
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

func TestOpenCodeToolCategoryAndPlugin(t *testing.T) {
	ad, _ := Get("opencode")
	pa := ad.(PluginHooksAdapter)
	for native, want := range map[string]string{"bash": "shell", "edit": "edit", "apply_patch": "edit", "write": "write", "grep": "read", "webfetch": "", "github_create_issue": "mcp", "desconhecida": ""} {
		if got := pa.ToolCategory(native); got != want {
			t.Errorf("%s: %q, esperado %q", native, got, want)
		}
	}
	if _, ok := pa.HookEvent("prompt_submit"); ok {
		t.Error("prompt_submit não tem formato verificado no OpenCode")
	}
	file, js := pa.HookPlugin("meu-pack", `/opt/atlana "x"`, "/h/opencode", []string{"pre_tool", "session_start"})
	got := string(js)
	if file != "atlana-meu-pack.js" || !strings.Contains(got, "export const AtlanaHooks_meu_pack") || !strings.Contains(got, `const EXE = "/opt/atlana \"x\""`) {
		t.Fatalf("plugin:\n%s", got)
	}
	if !strings.Contains(got, "session.created") || strings.Contains(got, "session.idle") || strings.Contains(got, "tool.execute.after") {
		t.Fatalf("só os eventos usados:\n%s", got)
	}
}

func TestCommands(t *testing.T) {
	c := manifest.Command{Name: "commit", Description: "Escreve a mensagem de commit.", ArgumentHint: "[escopo]", Body: "Escreva um commit para $ARGUMENTS.\nUse \"aspas\" e <tags> & \\barras.\n"}
	env := fakeEnv(t)
	cases := []struct {
		id, root, file string
		want           []string
	}{
		{"claude", filepath.Join(env.WorkDir, ".claude", "commands"), "commit.md", []string{"description: Escreve a mensagem de commit.", "argument-hint: '[escopo]'", "para $ARGUMENTS."}},
		{"opencode", filepath.Join(env.WorkDir, ".opencode", "commands"), "commit.md", []string{"description: Escreve a mensagem de commit.", "para $ARGUMENTS."}},
		{"cursor", filepath.Join(env.WorkDir, ".cursor", "commands"), "commit.md", []string{"Escreva um commit"}},
		{"gemini", filepath.Join(env.WorkDir, ".gemini", "commands"), "commit.toml", []string{`description = "Escreve a mensagem de commit."`, `para {{args}}.\nUse \"aspas\" e <tags> & \\barras.\n"`}},
	}
	for _, tc := range cases {
		ad, _ := Get(tc.id)
		ca := ad.(CommandsAdapter)
		if root, _ := ca.CommandsRoot(ScopeProject, env); root != tc.root {
			t.Errorf("%s: root %s", tc.id, root)
		}
		file, b, notes, err := ca.Command(c)
		if err != nil || file != tc.file {
			t.Fatalf("%s: %s %v", tc.id, file, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s: sem %q:\n%s", tc.id, w, b)
			}
		}
		if tc.id == "cursor" && (len(notes) != 1 || strings.HasPrefix(string(b), "---")) {
			t.Errorf("cursor: Markdown puro com nota sobre $ARGUMENTS: %v\n%s", notes, b)
		}
		if tc.id == "opencode" && strings.Contains(string(b), "argument-hint") {
			t.Error("opencode não tem argument-hint")
		}
	}
	ad, _ := Get("opencode")
	if root, _ := ad.(CommandsAdapter).CommandsRoot(ScopeUser, env); root != filepath.Join(env.Home, ".config", "opencode", "commands") {
		t.Errorf("opencode user: %s", root)
	}
	for _, id := range []string{"codex", "crush", "grok"} {
		ad, _ := Get(id)
		if ad.Supports(manifest.ModuleCommands) {
			t.Errorf("%s não instala commands", id)
		}
	}
}

func TestAntigravity(t *testing.T) {
	env := fakeEnv(t)
	ad, _ := Get("antigravity")
	a := ad.(antigravity)
	for _, c := range []struct {
		scope         Scope
		skills, rules string
	}{
		{ScopeUser, filepath.Join(env.Home, ".gemini", "config", "skills"), filepath.Join(env.Home, ".gemini", "config", "rules")},
		{ScopeProject, filepath.Join(env.WorkDir, ".agents", "skills"), filepath.Join(env.WorkDir, ".agents", "rules")},
	} {
		if r, _ := a.SkillsRoot(c.scope, env); r != c.skills {
			t.Errorf("skills %s: %s", c.scope, r)
		}
		if r, _ := a.RulesRoot(c.scope, env); r != c.rules {
			t.Errorf("rules %s: %s", c.scope, r)
		}
	}
	out, _, err := a.Rules("demo", []RuleFile{{Rel: "AGENTS.md", Content: []byte("---\nx: 1\n---\nSeja breve.\n")}, {Rel: "docs/estilo.md", Content: []byte("Estilo.\n")}}, ScopeProject, env)
	if err != nil || len(out) != 2 {
		t.Fatal(err)
	}
	if out[0].Path != filepath.Join(env.WorkDir, ".agents", "rules", "atlana-demo-AGENTS.md") || string(out[0].Content) != "---\ntrigger: always_on\n---\n\nSeja breve.\n" {
		t.Errorf("rule: %s\n%s", out[0].Path, out[0].Content)
	}
	if filepath.Base(out[1].Path) != "atlana-demo-docs-estilo.md" {
		t.Errorf("rule aninhada: %s", out[1].Path)
	}

	cfg, err := a.MCPConfig(ScopeUser, env)
	if err != nil || cfg.Path != filepath.Join(env.Home, ".gemini", "config", "mcp_config.json") || cfg.Section != "mcpServers" {
		t.Fatalf("%+v %v", cfg, err)
	}
	if _, err := a.MCPConfig(ScopeProject, env); !errors.Is(err, ErrUnsupported) {
		t.Errorf("mcp project: %v", err)
	}
	s, err := a.MCPServer(map[string]any{"command": "${PLUGIN_ROOT}/srv", "env": map[string]any{"GITHUB_TOKEN": "${GITHUB_TOKEN}", "MODE": "ro"}}, "/pack")
	if err != nil || s["command"] != "/pack/srv" || len(s["env"].(map[string]any)) != 1 || s["env"].(map[string]any)["MODE"] != "ro" {
		t.Errorf("stdio: %v %v", s, err)
	}
	s, err = a.MCPServer(map[string]any{"command": "srv", "env": map[string]any{"T": "${T}"}}, "/pack")
	if _, has := s["env"]; err != nil || has {
		t.Errorf("env só com repasse some: %v %v", s, err)
	}
	for _, bad := range []map[string]any{
		{"url": "https://x/mcp", "headers": map[string]any{"Authorization": "Bearer ${TOK}"}},
		{"command": "srv", "env": map[string]any{"A": "${B}"}},
		{"command": "srv", "env": map[string]any{"A": "x-${A}"}},
		{"type": "sse", "url": "https://x/sse"},
	} {
		if _, err := a.MCPServer(bad, "/pack"); err == nil {
			t.Errorf("deveria recusar %v", bad)
		}
	}
	if s, err := a.MCPServer(map[string]any{"type": "http", "url": "https://x/mcp"}, "/pack"); err != nil || s["url"] != "https://x/mcp" || s["type"] != nil {
		t.Errorf("http: %v %v", s, err)
	}

	file, b, _, err := a.Command(manifest.Command{Name: "resumir", Description: "Resume.", Body: "Foque em $ARGUMENTS.\n"})
	if err != nil || file != filepath.Join("resumir", "SKILL.md") {
		t.Fatal(file, err)
	}
	for _, w := range []string{"name: resumir", "description: Resume.", "[o texto que a pessoa digitou depois de /resumir]"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("command-skill sem %q:\n%s", w, b)
		}
	}
}

func TestAntigravityAgentsAndHooks(t *testing.T) {
	env := fakeEnv(t)
	a := antigravity{}
	if r, _ := a.AgentsRoot(ScopeProject, env); r != filepath.Join(env.WorkDir, ".agents", "agents") {
		t.Errorf("agents root: %s", r)
	}
	file, b, notes, err := a.Subagent(manifest.Subagent{Name: "revisor", Description: "Revisa.", Tools: []string{"read", "search", "shell:readonly"}, Model: "fast", Body: "Revise o diff.\n"})
	if err != nil || file != filepath.Join("revisor", "agent.md") {
		t.Fatal(file, err)
	}
	for _, w := range []string{"subagent: true", "mainAgent: false", "tools:\n    - read_file\n", "# revisor\n\nRevise o diff."} {
		if !strings.Contains(string(b), w) {
			t.Errorf("sem %q:\n%s", w, b)
		}
	}
	if strings.Contains(string(b), "run_command") || len(notes) != 2 {
		t.Errorf("shell:readonly fica de fora e model avisa: %v\n%s", notes, b)
	}
	_, b, _, _ = a.Subagent(manifest.Subagent{Name: "tudo", Description: "Tudo.", Model: "inherit", Body: "# Título\n\nCorpo.\n"})
	if !strings.Contains(string(b), "- search_web") || strings.Contains(string(b), "# tudo") {
		t.Errorf("sem tools = lista completa, H1 mantido:\n%s", b)
	}

	cfg, _ := a.HooksConfig(ScopeUser, env)
	if cfg.Path != filepath.Join(env.Home, ".gemini", "config", "hooks.json") || cfg.Section != "atlana" {
		t.Errorf("%+v", cfg)
	}
	if _, ok := a.HookEvent("prompt_submit"); ok {
		t.Error("prompt_submit não tem equivalente fiel")
	}
	if e := a.HookEntry("PreToolUse", "x"); e["matcher"] != "*" || e["hooks"] == nil {
		t.Errorf("tool: %v", e)
	}
	if e := a.HookEntry("Stop", "x"); e["command"] != "x" || e["hooks"] != nil {
		t.Errorf("flat: %v", e)
	}
	if got := string(AntigravityReply("pre_tool", 2, "não edite .plan\n")); got != `{"decision":"deny","reason":"não edite .plan"}` {
		t.Errorf("deny: %s", got)
	}
	for _, c := range []struct {
		ev   string
		code int
	}{{"pre_tool", 0}, {"stop", 2}, {"post_tool", 2}} {
		if got := string(AntigravityReply(c.ev, c.code, "x")); got != "{}" {
			t.Errorf("%v: %s (nunca allow)", c, got)
		}
	}
}

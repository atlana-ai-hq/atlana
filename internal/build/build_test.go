package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// pack completo: skill, rule, subagent, hook, dois servidores MCP e um bin/ de apoio.
func pack(t *testing.T) *manifest.Pack {
	t.Helper()
	d := t.TempDir()
	write(t, filepath.Join(d, "atlana.yaml"), `apiVersion: atlana/v1
kind: Pack
name: demo
version: 1.2.0
description: Pack de teste.
license: MIT
authors: [alguem]
modules:
  rules: [AGENTS.md]
  skills: [hi]
  agents: [planner]
  hooks: [guard]
  mcp: [local, secret]
  commands: [oi]
requires:
  env: [TOKEN]
`, 0o644)
	write(t, filepath.Join(d, "AGENTS.md"), "# Regras\n", 0o644)
	write(t, filepath.Join(d, "skills", "hi", "SKILL.md"), "---\nname: hi\ndescription: Diz oi.\n---\noi\n", 0o644)
	write(t, filepath.Join(d, "agents", "planner.md"), "---\nname: planner\ndescription: Planeja.\ntools: [read]\n---\n\nPlaneje.\n", 0o644)
	write(t, filepath.Join(d, "hooks", "guard", "hook.yaml"), "name: guard\nevent: pre_tool\nmatch:\n  tool: [edit]\nrun: ./run.sh\nfallback: rule\n", 0o644)
	write(t, filepath.Join(d, "hooks", "guard", "run.sh"), "#!/bin/sh\nexit 2\n", 0o755)
	write(t, filepath.Join(d, "hooks", "guard", "rule.md"), "Não edite.\n", 0o644)
	write(t, filepath.Join(d, "bin", "srv"), "#!/bin/sh\n", 0o755)
	write(t, filepath.Join(d, "commands", "oi.md"), "---\ndescription: Diz oi.\n---\nOi, $ARGUMENTS.\n", 0o644)
	write(t, filepath.Join(d, "mcp.json"), `{"mcpServers": {
  "local": {"command": "${PLUGIN_ROOT}/bin/srv", "args": ["--root", "${PLUGIN_ROOT}"]},
  "secret": {"command": "npx", "env": {"TOKEN": "${TOKEN}"}}
}}`, 0o644)
	p, rep, err := manifest.Load(d)
	if err != nil {
		t.Fatalf("%v %v", err, rep.Issues)
	}
	return p
}

func files(r *Result) map[string]File {
	m := map[string]File{}
	for _, f := range r.Files {
		m[f.Path] = f
	}
	return m
}

func jsonOf(t *testing.T, f File) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(f.Content, &v); err != nil {
		t.Fatalf("%s: %v\n%s", f.Path, err, f.Content)
	}
	return v
}

func notes(r *Result) string { return strings.Join(r.Notes, "\n") }

func TestClaudePlugin(t *testing.T) {
	r, err := Build(pack(t), ClaudePlugin, nil)
	if err != nil {
		t.Fatal(err)
	}
	fs := files(r)
	for _, p := range []string{".claude-plugin/plugin.json", "skills/hi/SKILL.md", "agents/planner.md", ".mcp.json", "bin/srv"} {
		if _, ok := fs[p]; !ok {
			t.Errorf("faltou %s", p)
		}
	}
	if _, ok := fs["hooks/hooks.json"]; ok {
		t.Error("hooks não entram sem --only hooks")
	}
	m := jsonOf(t, fs[".claude-plugin/plugin.json"])
	if m["name"] != "demo" || m["version"] != "1.2.0" || m["author"].(map[string]any)["name"] != "alguem" {
		t.Errorf("plugin.json: %v", m)
	}
	srv := jsonOf(t, fs[".mcp.json"])["mcpServers"].(map[string]any)["local"].(map[string]any)
	if srv["command"] != "${CLAUDE_PLUGIN_ROOT}/bin/srv" {
		t.Errorf("PLUGIN_ROOT do Claude: %v", srv)
	}
	if !strings.Contains(notes(r), "rules: plugins do Claude Code") || !strings.Contains(notes(r), "só entram no artefato com --only hooks") {
		t.Errorf("notas: %s", notes(r))
	}

	r, _ = Build(pack(t), ClaudePlugin, []manifest.Module{manifest.ModuleHooks})
	fs = files(r)
	h := string(fs["hooks/hooks.json"].Content)
	if !strings.Contains(h, `"PreToolUse"`) || !strings.Contains(h, `--dir \"${CLAUDE_PLUGIN_ROOT}/hooks/atlana\"`) {
		t.Fatalf("hooks.json:\n%s", h)
	}
	if _, ok := fs["hooks/atlana/demo/guard/run.sh"]; !ok {
		t.Fatal("run.sh do hook não copiado")
	}
}

func TestGeminiExtension(t *testing.T) {
	r, err := Build(pack(t), GeminiExtension, nil)
	if err != nil {
		t.Fatal(err)
	}
	fs := files(r)
	m := jsonOf(t, fs["gemini-extension.json"])
	if m["contextFileName"] != "GEMINI.md" || m["author"] != nil {
		t.Errorf("manifest: %v", m)
	}
	if srv := m["mcpServers"].(map[string]any)["local"].(map[string]any); srv["command"] != "${extensionPath}/bin/srv" {
		t.Errorf("extensionPath: %v", srv)
	}
	if !strings.Contains(string(fs["GEMINI.md"].Content), "# Regras") {
		t.Error("GEMINI.md sem as rules")
	}
	if !strings.Contains(string(fs["agents/planner.md"].Content), "kind: local") {
		t.Error("subagent do Gemini")
	}
}

func TestCursorPlugin(t *testing.T) {
	r, err := Build(pack(t), CursorPlugin, []manifest.Module{manifest.ModuleRules, manifest.ModuleHooks, manifest.ModuleMCP, manifest.ModuleAgents})
	if err != nil {
		t.Fatal(err)
	}
	fs := files(r)
	for _, p := range []string{".cursor-plugin/plugin.json", "rules/atlana-demo-AGENTS.mdc", "rules/atlana-demo-hooks-guard.mdc", "mcp.json"} {
		if _, ok := fs[p]; !ok {
			t.Errorf("faltou %s", p)
		}
	}
	servers := jsonOf(t, fs["mcp.json"])["mcpServers"].(map[string]any)
	if _, ok := servers["local"]; ok {
		t.Error("servidor com ${PLUGIN_ROOT} não pode ir para o Cursor")
	}
	if servers["secret"].(map[string]any)["env"].(map[string]any)["TOKEN"] != "${env:TOKEN}" {
		t.Errorf("env do Cursor: %v", servers)
	}
	if !strings.Contains(notes(r), "subagents ainda não são gerados para o Cursor") {
		t.Error(notes(r))
	}
}

func TestAgentPluginSpec(t *testing.T) {
	r, err := Build(pack(t), AgentPlugin, nil)
	if err != nil {
		t.Fatal(err)
	}
	fs := files(r)
	m := jsonOf(t, fs["plugin.json"])
	allowed := map[string]bool{"$schema": true, "name": true, "version": true, "description": true, "author": true, "homepage": true, "repository": true, "license": true, "keywords": true, "extensions": true}
	for k := range m {
		if !allowed[k] {
			t.Errorf("campo fora da spec: %s", k)
		}
	}
	if m["$schema"] != AgentPluginsSchema {
		t.Errorf("$schema: %v", m["$schema"])
	}
	mcp := jsonOf(t, fs["mcp.json"])
	servers := mcp["mcpServers"].(map[string]any)
	local := servers["local"].(map[string]any)
	if mcp["$schema"] != AgentPluginsMCPSchema || local["type"] != "stdio" || local["command"] != "./bin/srv" {
		t.Errorf("mcp.json: %v", mcp)
	}
	if _, ok := servers["secret"]; ok || !strings.Contains(notes(r), "não expande variáveis") {
		t.Errorf("servidor com segredo não tem forma portável: %v\n%s", servers, notes(r))
	}
	if _, ok := fs["agents/planner.md"]; ok {
		t.Error("a spec só tem skills e MCP")
	}
}

func TestAgentPluginServer(t *testing.T) {
	s, err := agentPluginServer(map[string]any{"url": "https://x", "type": "sse"})
	if err != nil || s["type"] != "sse" {
		t.Fatal(s, err)
	}
	if s, _ := agentPluginServer(map[string]any{"url": "https://x"}); s["type"] != "streamable-http" {
		t.Fatal(s)
	}
	if _, err := agentPluginServer(map[string]any{"url": "https://x", "headers": map[string]any{"Authorization": "Bearer ${T}"}}); err == nil {
		t.Fatal("header com segredo")
	}
	if _, err := agentPluginServer(map[string]any{"command": "${HOME}/x"}); err == nil {
		t.Fatal("command com variável")
	}
}

func TestHookFallbackFail(t *testing.T) {
	p := pack(t)
	write(t, filepath.Join(p.Dir, "hooks", "guard", "hook.yaml"), "name: guard\nevent: pre_tool\nrun: ./run.sh\nfallback: fail\n", 0o644)
	if _, err := Build(p, CursorPlugin, []manifest.Module{manifest.ModuleHooks}); err == nil || !strings.Contains(err.Error(), "fallback: fail") {
		t.Fatalf("esperava erro: %v", err)
	}
}

func TestWriteOnlyReplacesOwnBuilds(t *testing.T) {
	r, _ := Build(pack(t), ClaudePlugin, nil)
	out := t.TempDir()
	dst, err := Write(r, out, "test")
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dst, "bin", "srv")); err != nil || st.Mode()&0o111 == 0 {
		t.Fatal("arquivo de apoio perdeu o +x")
	}
	os.WriteFile(filepath.Join(dst, "sobra.txt"), []byte("x"), 0o644)
	if _, err := Write(r, out, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "sobra.txt")); !os.IsNotExist(err) {
		t.Fatal("rebuild deveria começar limpo")
	}

	foreign := filepath.Join(out, "claude-plugin", "demo")
	os.Remove(filepath.Join(foreign, Marker))
	if _, err := Write(r, out, "test"); err == nil || !strings.Contains(err.Error(), "não foi gerado pelo atlana build") {
		t.Fatalf("não pode apagar pasta que não é nossa: %v", err)
	}
	if _, err := os.Stat(filepath.Join(foreign, "skills")); err != nil {
		t.Fatal("a pasta da pessoa foi mexida")
	}
}

func TestReadme(t *testing.T) {
	p := pack(t)
	r, _ := Build(p, ClaudePlugin, nil)
	got := string(files(r)["README.md"].Content)
	for _, want := range []string{"# demo\n\nPack de teste.", "**Skills:** hi", "**Servidores MCP:** local, secret", "variáveis de ambiente antes de usar: TOKEN", "MIT"} {
		if !strings.Contains(got, want) {
			t.Errorf("README sem %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Hooks") {
		t.Error("hooks não entraram, então o README não fala deles")
	}
	write(t, filepath.Join(p.Dir, "README.md"), "# meu readme\n", 0o644)
	r, _ = Build(p, ClaudePlugin, nil)
	if c := files(r)["README.md"]; c.Source == "" || len(c.Content) != 0 {
		t.Fatal("README do pack tem prioridade sobre o gerado")
	}
}

func TestBuildCommands(t *testing.T) {
	r, _ := Build(pack(t), ClaudePlugin, nil)
	if c := string(files(r)["commands/oi.md"].Content); !strings.Contains(c, "description: Diz oi.") || !strings.Contains(c, "$ARGUMENTS") {
		t.Fatalf("claude:\n%s", c)
	}
	r, _ = Build(pack(t), GeminiExtension, nil)
	if c := string(files(r)["commands/oi.toml"].Content); !strings.Contains(c, "{{args}}") {
		t.Fatalf("gemini:\n%s", c)
	}
	r, _ = Build(pack(t), AgentPlugin, nil)
	if _, ok := files(r)["commands/oi.md"]; ok || !strings.Contains(notes(r), "commands: a spec Agent Plugins") {
		t.Fatalf("agent-plugin: %s", notes(r))
	}
}

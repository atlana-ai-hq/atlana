package adapter

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func local() map[string]any {
	return map[string]any{
		"command": "${PLUGIN_ROOT}/bin/srv",
		"args":    []any{"--root", "${PLUGIN_ROOT}"},
		"env":     map[string]any{"TOKEN": "${TOKEN}"},
	}
}

func remote() map[string]any {
	return map[string]any{"url": "https://mcp.example", "headers": map[string]any{"Authorization": "Bearer ${TOKEN}", "X-Team": "${TEAM}"}}
}

func server(t *testing.T, id string, def map[string]any) map[string]any {
	t.Helper()
	ad, _ := Get(id)
	s, err := ad.(MCPAdapter).MCPServer(def, "/pack")
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return s
}

func TestMCPConfigPaths(t *testing.T) {
	env := fakeEnv(t)
	cases := []struct {
		id     string
		scope  Scope
		path   string
		format string
	}{
		{"claude", ScopeUser, filepath.Join(env.Home, ".claude.json"), FormatJSON},
		{"claude", ScopeProject, filepath.Join(env.WorkDir, ".mcp.json"), FormatJSON},
		{"cursor", ScopeUser, filepath.Join(env.Home, ".cursor", "mcp.json"), FormatJSON},
		{"cursor", ScopeProject, filepath.Join(env.WorkDir, ".cursor", "mcp.json"), FormatJSON},
		{"gemini", ScopeUser, filepath.Join(env.Home, ".gemini", "settings.json"), FormatJSON},
		{"gemini", ScopeProject, filepath.Join(env.WorkDir, ".gemini", "settings.json"), FormatJSON},
		{"codex", ScopeUser, filepath.Join(env.Home, ".codex", "config.toml"), FormatTOML},
		{"codex", ScopeProject, filepath.Join(env.WorkDir, ".codex", "config.toml"), FormatTOML},
	}
	for _, c := range cases {
		ad, _ := Get(c.id)
		cfg, err := ad.(MCPAdapter).MCPConfig(c.scope, env)
		if err != nil || cfg.Path != c.path || cfg.Format != c.format {
			t.Errorf("%s/%s = %+v %v", c.id, c.scope, cfg, err)
		}
	}
	if _, ok := Adapter(grok{}).(MCPAdapter); ok {
		t.Error("grok não deveria instalar MCP no v1")
	}
}

func TestMCPTranslations(t *testing.T) {
	c := server(t, "claude", local())
	if c["command"] != "/pack/bin/srv" || !reflect.DeepEqual(c["args"], []any{"--root", "/pack"}) || c["env"].(map[string]any)["TOKEN"] != "${TOKEN}" {
		t.Errorf("claude local: %v", c)
	}
	if r := server(t, "claude", remote()); r["type"] != "http" {
		t.Errorf("claude remoto sem type: %v", r)
	}
	if cu := server(t, "cursor", local()); cu["env"].(map[string]any)["TOKEN"] != "${env:TOKEN}" {
		t.Errorf("cursor env: %v", cu)
	}
	g := server(t, "gemini", remote())
	if g["httpUrl"] != "https://mcp.example" || g["url"] != nil {
		t.Errorf("gemini remoto: %v", g)
	}
	sse := remote()
	sse["type"] = "sse"
	if g := server(t, "gemini", sse); g["url"] == nil || g["type"] != nil {
		t.Errorf("gemini sse: %v", g)
	}
	x := server(t, "codex", local())
	if !reflect.DeepEqual(x["env_vars"], []any{"TOKEN"}) || x["env"] != nil {
		t.Errorf("codex env: %v", x)
	}
	xr := server(t, "codex", remote())
	if xr["bearer_token_env_var"] != "TOKEN" || xr["env_http_headers"].(map[string]any)["X-Team"] != "TEAM" || xr["headers"] != nil {
		t.Errorf("codex headers: %v", xr)
	}
}

func TestCodexRejectsRenamedEnv(t *testing.T) {
	def := map[string]any{"command": "x", "env": map[string]any{"API_KEY": "${MY_KEY}"}}
	if _, err := (codex{}).MCPServer(def, "/p"); err == nil || !strings.Contains(err.Error(), "mesmo nome") {
		t.Fatalf("esperava erro de renomear variável: %v", err)
	}
}

func TestPrepareDoesNotMutateInput(t *testing.T) {
	def := local()
	server(t, "cursor", def)
	if def["env"].(map[string]any)["TOKEN"] != "${TOKEN}" || def["command"] != "${PLUGIN_ROOT}/bin/srv" {
		t.Fatal("prepare não pode alterar a definição do pack")
	}
}

package adapter

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Formatos de arquivo de config (os mesmos valores de configmerge.Format).
const (
	FormatJSON = "json"
	FormatTOML = "toml"
)

// ConfigFile diz em que arquivo e seção de config um agente guarda um módulo (MCP, hooks).
type ConfigFile struct {
	Path    string
	Format  string // FormatJSON ou FormatTOML
	Section string // ex.: "mcpServers"
}

// MCPAdapter é implementado por adaptadores que instalam servidores MCP.
//
// O servidor entra por merge na config do agente; nada fora dele muda, e a
// propriedade fica no atlana.lock (ADR 0004).
type MCPAdapter interface {
	MCPConfig(scope Scope, env Env) (ConfigFile, error)
	// MCPServer traduz um servidor no formato Agent Plugins para o do agente.
	MCPServer(def map[string]any, packDir string) (map[string]any, error)
}

var envPlaceholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// prepare copia o servidor, troca ${PLUGIN_ROOT} pelo diretório do pack e
// aplica ref a cada ${VAR} dentro de env e headers.
func prepare(def map[string]any, packDir string, ref func(name string) string) map[string]any {
	var walk func(v any, inSecret bool) any
	walk = func(v any, inSecret bool) any {
		switch x := v.(type) {
		case string:
			x = strings.ReplaceAll(x, "${PLUGIN_ROOT}", packDir)
			if inSecret && ref != nil {
				x = envPlaceholder.ReplaceAllStringFunc(x, func(m string) string {
					return ref(envPlaceholder.FindStringSubmatch(m)[1])
				})
			}
			return x
		case []any:
			out := make([]any, len(x))
			for i, it := range x {
				out[i] = walk(it, inSecret)
			}
			return out
		case map[string]any:
			out := make(map[string]any, len(x))
			for k, it := range x {
				out[k] = walk(it, inSecret)
			}
			return out
		}
		return v
	}
	out := make(map[string]any, len(def))
	for k, v := range def {
		out[k] = walk(v, k == "env" || k == "headers")
	}
	return out
}

// claude: ~/.claude.json (user) ou <projeto>/.mcp.json (project). O Claude Code
// expande ${VAR}; servidores remotos precisam de "type".
func (claude) MCPConfig(scope Scope, env Env) (ConfigFile, error) {
	var path string
	var err error
	if scope == ScopeUser {
		path, err = scopedDir(scope, env, ".claude.json")
	} else {
		path, err = scopedDir(scope, env, ".mcp.json")
	}
	return ConfigFile{Path: path, Format: FormatJSON, Section: "mcpServers"}, err
}
func (claude) MCPServer(def map[string]any, packDir string) (map[string]any, error) {
	s := prepare(def, packDir, nil)
	if _, ok := s["url"]; ok {
		if _, ok := s["type"]; !ok {
			s["type"] = "http"
		}
	}
	return s, nil
}

// cursor: ~/.cursor/mcp.json ou <projeto>/.cursor/mcp.json; variáveis como ${env:VAR}.
func (cursor) MCPConfig(scope Scope, env Env) (ConfigFile, error) {
	path, err := scopedDir(scope, env, ".cursor", "mcp.json")
	return ConfigFile{Path: path, Format: FormatJSON, Section: "mcpServers"}, err
}
func (cursor) MCPServer(def map[string]any, packDir string) (map[string]any, error) {
	return prepare(def, packDir, func(n string) string { return "${env:" + n + "}" }), nil
}

// gemini: settings.json do usuário ou do projeto. HTTP remoto usa httpUrl; SSE, url.
func (gemini) MCPConfig(scope Scope, env Env) (ConfigFile, error) {
	path, err := scopedDir(scope, env, ".gemini", "settings.json")
	return ConfigFile{Path: path, Format: FormatJSON, Section: "mcpServers"}, err
}
func (gemini) MCPServer(def map[string]any, packDir string) (map[string]any, error) {
	s := prepare(def, packDir, nil)
	if u, ok := s["url"]; ok && s["type"] != "sse" {
		delete(s, "url")
		s["httpUrl"] = u
	}
	delete(s, "type")
	return s, nil
}

// codex: config.toml em $CODEX_HOME (ou ~/.codex) ou em <projeto>/.codex. O Codex
// não expande ${VAR}: env vira env_vars (repassa a variável do ambiente) e
// headers viram bearer_token_env_var / env_http_headers.
func (codex) MCPConfig(scope Scope, env Env) (ConfigFile, error) {
	var path string
	var err error
	if scope == ScopeUser && env.Getenv != nil && env.Getenv("CODEX_HOME") != "" {
		path = filepath.Join(env.Getenv("CODEX_HOME"), "config.toml")
	} else {
		path, err = scopedDir(scope, env, ".codex", "config.toml")
	}
	return ConfigFile{Path: path, Format: FormatTOML, Section: "mcp_servers"}, err
}
func (codex) MCPServer(def map[string]any, packDir string) (map[string]any, error) {
	s := prepare(def, packDir, nil)
	delete(s, "type")
	if raw, ok := s["env"].(map[string]any); ok {
		delete(s, "env")
		var vars []any
		for _, k := range sortedKeys(raw) {
			m := envRef(raw[k])
			if m != k {
				return nil, fmt.Errorf("env %s=%v: o Codex só repassa a variável com o mesmo nome (use %s: ${%s})", k, raw[k], k, k)
			}
			vars = append(vars, k)
		}
		if len(vars) > 0 {
			s["env_vars"] = vars
		}
	}
	if raw, ok := s["headers"].(map[string]any); ok {
		delete(s, "headers")
		envHeaders := map[string]any{}
		for _, k := range sortedKeys(raw) {
			v, _ := raw[k].(string)
			if strings.EqualFold(k, "Authorization") && strings.HasPrefix(v, "Bearer ") {
				s["bearer_token_env_var"] = envRef(strings.TrimPrefix(v, "Bearer "))
				continue
			}
			name := envRef(v)
			if name == "" {
				return nil, fmt.Errorf("header %s: o Codex só aceita ${VAR} ou Bearer ${VAR}", k)
			}
			envHeaders[k] = name
		}
		if len(envHeaders) > 0 {
			s["env_http_headers"] = envHeaders
		}
	}
	return s, nil
}

// envRef devolve NOME se v for exatamente "${NOME}".
func envRef(v any) string {
	s, _ := v.(string)
	m := envPlaceholder.FindStringSubmatch(s)
	if m == nil || m[0] != s {
		return ""
	}
	return m[1]
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

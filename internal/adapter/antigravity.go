package adapter

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// antigravity: Google Antigravity (CLI `agy`). A raiz de customização é
// ~/.gemini/config (user) ou <projeto>/.agents (project), com skills/<n>/SKILL.md,
// rules/*.md e mcp_config.json (ADR 0013). Não confundir com o Gemini CLI, que
// usa ~/.gemini/settings.json.
type antigravity struct{}

func (antigravity) ID() string   { return "antigravity" }
func (antigravity) Name() string { return "Antigravity" }
func (antigravity) Detect(env Env) Detection {
	return detectBy(env, []string{"agy"}, []string{
		filepath.Join(env.Home, ".gemini", "antigravity-cli"),
		filepath.Join(env.Home, ".gemini", "antigravity"),
	})
}
func (antigravity) Supports(m manifest.Module) bool {
	return m == manifest.ModuleSkills || m == manifest.ModuleRules || m == manifest.ModuleMCP ||
		m == manifest.ModuleCommands || m == manifest.ModuleAgents || m == manifest.ModuleHooks
}

func antigravityRoot(scope Scope, env Env, sub ...string) (string, error) {
	if scope == ScopeUser {
		return scopedDir(scope, env, append([]string{".gemini", "config"}, sub...)...)
	}
	return scopedDir(scope, env, append([]string{".agents"}, sub...)...)
}

func (antigravity) SkillsRoot(scope Scope, env Env) (string, error) {
	return antigravityRoot(scope, env, "skills")
}

// Rules: arquivos planos em rules/, cada um com trigger: always_on.
func (antigravity) RulesRoot(scope Scope, env Env) (string, error) {
	return antigravityRoot(scope, env, "rules")
}
func (a antigravity) Rules(pack string, files []RuleFile, scope Scope, env Env) ([]RuleOutput, []string, error) {
	root, err := a.RulesRoot(scope, env)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]string{}
	out := make([]RuleOutput, 0, len(files))
	for _, f := range files {
		name := "atlana-" + pack + "-" + strings.ReplaceAll(strings.TrimSuffix(f.Rel, filepath.Ext(f.Rel)), "/", "-") + ".md"
		if prev, ok := seen[name]; ok {
			return nil, nil, fmt.Errorf("antigravity: %s e %s viram o mesmo arquivo %s", prev, f.Rel, name)
		}
		seen[name] = f.Rel
		body := "---\ntrigger: always_on\n---\n\n" + strings.TrimLeft(stripFrontmatter(f.Content), "\n")
		out = append(out, RuleOutput{Path: filepath.Join(root, name), Content: []byte(body)})
	}
	return out, nil, nil
}

// MCP: só ~/.gemini/config/mcp_config.json (a doc não traz config de projeto).
// O Antigravity não expande ${VAR}: "NOME": "${NOME}" em env sai da entrada e o
// servidor herda a variável do ambiente do agy; qualquer outro ${VAR} recusa o
// servidor, porque o atlana nunca grava o valor do segredo.
func (antigravity) MCPConfig(scope Scope, env Env) (ConfigFile, error) {
	if scope != ScopeUser {
		return ConfigFile{}, fmt.Errorf("MCP do Antigravity só no escopo user (~/.gemini/config/mcp_config.json): %w", ErrUnsupported)
	}
	path, err := antigravityRoot(scope, env, "mcp_config.json")
	return ConfigFile{Path: path, Format: FormatJSON, Section: "mcpServers"}, err
}
func (antigravity) MCPServer(def map[string]any, packDir string) (map[string]any, error) {
	s := prepare(def, packDir, nil)
	if t, _ := s["type"].(string); t == "sse" {
		return nil, fmt.Errorf("o Antigravity não suporta SSE; use Streamable HTTP ou uma ponte stdio")
	}
	delete(s, "type")
	if raw, ok := s["env"].(map[string]any); ok {
		kept := map[string]any{}
		for _, k := range sortedKeys(raw) {
			v := raw[k]
			if name := envRef(v); name != "" {
				if name != k {
					return nil, fmt.Errorf("env.%s usa ${%s}, mas o Antigravity não expande variáveis; use o mesmo nome (%s: ${%s})", k, name, k, k)
				}
				continue // herdada do ambiente
			}
			if str, _ := v.(string); envPlaceholder.MatchString(str) {
				return nil, fmt.Errorf("env.%s mistura texto e ${VAR}, que o Antigravity não expande", k)
			}
			kept[k] = v
		}
		delete(s, "env")
		if len(kept) > 0 {
			s["env"] = kept
		}
	}
	if h, ok := s["headers"].(map[string]any); ok {
		for _, k := range sortedKeys(h) {
			if str, _ := h[k].(string); envPlaceholder.MatchString(str) {
				return nil, fmt.Errorf("headers.%s usa ${VAR}, que o Antigravity não expande; o atlana não grava segredos na config", k)
			}
		}
	}
	return s, nil
}

// Commands: no Antigravity a skill é o slash command (/<nome>), e os workflows
// estão descontinuados. O command vira skills/<nome>/SKILL.md.
func (a antigravity) CommandsRoot(scope Scope, env Env) (string, error) {
	return a.SkillsRoot(scope, env)
}
func (antigravity) Command(c manifest.Command) (string, []byte, []string, error) {
	body := strings.ReplaceAll(c.Body, "$ARGUMENTS", "[o texto que a pessoa digitou depois de /"+c.Name+"]")
	var notes []string
	if manifest.Positional.MatchString(body) {
		notes = append(notes, fmt.Sprintf("antigravity: o command %s usa $1…$9, que o Antigravity não expande", c.Name))
	}
	b, err := renderAgent(map[string]any{"name": c.Name, "description": c.Description}, []string{"name", "description"}, body)
	return filepath.Join(c.Name, "SKILL.md"), b, notes, err
}

// Subagents: agents/<nome>/agent.md com subagent: true e mainAgent: false.
// Sem tools, o Antigravity dá só um conjunto pequeno de leitura; no atlana,
// lista vazia é "todas", então ela vira a lista completa.
var antigravityTools = map[string][]string{
	"read": {"read_file"}, "search": {"read_file"}, "edit": {"edit_file"}, "write": {"write_file"},
	"shell": {"run_command", "manage_task"}, "web": {"search_web"},
	"shell:readonly": nil, "mcp": nil,
}

func (antigravity) AgentsRoot(scope Scope, env Env) (string, error) {
	return antigravityRoot(scope, env, "agents")
}
func (antigravity) Subagent(s manifest.Subagent) (string, []byte, []string, error) {
	front, notes, err := subagentWithTools("antigravity", s, antigravityTools)
	if err != nil {
		return "", nil, notes, err
	}
	if _, ok := front["tools"]; !ok {
		front["tools"] = []string{"read_file", "edit_file", "write_file", "run_command", "manage_task", "search_web"}
	}
	front["subagent"] = true
	front["mainAgent"] = false
	if s.Model != "inherit" {
		notes = append(notes, fmt.Sprintf("antigravity: model %q não tem equivalente; o subagent %s usa o modelo da sessão", s.Model, s.Name))
	}
	body := s.Body
	if !strings.HasPrefix(strings.TrimLeft(body, "\n"), "# ") {
		body = "# " + s.Name + "\n\n" + body
	}
	b, err := renderAgent(front, []string{"name", "description", "subagent", "mainAgent", "tools"}, body)
	return filepath.Join(s.Name, "agent.md"), b, notes, err
}

// Hooks: hooks.json da raiz, num hook nomeado "atlana" com o mesmo formato de
// eventos do Claude Code. O Antigravity lê a decisão em JSON no stdout (o
// dispatcher responde por ele; ver AntigravityReply).
var antigravityEvents = map[string]string{
	"pre_tool": "PreToolUse", "post_tool": "PostToolUse",
	"session_start": "SessionStart", "stop": "Stop",
	// prompt_submit fica de fora: PreInvocation roda antes de cada chamada ao
	// modelo, não só quando a pessoa envia uma mensagem.
}

func (antigravity) HooksConfig(scope Scope, env Env) (ConfigFile, error) {
	path, err := antigravityRoot(scope, env, "hooks.json")
	return ConfigFile{Path: path, Format: FormatJSON, Section: "atlana"}, err
}
func (antigravity) HookEvent(e string) (string, bool) { n, ok := antigravityEvents[e]; return n, ok }
func (antigravity) ToolCategory(native string) string {
	switch {
	case native == "run_command":
		return "shell"
	case native == "edit_file":
		return "edit"
	case native == "write_file":
		return "write"
	case native == "read_file" || native == "view_file" || native == "list_directory" || native == "grep_search" || native == "find":
		return "read"
	case native == "mcp_tool" || strings.HasPrefix(native, "mcp_"):
		return "mcp"
	}
	return ""
}

// HookEntry: eventos de ferramenta usam {matcher, hooks}; os outros, o handler direto.
func (antigravity) HookEntry(nativeEvent, command string) map[string]any {
	if nativeEvent == "PreToolUse" || nativeEvent == "PostToolUse" {
		return toolEntry(nativeEvent, command, nativeEvent)
	}
	return map[string]any{"type": "command", "command": command}
}

// AntigravityReply traduz o resultado do dispatcher para o stdout do
// Antigravity. Só nega quando um hook negou (exit 2) num pre_tool; nos outros
// casos devolve {}, sem "allow", para não pular as permissões da pessoa.
func AntigravityReply(event string, code int, reason string) []byte {
	if code == 2 && event == "pre_tool" {
		b, _ := json.Marshal(map[string]string{"decision": "deny", "reason": strings.TrimSpace(reason)})
		return b
	}
	return []byte("{}")
}

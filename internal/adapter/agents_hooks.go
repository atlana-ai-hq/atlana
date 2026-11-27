package adapter

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// AgentsAdapter é implementado por adaptadores que instalam subagents.
type AgentsAdapter interface {
	AgentsRoot(scope Scope, env Env) (string, error)
	// Subagent traduz o subagent canônico: nome do arquivo dentro de AgentsRoot,
	// conteúdo e avisos (ex.: ferramenta sem equivalente).
	Subagent(s manifest.Subagent) (file string, content []byte, notes []string, err error)
}

// HooksAdapter é implementado por adaptadores que ligam hooks nativos.
//
// O atlana registra, por evento usado, uma entrada que chama
// `atlana hook dispatch`; quem roda os scripts é o dispatcher (ADR 0005).
type HooksAdapter interface {
	HooksConfig(scope Scope, env Env) (ConfigFile, error)
	// HookEvent traduz o evento canônico (pre_tool…) para o nativo.
	HookEvent(event string) (string, bool)
	// ToolCategory traduz o nome nativo da ferramenta para shell|edit|write|read|mcp ("" se desconhecida).
	ToolCategory(native string) string
	// HookEntry é o item do array hooks.<evento> que chama o dispatcher.
	HookEntry(nativeEvent, command string) map[string]any
}

// toolMap traduz ferramentas abstratas; "" = sem equivalente.
func mapTools(agent string, tools []string, table map[string][]string) ([]string, []string) {
	var out, notes []string
	seen := map[string]bool{}
	for _, t := range tools {
		native, ok := table[t]
		if !ok || len(native) == 0 {
			notes = append(notes, fmt.Sprintf("%s: a ferramenta %q não tem equivalente seguro e foi deixada de fora", agent, t))
			continue
		}
		for _, n := range native {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out, notes
}

func renderAgent(front map[string]any, order []string, body string) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range order {
		v, ok := front[k]
		if !ok {
			continue
		}
		y, err := yaml.Marshal(map[string]any{k: v})
		if err != nil {
			return nil, err
		}
		b.Write(y)
	}
	b.WriteString("---\n\n")
	b.WriteString(body)
	return []byte(b.String()), nil
}

// subagentWithTools monta o frontmatter comum e recusa o caso em que a lista
// pedida some por inteiro (no agente, lista vazia = todas as ferramentas).
func subagentWithTools(agent string, s manifest.Subagent, table map[string][]string) (map[string]any, []string, error) {
	front := map[string]any{"name": s.Name, "description": s.Description}
	if len(s.Tools) == 0 {
		return front, nil, nil
	}
	tools, notes := mapTools(agent, s.Tools, table)
	if len(tools) == 0 {
		return nil, notes, fmt.Errorf("nenhuma das ferramentas pedidas tem equivalente; instalar daria acesso a todas")
	}
	front["tools"] = tools
	return front, notes, nil
}

var claudeTools = map[string][]string{
	"read": {"Read"}, "search": {"Grep", "Glob"}, "edit": {"Edit"}, "write": {"Write"},
	"shell": {"Bash"}, "web": {"WebFetch", "WebSearch"},
	// shell:readonly e mcp: o Claude Code não tem um jeito portável de restringir.
	"shell:readonly": nil, "mcp": nil,
}

// claude: subagents em .claude/agents/<nome>.md.
func (claude) AgentsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".claude", "agents")
}
func (claude) Subagent(s manifest.Subagent) (string, []byte, []string, error) {
	front, notes, err := subagentWithTools("claude", s, claudeTools)
	if err != nil {
		return "", nil, notes, err
	}
	if tools, ok := front["tools"].([]string); ok {
		front["tools"] = strings.Join(tools, ", ")
	}
	front["model"] = map[string]string{"inherit": "inherit", "fast": "haiku", "smart": "opus"}[s.Model]
	b, err := renderAgent(front, []string{"name", "description", "tools", "model"}, s.Body)
	return s.Name + ".md", b, notes, err
}

var geminiTools = map[string][]string{
	"read":   {"read_file", "read_many_files", "list_directory"},
	"search": {"glob", "search_file_content"},
	"edit":   {"replace"}, "write": {"write_file"}, "shell": {"run_shell_command"},
	"web": {"web_fetch", "google_web_search"}, "mcp": {"mcp_*"},
	"shell:readonly": nil,
}

// gemini: subagents em .gemini/agents/<nome>.md (kind: local).
func (gemini) AgentsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".gemini", "agents")
}
func (gemini) Subagent(s manifest.Subagent) (string, []byte, []string, error) {
	front, notes, err := subagentWithTools("gemini", s, geminiTools)
	if err != nil {
		return "", nil, notes, err
	}
	front["kind"] = "local"
	if s.Model != "inherit" {
		// O Gemini pede nome de modelo; sem um alias estável, fica o da sessão.
		notes = append(notes, fmt.Sprintf("gemini: model %q não tem alias estável; o subagent %s usa o modelo da sessão", s.Model, s.Name))
	}
	b, err := renderAgent(front, []string{"name", "description", "kind", "tools"}, s.Body)
	return s.Name + ".md", b, notes, err
}

// toolEntry monta o item que chama o dispatcher; eventos de ferramenta casam com tudo ("*").
func toolEntry(nativeEvent, command string, toolEvents ...string) map[string]any {
	item := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}
	for _, e := range toolEvents {
		if e == nativeEvent {
			item["matcher"] = "*"
		}
	}
	return item
}

var claudeEvents = map[string]string{
	"pre_tool": "PreToolUse", "post_tool": "PostToolUse", "prompt_submit": "UserPromptSubmit",
	"session_start": "SessionStart", "stop": "Stop",
}

// claude: hooks em ~/.claude/settings.json ou <projeto>/.claude/settings.json.
func (claude) HooksConfig(scope Scope, env Env) (ConfigFile, error) {
	path, err := scopedDir(scope, env, ".claude", "settings.json")
	return ConfigFile{Path: path, Format: FormatJSON, Section: "hooks"}, err
}
func (claude) HookEvent(e string) (string, bool) { n, ok := claudeEvents[e]; return n, ok }
func (claude) ToolCategory(native string) string {
	switch {
	case native == "Bash":
		return "shell"
	case native == "Edit" || native == "MultiEdit" || native == "NotebookEdit":
		return "edit"
	case native == "Write":
		return "write"
	case native == "Read" || native == "Grep" || native == "Glob" || native == "LS":
		return "read"
	case strings.HasPrefix(native, "mcp__"):
		return "mcp"
	}
	return ""
}
func (claude) HookEntry(nativeEvent, command string) map[string]any {
	return toolEntry(nativeEvent, command, "PreToolUse", "PostToolUse")
}

var geminiEvents = map[string]string{
	"pre_tool": "BeforeTool", "post_tool": "AfterTool", "prompt_submit": "BeforeAgent",
	"session_start": "SessionStart", "stop": "AfterAgent",
}

// gemini: hooks no mesmo settings.json do MCP.
func (gemini) HooksConfig(scope Scope, env Env) (ConfigFile, error) {
	path, err := scopedDir(scope, env, ".gemini", "settings.json")
	return ConfigFile{Path: path, Format: FormatJSON, Section: "hooks"}, err
}
func (gemini) HookEvent(e string) (string, bool) { n, ok := geminiEvents[e]; return n, ok }
func (gemini) ToolCategory(native string) string {
	switch native {
	case "run_shell_command":
		return "shell"
	case "replace":
		return "edit"
	case "write_file":
		return "write"
	case "read_file", "read_many_files", "glob", "search_file_content", "list_directory":
		return "read"
	}
	return ""
}
func (gemini) HookEntry(nativeEvent, command string) map[string]any {
	return toolEntry(nativeEvent, command, "BeforeTool", "AfterTool")
}

// ShellQuote protege um argumento para o shell que os agentes usam nos hooks.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// DispatchCommand é o comando registrado na config do agente para um evento.
func DispatchCommand(exe, agent, event, dir string) string {
	return fmt.Sprintf("%s hook dispatch --agent %s --event %s --dir %s", ShellQuote(exe), agent, event, ShellQuote(filepath.Clean(dir)))
}

package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// configHome é $XDG_CONFIG_HOME, ou ~/.config.
func configHome(env Env) (string, error) {
	if env.Getenv != nil {
		if x := env.Getenv("XDG_CONFIG_HOME"); x != "" {
			return x, nil
		}
	}
	if env.Home == "" {
		return scopedDir(ScopeUser, env) // devolve o erro de base vazia
	}
	return filepath.Join(env.Home, ".config"), nil
}

// crush: skills em $CRUSH_SKILLS_DIR ou ~/.config/crush/skills, e
// <projeto>/.crush/skills. A config do Crush agora é o crushrc (um script de
// shell; o JSON está obsoleto), então MCP, hooks e rules ainda não são
// gravados: o Atlana não edita script de shell da pessoa (ADR 0009).
type crush struct{}

func (crush) ID() string   { return "crush" }
func (crush) Name() string { return "Crush" }
func (crush) Detect(env Env) Detection {
	dirs := []string{filepath.Join(env.Home, ".config", "crush")}
	if c, err := configHome(env); err == nil {
		dirs = append(dirs, filepath.Join(c, "crush"))
	}
	return detectBy(env, []string{"crush"}, dirs)
}
func (crush) Supports(m manifest.Module) bool { return m == manifest.ModuleSkills }
func (crush) SkillsRoot(scope Scope, env Env) (string, error) {
	if scope != ScopeUser {
		return scopedDir(scope, env, ".crush", "skills")
	}
	if env.Getenv != nil {
		if d := env.Getenv("CRUSH_SKILLS_DIR"); d != "" {
			return d, nil
		}
	}
	c, err := configHome(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(c, "crush", "skills"), nil
}

// opencode: skills em ~/.config/opencode/skills e <projeto>/.opencode/skills;
// MCP por merge em opencode.json(c) (seção "mcp", forma V1, que o OpenCode
// 1.18 lê e para a qual normaliza a V2); subagents em agents/<nome>.md com
// mode: subagent e um mapa permission (ADR 0010).
type opencode struct{}

func (opencode) ID() string   { return "opencode" }
func (opencode) Name() string { return "OpenCode" }
func (opencode) Detect(env Env) Detection {
	dirs := []string{filepath.Join(env.Home, ".opencode")}
	if c, err := configHome(env); err == nil {
		dirs = append(dirs, filepath.Join(c, "opencode"))
	}
	return detectBy(env, []string{"opencode"}, dirs)
}
func (opencode) Supports(m manifest.Module) bool {
	return m == manifest.ModuleSkills || m == manifest.ModuleMCP || m == manifest.ModuleAgents ||
		m == manifest.ModuleRules || m == manifest.ModuleHooks || m == manifest.ModuleCommands
}
func (opencode) SkillsRoot(scope Scope, env Env) (string, error) {
	return opencodeDir(scope, env, ".opencode", "skills")
}
func (opencode) AgentsRoot(scope Scope, env Env) (string, error) {
	return opencodeDir(scope, env, ".opencode", "agents")
}

// opencodeDir: user em $XDG_CONFIG_HOME/opencode/<sub>, project em <projeto>/<projSub...>.
func opencodeDir(scope Scope, env Env, projSub ...string) (string, error) {
	if scope != ScopeUser {
		return scopedDir(scope, env, projSub...)
	}
	c, err := configHome(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(c, "opencode", projSub[len(projSub)-1]), nil
}

// MCPConfig usa opencode.jsonc se já existir (o merge recusa comentários) e,
// senão, opencode.json. No projeto, o arquivo fica na raiz.
func (opencode) MCPConfig(scope Scope, env Env) (ConfigFile, error) {
	var dir string
	var err error
	if scope == ScopeUser {
		var c string
		c, err = configHome(env)
		dir = filepath.Join(c, "opencode")
	} else {
		dir, err = scopedDir(scope, env)
	}
	if err != nil {
		return ConfigFile{}, err
	}
	path := filepath.Join(dir, "opencode.json")
	if st, e := os.Stat(filepath.Join(dir, "opencode.jsonc")); e == nil && st.Mode().IsRegular() {
		path = filepath.Join(dir, "opencode.jsonc")
	}
	return ConfigFile{Path: path, Format: FormatJSON, Section: "mcp"}, nil
}

// MCPServer: stdio vira type local com command em array e environment; HTTP e
// SSE viram type remote. ${VAR} vira {env:VAR}.
func (opencode) MCPServer(def map[string]any, packDir string) (map[string]any, error) {
	s := prepare(def, packDir, func(n string) string { return "{env:" + n + "}" })
	if _, ok := s["cwd"]; ok {
		return nil, errors.New("opencode: cwd não é suportado num servidor MCP local")
	}
	if u, ok := s["url"]; ok {
		out := map[string]any{"type": "remote", "url": u}
		if h, ok := s["headers"]; ok {
			out["headers"] = h
		}
		return out, nil
	}
	cmd, _ := s["command"].(string)
	if cmd == "" {
		return nil, errors.New("opencode: servidor sem command nem url")
	}
	argv := []any{cmd}
	if args, ok := s["args"].([]any); ok {
		argv = append(argv, args...)
	}
	out := map[string]any{"type": "local", "command": argv}
	if e, ok := s["env"]; ok {
		out["environment"] = e
	}
	return out, nil
}

// Subagent: o OpenCode controla ferramentas por permissão: edit (cobre write),
// bash e webfetch. Leitura e busca ficam sempre liberadas.
func (opencode) Subagent(s manifest.Subagent) (string, []byte, []string, error) {
	front := map[string]any{"description": s.Description, "mode": "subagent"}
	var notes []string
	if len(s.Tools) > 0 {
		has := map[string]bool{}
		for _, t := range s.Tools {
			has[t] = true
		}
		allow := func(b bool) string {
			if b {
				return "allow"
			}
			return "deny"
		}
		perm := map[string]string{"edit": allow(has["edit"] || has["write"]), "webfetch": allow(has["web"])}
		switch {
		case has["shell"]:
			perm["bash"] = "allow"
		case has["shell:readonly"]:
			perm["bash"] = "ask"
			notes = append(notes, fmt.Sprintf("opencode: %s pede shell:readonly; o OpenCode não restringe a leitura, então cada comando pede confirmação (bash: ask)", s.Name))
		default:
			perm["bash"] = "deny"
		}
		if has["edit"] != has["write"] {
			notes = append(notes, fmt.Sprintf("opencode: edit e write são uma permissão só; %s pode as duas coisas", s.Name))
		}
		if !has["mcp"] {
			notes = append(notes, fmt.Sprintf("opencode: ferramentas MCP não são restringidas por subagent; %s continua vendo as que estiverem ligadas", s.Name))
		}
		front["permission"] = perm
	}
	if s.Model != "inherit" && s.Model != "" {
		notes = append(notes, fmt.Sprintf("opencode: model %q não tem alias estável; o subagent %s usa o modelo da sessão", s.Model, s.Name))
	}
	b, err := renderAgent(front, []string{"description", "mode", "permission"}, s.Body)
	return s.Name + ".md", b, notes, err
}

// RulesConfigAdapter é implementado por agentes que só carregam rules listadas
// na própria config (ex.: "instructions" do OpenCode). Os arquivos continuam
// sendo do atlana; na config entra só um item por pack, com dono no lock.
type RulesConfigAdapter interface {
	RulesConfig(pack string, scope Scope, env Env) (cfg ConfigFile, entry string, err error)
}

// PluginHooksAdapter é implementado por agentes cujos hooks são plugins de
// código, não itens de config: o atlana gera um plugin por pack que chama
// `atlana hook dispatch --pack <pack>` (ADR 0011).
type PluginHooksAdapter interface {
	HookEvent(event string) (string, bool)
	ToolCategory(native string) string
	PluginsDir(scope Scope, env Env) (string, error)
	HookPlugin(pack, exe, dir string, events []string) (file string, content []byte)
}

// opencode, rules: cada rule vira um arquivo plano em atlana/atlana-<pack>/,
// porque um item absoluto de instructions só expande o glob no último trecho.
func (opencode) RulesRoot(scope Scope, env Env) (string, error) {
	return opencodeDir(scope, env, ".opencode", "atlana")
}

func (o opencode) Rules(pack string, files []RuleFile, scope Scope, env Env) ([]RuleOutput, []string, error) {
	root, err := o.RulesRoot(scope, env)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]string{}
	out := make([]RuleOutput, 0, len(files))
	for _, f := range files {
		name := strings.ReplaceAll(strings.TrimSuffix(f.Rel, filepath.Ext(f.Rel)), "/", "-") + ".md"
		if prev, ok := seen[name]; ok {
			return nil, nil, fmt.Errorf("opencode: %s e %s viram o mesmo arquivo %s", prev, f.Rel, name)
		}
		seen[name] = f.Rel
		out = append(out, RuleOutput{Path: filepath.Join(root, "atlana-"+pack, name), Content: f.Content})
	}
	return out, nil, nil
}

func (o opencode) RulesConfig(pack string, scope Scope, env Env) (ConfigFile, string, error) {
	cfg, err := o.MCPConfig(scope, env)
	if err != nil {
		return ConfigFile{}, "", err
	}
	cfg.Section = "instructions"
	if scope == ScopeUser {
		root, err := o.RulesRoot(scope, env)
		return cfg, filepath.Join(root, "atlana-"+pack, "*.md"), err
	}
	// No projeto, caminho relativo: o OpenCode procura a partir do diretório atual.
	return cfg, ".opencode/atlana/atlana-" + pack + "/*.md", nil
}

var opencodeEvents = map[string]string{
	"pre_tool": "tool.execute.before", "post_tool": "tool.execute.after",
	"session_start": "session.created", "stop": "session.idle",
}

func (opencode) HookEvent(e string) (string, bool) { n, ok := opencodeEvents[e]; return n, ok }

var opencodeBuiltinTools = map[string]string{
	"bash": "shell", "edit": "edit", "multiedit": "edit", "patch": "edit", "apply_patch": "edit",
	"write": "write", "read": "read", "glob": "read", "grep": "read", "list": "read",
	"webfetch": "", "websearch": "", "codesearch": "", "task": "", "todowrite": "", "todoread": "",
	"skill": "", "question": "", "lsp": "",
}

// ToolCategory: ferramentas de MCP aparecem como <servidor>_<ferramenta>.
func (opencode) ToolCategory(native string) string {
	if c, ok := opencodeBuiltinTools[native]; ok {
		return c
	}
	if strings.Contains(native, "_") {
		return "mcp"
	}
	return ""
}

func (opencode) PluginsDir(scope Scope, env Env) (string, error) {
	return opencodeDir(scope, env, ".opencode", "plugins")
}

// HookPlugin gera um plugin V1 do OpenCode (o formato que a 1.18 carrega de
// plugins/). Exit 2 do dispatcher em tool.execute.before nega a ferramenta.
func (opencode) HookPlugin(pack, exe, dir string, events []string) (string, []byte) {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	ident := strings.NewReplacer("-", "_", ".", "_").Replace(pack)
	has := map[string]bool{}
	for _, e := range events {
		has[e] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// Gerado pelo atlana para o pack %s. Não edite: o atlana atualiza e remove este arquivo.\n", pack)
	b.WriteString("import { spawnSync } from \"node:child_process\";\n\n")
	fmt.Fprintf(&b, "const EXE = %s;\nconst DIR = %s;\nconst PACK = %s;\n\n", q(exe), q(filepath.Clean(dir)), q(pack))
	b.WriteString(`function dispatch(event, payload, block) {
  const r = spawnSync(EXE, ["hook", "dispatch", "--agent", "opencode", "--event", event, "--dir", DIR, "--pack", PACK], {
    input: JSON.stringify(payload),
    encoding: "utf8",
    timeout: 120000,
  });
  if (block && r.status === 2) {
    throw new Error((r.stderr || "").trim() || "ação negada por um hook do atlana (" + event + ")");
  }
}

`)
	fmt.Fprintf(&b, "export const AtlanaHooks_%s = async ({ directory }) => ({\n", ident)
	if has["pre_tool"] {
		b.WriteString(`  "tool.execute.before": async (input, output) => {
    dispatch("pre_tool", { tool_name: input.tool, tool_input: (output && output.args) || {}, session_id: input.sessionID, cwd: directory }, true);
  },
`)
	}
	if has["post_tool"] {
		b.WriteString(`  "tool.execute.after": async (input) => {
    dispatch("post_tool", { tool_name: input.tool, tool_input: input.args || {}, session_id: input.sessionID, cwd: directory }, false);
  },
`)
	}
	if has["session_start"] || has["stop"] {
		b.WriteString("  event: async ({ event }) => {\n    const type = event && event.type;\n")
		if has["session_start"] {
			b.WriteString(`    if (type === "session.created") dispatch("session_start", { cwd: directory }, false);` + "\n")
		}
		if has["stop"] {
			b.WriteString(`    if (type === "session.idle") dispatch("stop", { cwd: directory }, false);` + "\n")
		}
		b.WriteString("  },\n")
	}
	b.WriteString("});\n")
	return "atlana-" + pack + ".js", []byte(b.String())
}

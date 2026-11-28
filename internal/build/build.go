// Package build gera plugins nativos a partir de um pack, para quem não quer
// instalar a CLI: plugin do Claude Code, plugin do Cursor, extensão do Gemini
// CLI e plugin no padrão aberto Agent Plugins. Build calcula tudo em memória;
// só Write grava, e só em diretórios de build do próprio atlana (ADR 0007).
package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/adapter"
	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// Target é um formato de saída.
type Target string

const (
	ClaudePlugin    Target = "claude-plugin"
	CursorPlugin    Target = "cursor-plugin"
	GeminiExtension Target = "gemini-extension"
	AgentPlugin     Target = "agent-plugin"
)

// Targets lista os formatos suportados.
var Targets = []Target{ClaudePlugin, CursorPlugin, GeminiExtension, AgentPlugin}

// ParseTarget valida um nome de target.
func ParseTarget(s string) (Target, error) {
	for _, t := range Targets {
		if string(t) == s {
			return t, nil
		}
	}
	return "", fmt.Errorf("target desconhecido %q (válidos: claude-plugin, cursor-plugin, gemini-extension, agent-plugin)", s)
}

// AgentPluginsSchema e AgentPluginsMCPSchema são os $schema exigidos pela spec Agent Plugins 1.0.0.
const (
	AgentPluginsSchema    = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	AgentPluginsMCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
)

// File é um arquivo do artefato: conteúdo gerado ou cópia de Source.
type File struct {
	Path    string // relativo à raiz do artefato, com "/"
	Content []byte
	Source  string
}

// Result é o artefato calculado.
type Result struct {
	Target Target
	Name   string
	Files  []File
	Notes  []string
}

// Build traduz o pack para o target. modules vazio = todos menos hooks (hooks
// rodam comandos: só entram quando pedidos, como no install).
func Build(p *manifest.Pack, t Target, modules []manifest.Module) (*Result, error) {
	b := &builder{p: p, res: &Result{Target: t, Name: p.Name}, mods: map[manifest.Module]bool{}}
	if len(modules) == 0 {
		for _, m := range manifest.AllModules {
			if len(p.Modules.Items(m)) == 0 {
				continue
			}
			if m == manifest.ModuleHooks {
				b.note("o pack tem hooks; eles só entram no artefato com --only hooks")
				continue
			}
			b.mods[m] = true
		}
	} else {
		for _, m := range modules {
			if len(p.Modules.Items(m)) > 0 {
				b.mods[m] = true
			}
		}
	}
	var err error
	switch t {
	case ClaudePlugin:
		err = b.claude()
	case CursorPlugin:
		err = b.cursor()
	case GeminiExtension:
		err = b.gemini()
	case AgentPlugin:
		err = b.agentPlugin()
	default:
		err = fmt.Errorf("target desconhecido %q", t)
	}
	if err != nil {
		return nil, err
	}
	if err := b.support(); err != nil {
		return nil, err
	}
	b.readme()
	sort.Slice(b.res.Files, func(i, j int) bool { return b.res.Files[i].Path < b.res.Files[j].Path })
	return b.res, nil
}

type builder struct {
	p    *manifest.Pack
	res  *Result
	mods map[manifest.Module]bool
}

func (b *builder) note(format string, a ...any) {
	b.res.Notes = append(b.res.Notes, fmt.Sprintf(format, a...))
}

func (b *builder) add(path string, content []byte) {
	b.res.Files = append(b.res.Files, File{Path: path, Content: content})
}

func (b *builder) addJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b.add(path, append(raw, '\n'))
	return nil
}

// copyDir planeja a cópia de src para dst (relativo ao artefato), sem links simbólicos.
func (b *builder) copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: links simbólicos não são permitidos", path)
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		b.res.Files = append(b.res.Files, File{Path: dst + "/" + filepath.ToSlash(rel), Source: path})
		return nil
	})
}

func (b *builder) skills() error {
	if !b.mods[manifest.ModuleSkills] {
		return nil
	}
	for _, s := range b.p.Modules.Skills {
		if err := b.copyDir(b.p.SkillDir(s), "skills/"+s); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) subagents(id string) error {
	if !b.mods[manifest.ModuleAgents] {
		return nil
	}
	ad, _ := adapter.Get(id)
	aa := ad.(adapter.AgentsAdapter)
	for _, name := range b.p.Modules.Agents {
		s, err := b.p.Subagent(name)
		if err != nil {
			return err
		}
		file, content, notes, err := aa.Subagent(s)
		b.res.Notes = append(b.res.Notes, notes...)
		if err != nil {
			b.note("subagent %s pulado: %v", name, err)
			continue
		}
		b.add("agents/"+file, content)
	}
	return nil
}

// commands gera commands/<nome>.<ext> com o tradutor do agente (claude, gemini).
func (b *builder) commands(id string) error {
	if !b.mods[manifest.ModuleCommands] {
		return nil
	}
	ad, _ := adapter.Get(id)
	ca := ad.(adapter.CommandsAdapter)
	for _, name := range b.p.Modules.Commands {
		c, err := b.p.Command(name)
		if err != nil {
			return err
		}
		file, content, notes, err := ca.Command(c)
		b.res.Notes = append(b.res.Notes, notes...)
		if err != nil {
			b.note("command %s pulado: %v", name, err)
			continue
		}
		b.add("commands/"+file, content)
	}
	return nil
}

func (b *builder) rules() ([]adapter.RuleFile, error) {
	if !b.mods[manifest.ModuleRules] {
		return nil, nil
	}
	var out []adapter.RuleFile
	for _, r := range b.p.Modules.Rules {
		raw, err := os.ReadFile(filepath.Join(b.p.Dir, filepath.FromSlash(r)))
		if err != nil {
			return nil, err
		}
		out = append(out, adapter.RuleFile{Rel: filepath.ToSlash(filepath.Clean(r)), Content: raw})
	}
	return out, nil
}

// mcp traduz cada servidor com o adaptador do agente; root é a variável de raiz do plugin.
func (b *builder) mcp(id, root string) (map[string]any, error) {
	if !b.mods[manifest.ModuleMCP] {
		return nil, nil
	}
	defs, err := b.p.MCPServers()
	if err != nil {
		return nil, err
	}
	ad, _ := adapter.Get(id)
	ma := ad.(adapter.MCPAdapter)
	out := map[string]any{}
	for _, name := range b.p.Modules.MCP {
		s, err := ma.MCPServer(defs[name], root)
		if err != nil {
			b.note("servidor %s pulado: %v", name, err)
			continue
		}
		out[name] = s
	}
	return out, nil
}

// hooks gera hooks/hooks.json com um dispatcher por evento e copia os hooks
// para hooks/atlana/<pack>/<hook>/. O dispatcher precisa do atlana no PATH.
func (b *builder) hooks(id, rootVar string) error {
	if !b.mods[manifest.ModuleHooks] {
		return nil
	}
	ad, _ := adapter.Get(id)
	ha := ad.(adapter.HooksAdapter)
	events := map[string][]any{}
	used := map[string]bool{}
	for _, name := range b.p.Modules.Hooks {
		h, err := b.p.Hook(name)
		if err != nil {
			return err
		}
		native, ok := ha.HookEvent(h.Event)
		if !ok {
			b.note("hook %s: o evento %s não existe neste agente (pulado)", name, h.Event)
			continue
		}
		if err := b.copyDir(h.Dir, "hooks/atlana/"+b.p.Name+"/"+name); err != nil {
			return err
		}
		if used[h.Event] {
			continue
		}
		used[h.Event] = true
		cmd := fmt.Sprintf(`atlana hook dispatch --agent %s --event %s --dir "%s/hooks/atlana"`, id, h.Event, rootVar)
		events[native] = append(events[native], ha.HookEntry(native, cmd))
	}
	if len(events) == 0 {
		return nil
	}
	b.note("os hooks do artefato chamam `atlana hook dispatch`: quem instalar o plugin precisa do atlana no PATH")
	return b.addJSON("hooks/hooks.json", map[string]any{"hooks": events})
}

// hookFallbacks aplica rule | skip | fail; devolve as rules a gerar.
func (b *builder) hookFallbacks(target string) ([]adapter.RuleFile, error) {
	if !b.mods[manifest.ModuleHooks] {
		return nil, nil
	}
	var rules []adapter.RuleFile
	for _, name := range b.p.Modules.Hooks {
		h, err := b.p.Hook(name)
		if err != nil {
			return nil, err
		}
		switch h.Fallback {
		case "fail":
			return nil, fmt.Errorf("o hook %s exige hooks (fallback: fail), e o %s não tem", name, target)
		case "skip":
			b.note("hook %s pulado: o %s não tem hooks (fallback: skip)", name, target)
		case "rule":
			raw, err := os.ReadFile(h.RulePath())
			if err != nil {
				return nil, err
			}
			rules = append(rules, adapter.RuleFile{Rel: "hooks/" + name + ".md", Content: raw})
		}
	}
	return rules, nil
}

func (b *builder) author() map[string]any {
	if len(b.p.Authors) == 0 {
		return nil
	}
	return map[string]any{"name": b.p.Authors[0]}
}

func (b *builder) manifest(extra map[string]any) map[string]any {
	m := map[string]any{"name": b.p.Name, "version": b.p.Version, "description": b.p.Description}
	if a := b.author(); a != nil {
		m["author"] = a
	}
	if b.p.License != "" {
		m["license"] = b.p.License
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func (b *builder) claude() error {
	if err := b.addJSON(".claude-plugin/plugin.json", b.manifest(nil)); err != nil {
		return err
	}
	if err := b.skills(); err != nil {
		return err
	}
	if err := b.subagents("claude"); err != nil {
		return err
	}
	if err := b.commands("claude"); err != nil {
		return err
	}
	if b.mods[manifest.ModuleRules] {
		b.note("rules: plugins do Claude Code não carregam CLAUDE.md; instale as rules com `atlana install` (pulado)")
	}
	servers, err := b.mcp("claude", "${CLAUDE_PLUGIN_ROOT}")
	if err != nil {
		return err
	}
	if len(servers) > 0 {
		if err := b.addJSON(".mcp.json", map[string]any{"mcpServers": servers}); err != nil {
			return err
		}
	}
	return b.hooks("claude", "${CLAUDE_PLUGIN_ROOT}")
}

func (b *builder) gemini() error {
	rules, err := b.rules()
	if err != nil {
		return err
	}
	extra := map[string]any{}
	if len(rules) > 0 {
		var ctx strings.Builder
		fmt.Fprintf(&ctx, "<!-- Gerado pelo atlana a partir de %s. -->\n", b.p.Name)
		for _, r := range rules {
			fmt.Fprintf(&ctx, "\n<!-- %s -->\n\n%s", r.Rel, strings.TrimRight(string(r.Content), "\n")+"\n")
		}
		b.add("GEMINI.md", []byte(ctx.String()))
		extra["contextFileName"] = "GEMINI.md"
	}
	servers, err := b.mcp("gemini", "${extensionPath}")
	if err != nil {
		return err
	}
	if len(servers) > 0 {
		extra["mcpServers"] = servers
	}
	m := b.manifest(extra)
	delete(m, "author") // a extensão do Gemini não tem esses campos
	delete(m, "license")
	if err := b.addJSON("gemini-extension.json", m); err != nil {
		return err
	}
	if err := b.skills(); err != nil {
		return err
	}
	if err := b.subagents("gemini"); err != nil {
		return err
	}
	if err := b.commands("gemini"); err != nil {
		return err
	}
	return b.hooks("gemini", "${extensionPath}")
}

func (b *builder) cursor() error {
	m := b.manifest(nil)
	delete(m, "license")
	if err := b.addJSON(".cursor-plugin/plugin.json", m); err != nil {
		return err
	}
	if err := b.skills(); err != nil {
		return err
	}
	rules, err := b.rules()
	if err != nil {
		return err
	}
	fallback, err := b.hookFallbacks("Cursor (no plugin)")
	if err != nil {
		return err
	}
	for _, r := range append(rules, fallback...) {
		name, content := adapter.CursorRule(b.p.Name, r)
		b.add("rules/"+name, content)
	}
	if b.mods[manifest.ModuleAgents] {
		b.note("subagents ainda não são gerados para o Cursor (pulado)")
	}
	if b.mods[manifest.ModuleCommands] {
		b.note("commands ainda não são gerados para o plugin do Cursor (pulado)")
	}
	servers, err := b.mcp("cursor", "${PLUGIN_ROOT}")
	if err != nil {
		return err
	}
	for name, s := range servers {
		raw, _ := json.Marshal(s)
		if strings.Contains(string(raw), "${PLUGIN_ROOT}") {
			// O Cursor não documenta uma variável de raiz de plugin.
			delete(servers, name)
			b.note("servidor %s pulado: usa ${PLUGIN_ROOT}, que o plugin do Cursor não expande", name)
		}
	}
	if len(servers) > 0 {
		return b.addJSON("mcp.json", map[string]any{"mcpServers": servers})
	}
	return nil
}

// agentPlugin segue a spec Agent Plugins 1.0.0: só skills e MCP.
func (b *builder) agentPlugin() error {
	m := b.manifest(map[string]any{"$schema": AgentPluginsSchema})
	if err := b.addJSON("plugin.json", m); err != nil {
		return err
	}
	if err := b.skills(); err != nil {
		return err
	}
	for _, mod := range []manifest.Module{manifest.ModuleRules, manifest.ModuleAgents, manifest.ModuleHooks, manifest.ModuleCommands} {
		if b.mods[mod] {
			b.note("%s: a spec Agent Plugins 1.0.0 só tem skills e MCP (pulado)", mod)
		}
	}
	if !b.mods[manifest.ModuleMCP] {
		return nil
	}
	defs, err := b.p.MCPServers()
	if err != nil {
		return err
	}
	servers := map[string]any{}
	for _, name := range b.p.Modules.MCP {
		s, err := agentPluginServer(defs[name])
		if err != nil {
			b.note("servidor %s pulado: %v", name, err)
			continue
		}
		servers[name] = s
	}
	if len(servers) == 0 {
		return nil
	}
	return b.addJSON("mcp.json", map[string]any{"$schema": AgentPluginsMCPSchema, "mcpServers": servers})
}

// agentPluginServer converte para os campos fechados da spec. A spec não expande
// ${VAR} (só ${PLUGIN_ROOT} e ${PLUGIN_DATA} em args, env e cwd) nem aceita segredos.
func agentPluginServer(def map[string]any) (map[string]any, error) {
	hasVar := func(v any) bool {
		raw, _ := json.Marshal(v)
		s := strings.NewReplacer("${PLUGIN_ROOT}", "", "${PLUGIN_DATA}", "").Replace(string(raw))
		return strings.Contains(s, "${")
	}
	if u, ok := def["url"].(string); ok {
		typ := "streamable-http"
		if def["type"] == "sse" {
			typ = "sse"
		}
		out := map[string]any{"type": typ, "url": u}
		if h, ok := def["headers"]; ok {
			if raw, _ := json.Marshal(h); strings.Contains(string(raw), "${") {
				return nil, errors.New("headers com ${VAR}: a spec Agent Plugins não expande variáveis nem aceita segredos")
			}
			out["headers"] = h
		}
		return out, nil
	}
	cmd, _ := def["command"].(string)
	switch {
	case strings.HasPrefix(cmd, "${PLUGIN_ROOT}/"):
		cmd = "./" + strings.TrimPrefix(cmd, "${PLUGIN_ROOT}/")
	case strings.Contains(cmd, "${"):
		return nil, errors.New("command com variável: a spec só aceita nome simples ou ./caminho")
	}
	out := map[string]any{"type": "stdio", "command": cmd}
	for _, k := range []string{"args", "env", "cwd"} {
		v, ok := def[k]
		if !ok {
			continue
		}
		if hasVar(v) {
			return nil, fmt.Errorf("%s com ${VAR}: a spec Agent Plugins não expande variáveis de ambiente", k)
		}
		out[k] = v
	}
	return out, nil
}

// support copia arquivos de apoio do pack (ex.: bin/, scripts/), que servidores
// MCP referenciam pela raiz do plugin. Módulos e o manifesto já foram tratados.
func (b *builder) support() error {
	skip := map[string]bool{manifest.FileName: true, "mcp.json": true, "skills": true, "agents": true, "hooks": true, "commands": true, ".git": true}
	for _, r := range b.p.Modules.Rules {
		skip[strings.SplitN(filepath.ToSlash(filepath.Clean(r)), "/", 2)[0]] = true
	}
	taken := map[string]bool{}
	for _, f := range b.res.Files {
		taken[f.Path] = true
	}
	entries, err := os.ReadDir(b.p.Dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if skip[e.Name()] || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		src := filepath.Join(b.p.Dir, e.Name())
		if e.IsDir() {
			if err := b.copyDir(src, e.Name()); err != nil {
				return err
			}
			continue
		}
		if !taken[e.Name()] && e.Type().IsRegular() {
			b.res.Files = append(b.res.Files, File{Path: e.Name(), Source: src})
		}
	}
	return nil
}

// readme gera um README.md quando o pack não traz um: diretórios de plugins
// (ex.: o do Claude Code) pedem README para listar e revisar o plugin.
func (b *builder) readme() {
	for _, f := range b.res.Files {
		if strings.EqualFold(f.Path, "README.md") {
			return
		}
	}
	var r strings.Builder
	fmt.Fprintf(&r, "# %s\n\n%s\n\n", b.p.Name, b.p.Description)
	fmt.Fprintf(&r, "Versão %s, gerado pelo [atlana](https://github.com/atlana-ai-hq/atlana) (`atlana build --target %s`) a partir de um pack no formato Atlana. Para editar, mude o pack e gere de novo.\n\n", b.p.Version, b.res.Target)
	r.WriteString("## Conteúdo\n\n")
	sections := []struct {
		title string
		mod   manifest.Module
		items []string
	}{
		{"Skills", manifest.ModuleSkills, b.p.Modules.Skills},
		{"Subagents", manifest.ModuleAgents, b.p.Modules.Agents},
		{"Hooks", manifest.ModuleHooks, b.p.Modules.Hooks},
		{"Servidores MCP", manifest.ModuleMCP, b.p.Modules.MCP},
		{"Commands", manifest.ModuleCommands, b.p.Modules.Commands},
		{"Rules", manifest.ModuleRules, b.p.Modules.Rules},
	}
	for _, s := range sections {
		if b.mods[s.mod] && len(s.items) > 0 {
			fmt.Fprintf(&r, "- **%s:** %s\n", s.title, strings.Join(s.items, ", "))
		}
	}
	if len(b.p.Requires.Env) > 0 {
		fmt.Fprintf(&r, "\n## Configuração\n\nDefina estas variáveis de ambiente antes de usar: %s.\n", strings.Join(b.p.Requires.Env, ", "))
	}
	if b.mods[manifest.ModuleHooks] && b.res.Target != AgentPlugin && b.res.Target != CursorPlugin {
		r.WriteString("\n## Hooks\n\nOs hooks rodam comandos na sua máquina e chamam `atlana hook dispatch`; instale o atlana (`go install github.com/atlana-ai-hq/atlana/cmd/atlana@latest`) e deixe-o no PATH.\n")
	}
	if b.p.License != "" {
		fmt.Fprintf(&r, "\n## Licença\n\n%s\n", b.p.License)
	}
	b.add("README.md", []byte(r.String()))
}

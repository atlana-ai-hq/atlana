package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Ferramentas abstratas de subagents; cada adaptador traduz para as do agente.
var AgentTools = []string{"read", "search", "edit", "write", "shell", "shell:readonly", "web", "mcp"}

// Modelos abstratos de subagents.
var AgentModels = []string{"inherit", "fast", "smart"}

// Subagent é agents/<nome>.md: frontmatter canônico e o prompt no corpo.
type Subagent struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
	Model       string   `yaml:"model"`
	Body        string   `yaml:"-"`
}

// AgentFile é o caminho de agents/<nome>.md.
func (p *Pack) AgentFile(name string) string { return filepath.Join(p.Dir, "agents", name+".md") }

// Subagent lê e normaliza agents/<nome>.md (model vazio = inherit).
func (p *Pack) Subagent(name string) (Subagent, error) {
	return ParseSubagent(p.AgentFile(name))
}

// ParseSubagent lê um arquivo de subagent.
func ParseSubagent(path string) (Subagent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Subagent{}, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\ufeff"))
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	fm, err := Frontmatter(raw)
	if err != nil {
		return Subagent{}, err
	}
	var s Subagent
	dec := yaml.NewDecoder(bytes.NewReader(fm))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return Subagent{}, fmt.Errorf("frontmatter inválido: %w", err)
	}
	// raw = "---\n" + fm + "\n---" + corpo
	s.Body = strings.TrimLeft(string(raw[4+len(fm)+4:]), "\n")
	if s.Model == "" {
		s.Model = "inherit"
	}
	return s, nil
}

func checkSubagent(p *Pack, name string, rep *Report) {
	file := p.AgentFile(name)
	if !isFile(file) {
		rep.errorf(filepath.Join(p.Dir, FileName), "agents: %q declarado, mas agents/%s.md não existe", name, name)
		return
	}
	s, err := ParseSubagent(file)
	if err != nil {
		rep.errorf(file, "%v", err)
		return
	}
	if s.Name != name {
		rep.errorf(file, "name %q precisa ser igual ao nome do arquivo (%s)", s.Name, name)
	}
	if strings.TrimSpace(s.Description) == "" {
		rep.errorf(file, "description é obrigatório")
	}
	for _, t := range s.Tools {
		if !contains(AgentTools, t) {
			rep.errorf(file, "tool %q desconhecida (válidas: %s)", t, strings.Join(AgentTools, ", "))
		}
	}
	if !contains(AgentModels, s.Model) {
		rep.errorf(file, "model %q inválido (use inherit, fast ou smart)", s.Model)
	}
	if strings.TrimSpace(s.Body) == "" {
		rep.errorf(file, "o corpo (prompt do subagent) está vazio")
	}
}

// Eventos, ferramentas e fallbacks de hooks canônicos.
var (
	HookEvents    = []string{"pre_tool", "post_tool", "prompt_submit", "session_start", "stop"}
	HookTools     = []string{"shell", "edit", "write", "read", "mcp"}
	HookFallbacks = []string{"rule", "skip", "fail"}
)

// Hook é hooks/<nome>/hook.yaml.
type Hook struct {
	Name  string `yaml:"name"`
	Event string `yaml:"event"`
	Match struct {
		Tool []string `yaml:"tool"`
		Path []string `yaml:"path"`
	} `yaml:"match"`
	Run      string `yaml:"run"`
	Timeout  int    `yaml:"timeout"`
	Fallback string `yaml:"fallback"`

	// Dir é o diretório do hook (não vem do YAML).
	Dir string `yaml:"-"`
}

// HookDir é o diretório hooks/<nome>.
func (p *Pack) HookDir(name string) string { return filepath.Join(p.Dir, "hooks", name) }

// Hook lê hooks/<nome>/hook.yaml.
func (p *Pack) Hook(name string) (Hook, error) { return ParseHook(p.HookDir(name)) }

// ParseHook lê dir/hook.yaml e aplica os padrões (timeout 30, fallback skip).
func ParseHook(dir string) (Hook, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "hook.yaml"))
	if err != nil {
		return Hook{}, err
	}
	var h Hook
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&h); err != nil {
		return Hook{}, fmt.Errorf("hook.yaml inválido: %w", err)
	}
	if h.Timeout == 0 {
		h.Timeout = 30
	}
	if h.Fallback == "" {
		h.Fallback = "skip"
	}
	h.Dir = dir
	return h, nil
}

// RulePath é o texto instalado como rule quando o agente não tem hooks (fallback: rule).
func (h Hook) RulePath() string { return filepath.Join(h.Dir, "rule.md") }

func checkHook(p *Pack, name string, rep *Report) {
	dir := p.HookDir(name)
	file := filepath.Join(dir, "hook.yaml")
	if !isFile(file) {
		rep.errorf(filepath.Join(p.Dir, FileName), "hooks: %q declarado, mas hooks/%s/hook.yaml não existe", name, name)
		return
	}
	h, err := ParseHook(dir)
	if err != nil {
		rep.errorf(file, "%v", err)
		return
	}
	if h.Name != name {
		rep.errorf(file, "name %q precisa ser igual ao diretório (%s)", h.Name, name)
	}
	if !contains(HookEvents, h.Event) {
		rep.errorf(file, "event %q inválido (válidos: %s)", h.Event, strings.Join(HookEvents, ", "))
	}
	for _, t := range h.Match.Tool {
		if !contains(HookTools, t) {
			rep.errorf(file, "match.tool %q inválido (válidos: %s)", t, strings.Join(HookTools, ", "))
		}
	}
	if (len(h.Match.Tool) > 0 || len(h.Match.Path) > 0) && h.Event != "pre_tool" && h.Event != "post_tool" {
		rep.errorf(file, "match só vale para pre_tool e post_tool")
	}
	if h.Run == "" || !safeRel(h.Run) {
		rep.errorf(file, "run %q precisa ser um caminho relativo dentro do hook (ex.: ./run.sh)", h.Run)
	} else if st, err := os.Stat(filepath.Join(dir, h.Run)); err != nil || st.IsDir() {
		rep.errorf(file, "run: %s não existe", h.Run)
	} else if st.Mode()&0o111 == 0 {
		rep.errorf(file, "run: %s precisa ser executável (chmod +x)", h.Run)
	}
	if h.Timeout < 1 || h.Timeout > 60 {
		rep.errorf(file, "timeout precisa estar entre 1 e 60 segundos")
	}
	if !contains(HookFallbacks, h.Fallback) {
		rep.errorf(file, "fallback %q inválido (use rule, skip ou fail)", h.Fallback)
	}
	if h.Fallback == "rule" && !isFile(h.RulePath()) {
		rep.errorf(file, "fallback: rule precisa de hooks/%s/rule.md", name)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

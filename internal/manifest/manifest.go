// Package manifest carrega e valida packs Atlana (atlana.yaml + SKILL.md).
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"

	"github.com/atlana-ai-hq/atlana/spec"
)

// FileName é o nome do manifesto dentro de um pack.
const FileName = "atlana.yaml"

// Module é um tipo de conteúdo que um pack pode oferecer.
type Module string

const (
	ModuleRules    Module = "rules"
	ModuleSkills   Module = "skills"
	ModuleAgents   Module = "agents"
	ModuleHooks    Module = "hooks"
	ModuleCommands Module = "commands"
	ModuleMCP      Module = "mcp"
)

// AllModules lista os módulos na ordem canônica.
var AllModules = []Module{ModuleRules, ModuleSkills, ModuleAgents, ModuleHooks, ModuleCommands, ModuleMCP}

// ParseModule converte texto (ex.: "skills") em Module.
func ParseModule(s string) (Module, error) {
	for _, m := range AllModules {
		if string(m) == s {
			return m, nil
		}
	}
	return "", fmt.Errorf("módulo desconhecido %q (válidos: rules, skills, agents, hooks, commands, mcp)", s)
}

// Pack é a forma tipada de atlana.yaml.
type Pack struct {
	APIVersion  string   `yaml:"apiVersion"`
	Kind        string   `yaml:"kind"`
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	License     string   `yaml:"license"`
	Visibility  string   `yaml:"visibility"`
	Authors     []string `yaml:"authors"`
	Modules     Modules  `yaml:"modules"`
	Requires    Requires `yaml:"requires"`
	Targets     Targets  `yaml:"targets"`

	// Dir é o diretório absoluto do pack (não vem do YAML).
	Dir string `yaml:"-"`
}

// Modules lista o conteúdo de cada módulo.
type Modules struct {
	Rules    []string `yaml:"rules"`
	Skills   []string `yaml:"skills"`
	Agents   []string `yaml:"agents"`
	Hooks    []string `yaml:"hooks"`
	Commands []string `yaml:"commands"`
	MCP      []string `yaml:"mcp"`
}

// Items devolve os itens declarados para um módulo.
func (m Modules) Items(mod Module) []string {
	switch mod {
	case ModuleRules:
		return m.Rules
	case ModuleSkills:
		return m.Skills
	case ModuleAgents:
		return m.Agents
	case ModuleHooks:
		return m.Hooks
	case ModuleCommands:
		return m.Commands
	case ModuleMCP:
		return m.MCP
	}
	return nil
}

// Requires descreve dependências. Env guarda só nomes, nunca valores.
type Requires struct {
	Packs []string `yaml:"packs"`
	Bins  []string `yaml:"bins"`
	Env   []string `yaml:"env"`
}

// Targets restringe ou ajusta agentes.
type Targets struct {
	Include   []string       `yaml:"include"`
	Exclude   []string       `yaml:"exclude"`
	Overrides map[string]any `yaml:"overrides"`
}

// Allows diz se o pack aceita ser instalado no agente.
func (t Targets) Allows(agent string) bool {
	for _, a := range t.Exclude {
		if a == agent {
			return false
		}
	}
	if len(t.Include) == 0 {
		return true
	}
	for _, a := range t.Include {
		if a == agent {
			return true
		}
	}
	return false
}

// SkillDir devolve o diretório de uma skill do pack.
func (p *Pack) SkillDir(name string) string {
	return filepath.Join(p.Dir, "skills", name)
}

// Issue é um problema encontrado na validação.
type Issue struct {
	File    string
	Message string
	Warning bool
}

func (i Issue) String() string {
	level := "erro"
	if i.Warning {
		level = "aviso"
	}
	return fmt.Sprintf("%s: %s: %s", level, i.File, i.Message)
}

// Report agrupa os problemas de um pack.
type Report struct {
	Issues []Issue
}

func (r *Report) errorf(file, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{File: file, Message: fmt.Sprintf(format, args...)})
}

func (r *Report) warnf(file, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{File: file, Message: fmt.Sprintf(format, args...), Warning: true})
}

// Errors conta só os erros (avisos não bloqueiam).
func (r *Report) Errors() int {
	n := 0
	for _, i := range r.Issues {
		if !i.Warning {
			n++
		}
	}
	return n
}

// OK é verdadeiro quando não há erros.
func (r *Report) OK() bool { return r.Errors() == 0 }

// printer formata as mensagens do validador de schema.
var printer = message.NewPrinter(language.English)

var (
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
)

func packSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(spec.PackSchema))
		if err != nil {
			schemaErr = err
			return
		}
		c := jsonschema.NewCompiler()
		const url = "atlana.schema.json"
		if err := c.AddResource(url, doc); err != nil {
			schemaErr = err
			return
		}
		schema, schemaErr = c.Compile(url)
	})
	return schema, schemaErr
}

// ErrInvalid indica que o pack tem erros de validação.
var ErrInvalid = errors.New("pack inválido")

// Load lê e valida um pack. Retorna o pack, o relatório e ErrInvalid se houver erros.
func Load(dir string) (*Pack, *Report, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	rep := &Report{}
	mf := filepath.Join(abs, FileName)
	raw, err := os.ReadFile(mf)
	if err != nil {
		return nil, nil, fmt.Errorf("lendo %s: %w", mf, err)
	}

	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		rep.errorf(mf, "YAML inválido: %v", err)
		return nil, rep, ErrInvalid
	}
	jsonDoc, err := toJSONCompatible(doc)
	if err != nil {
		rep.errorf(mf, "%v", err)
		return nil, rep, ErrInvalid
	}
	sch, err := packSchema()
	if err != nil {
		return nil, nil, fmt.Errorf("compilando schema embutido: %w", err)
	}
	if err := sch.Validate(jsonDoc); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			for _, leaf := range leaves(ve) {
				loc := "/" + strings.Join(leaf.InstanceLocation, "/")
				rep.errorf(mf, "schema %s: %s", loc, leaf.ErrorKind.LocalizedString(printer))
			}
		} else {
			rep.errorf(mf, "schema: %v", err)
		}
		return nil, rep, ErrInvalid
	}

	p := &Pack{}
	if err := yaml.Unmarshal(raw, p); err != nil {
		rep.errorf(mf, "YAML: %v", err)
		return nil, rep, ErrInvalid
	}
	p.Dir = abs
	if p.APIVersion == spec.LegacyAPIVersion {
		rep.warnf(mf, "apiVersion %s está obsoleta; troque por %s (o formato é o mesmo)", spec.LegacyAPIVersion, spec.APIVersion)
	}
	if p.Visibility == "" {
		p.Visibility = "public"
	}

	checkContent(p, rep)
	if !rep.OK() {
		return p, rep, ErrInvalid
	}
	return p, rep, nil
}

func leaves(ve *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(ve.Causes) == 0 {
		return []*jsonschema.ValidationError{ve}
	}
	var out []*jsonschema.ValidationError
	for _, c := range ve.Causes {
		out = append(out, leaves(c)...)
	}
	return out
}

// toJSONCompatible passa o YAML por JSON para normalizar tipos (map[string]any, float64...).
func toJSONCompatible(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("manifesto não é representável em JSON: %v", err)
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(b))
}

func checkContent(p *Pack, rep *Report) {
	mf := filepath.Join(p.Dir, FileName)

	for _, r := range p.Modules.Rules {
		if !safeRel(r) {
			rep.errorf(mf, "rules: caminho %q precisa ser relativo e ficar dentro do pack", r)
			continue
		}
		if !isFile(filepath.Join(p.Dir, r)) {
			rep.errorf(mf, "rules: arquivo %q não existe", r)
		}
	}

	declared := map[string]bool{}
	for _, s := range p.Modules.Skills {
		declared[s] = true
		dir := p.SkillDir(s)
		skillFile := filepath.Join(dir, "SKILL.md")
		if !isFile(skillFile) {
			rep.errorf(mf, "skills: %q declarada, mas %s não existe", s, rel(p.Dir, skillFile))
			continue
		}
		for _, is := range ValidateSkill(dir) {
			rep.Issues = append(rep.Issues, is)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(p.Dir, "skills")); err == nil {
		for _, e := range entries {
			if e.IsDir() && !declared[e.Name()] {
				rep.warnf(mf, "skills/%s existe mas não está em modules.skills (não será instalada)", e.Name())
			}
		}
	}

	for _, a := range p.Modules.Agents {
		checkSubagent(p, a, rep)
	}
	for _, h := range p.Modules.Hooks {
		checkHook(p, h, rep)
	}
	for _, c := range p.Modules.Commands {
		if !isFile(filepath.Join(p.Dir, "commands", c+".md")) {
			rep.errorf(mf, "commands: %q declarado, mas commands/%s.md não existe", c, c)
		}
	}
	if len(p.Modules.MCP) > 0 {
		checkMCP(p, rep)
	}
}

func checkMCP(p *Pack, rep *Report) {
	file := filepath.Join(p.Dir, "mcp.json")
	if !isFile(file) {
		rep.errorf(filepath.Join(p.Dir, FileName), "mcp: módulo declarado, mas mcp.json não existe")
		return
	}
	servers, err := p.MCPServers()
	if err != nil {
		rep.errorf(file, "%v", err)
		return
	}
	declaredEnv := map[string]bool{}
	for _, e := range p.Requires.Env {
		declaredEnv[e] = true
	}
	for _, k := range p.Modules.MCP {
		s, ok := servers[k]
		if !ok {
			rep.errorf(file, "mcpServers não tem a chave %q declarada em modules.mcp", k)
			continue
		}
		_, hasCmd := s["command"].(string)
		_, hasURL := s["url"].(string)
		if hasCmd == hasURL {
			rep.errorf(file, "mcpServers.%s: precisa de exatamente um entre command e url", k)
		}
		for _, field := range []string{"env", "headers"} {
			raw, ok := s[field]
			if !ok {
				continue
			}
			m, ok := raw.(map[string]any)
			if !ok {
				rep.errorf(file, "mcpServers.%s.%s precisa ser um objeto", k, field)
				continue
			}
			for name, v := range m {
				str, _ := v.(string)
				vars, ok := SecretRefs(field, str)
				if !ok {
					rep.errorf(file, "mcpServers.%s.%s.%s: use só referência a variável (${NOME}%s), nunca o valor", k, field, name, map[bool]string{true: " ou Bearer ${NOME}", false: ""}[field == "headers"])
					continue
				}
				for _, vr := range vars {
					if !declaredEnv[vr] {
						rep.warnf(file, "mcpServers.%s.%s.%s usa ${%s}, que não está em requires.env", k, field, name, vr)
					}
				}
			}
		}
	}
}

var (
	envRef    = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
	bearerRef = regexp.MustCompile(`^Bearer \$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
)

// SecretRefs valida um valor de env ou header: só "${NOME}" (e, em headers,
// "Bearer ${NOME}"). Devolve os nomes de variáveis citados.
func SecretRefs(field, v string) ([]string, bool) {
	if m := envRef.FindStringSubmatch(v); m != nil {
		return []string{m[1]}, true
	}
	if field == "headers" {
		if m := bearerRef.FindStringSubmatch(v); m != nil {
			return []string{m[1]}, true
		}
	}
	return nil, false
}

// MCPServers lê mcp.json do pack (formato Agent Plugins: mcpServers).
func (p *Pack) MCPServers() (map[string]map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(p.Dir, "mcp.json"))
	if err != nil {
		return nil, err
	}
	var doc struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("mcp.json inválido: %w", err)
	}
	return doc.MCPServers, nil
}

// Find procura packs (diretórios com atlana.yaml) a partir de root.
// Se root já é um pack, devolve só ele.
func Find(root string) ([]string, error) {
	if isFile(filepath.Join(root, FileName)) {
		return []string{root}, nil
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "testdata", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == FileName {
			out = append(out, filepath.Dir(path))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func safeRel(p string) bool {
	if p == "" || filepath.IsAbs(p) || strings.Contains(p, `\`) {
		return false
	}
	clean := filepath.Clean(p)
	return clean != ".." && !strings.HasPrefix(clean, "../")
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func rel(base, p string) string {
	if r, err := filepath.Rel(base, p); err == nil {
		return r
	}
	return p
}

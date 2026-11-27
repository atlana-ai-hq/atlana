package adapter

import (
	"path/filepath"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// Exporter é implementado por adaptadores de agentes sem pasta local de skills.
// A "instalação" vira uma exportação no formato Agent Skills, que a pessoa
// envia ao agente por conta própria. Planner, lock, status e uninstall
// funcionam igual: só a raiz muda. Ver ADR 0002.
type Exporter interface {
	ExportOnly() bool
}

// IsExporter diz se o adaptador só exporta.
func IsExporter(a Adapter) bool {
	e, ok := a.(Exporter)
	return ok && e.ExportOnly()
}

// grok: Grok Bot não tem pasta local de skills; o v1 exporta as pastas SKILL.md
// para upload. Sem MCP remoto e sem sync automático.
//
//	user:    $XDG_CONFIG_HOME/atlana/exports/grok/skills (padrão ~/.config/...)
//	project: <projeto>/.atlana/exports/grok/skills
type grok struct{}

func (grok) ID() string       { return "grok" }
func (grok) Name() string     { return "Grok Bot" }
func (grok) ExportOnly() bool { return true }
func (grok) Detect(env Env) Detection {
	return detectBy(env, []string{"grok"}, []string{filepath.Join(env.Home, ".grok")})
}
func (grok) Supports(m manifest.Module) bool { return m == manifest.ModuleSkills }
func (grok) SkillsRoot(scope Scope, env Env) (string, error) {
	if scope == ScopeUser {
		base := ""
		if env.Getenv != nil {
			base = env.Getenv("XDG_CONFIG_HOME")
		}
		if base == "" {
			if env.Home == "" {
				return scopedDir(scope, env) // devolve o erro de base vazia
			}
			base = filepath.Join(env.Home, ".config")
		}
		return filepath.Join(base, "atlana", "exports", "grok", "skills"), nil
	}
	return scopedDir(scope, env, ".atlana", "exports", "grok", "skills")
}

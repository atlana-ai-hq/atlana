package adapter

import (
	"path/filepath"

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

// opencode: skills em ~/.config/opencode/skills e <projeto>/.opencode/skills,
// os únicos caminhos iguais nos formatos V1 e V2 do OpenCode. MCP, agents e
// rules mudaram de forma entre as versões e ficam para depois (ADR 0009).
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
func (opencode) Supports(m manifest.Module) bool { return m == manifest.ModuleSkills }
func (opencode) SkillsRoot(scope Scope, env Env) (string, error) {
	if scope != ScopeUser {
		return scopedDir(scope, env, ".opencode", "skills")
	}
	c, err := configHome(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(c, "opencode", "skills"), nil
}

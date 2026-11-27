package adapter

import (
	"errors"
	"path/filepath"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// ErrUnsupported indica que o adaptador ainda não instala o módulo pedido.
var ErrUnsupported = errors.New("módulo ainda não suportado por este adaptador")

// cursor: skills em ~/.cursor/skills ou <projeto>/.cursor/skills.
type cursor struct{}

func (cursor) ID() string   { return "cursor" }
func (cursor) Name() string { return "Cursor" }
func (cursor) Detect(env Env) Detection {
	return detectBy(env, []string{"cursor", "cursor-agent"}, []string{filepath.Join(env.Home, ".cursor")})
}
func (cursor) Supports(m manifest.Module) bool { return m == manifest.ModuleSkills }
func (cursor) SkillsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".cursor", "skills")
}

// claude: skills em ~/.claude/skills ou <projeto>/.claude/skills.
type claude struct{}

func (claude) ID() string   { return "claude" }
func (claude) Name() string { return "Claude Code" }
func (claude) Detect(env Env) Detection {
	return detectBy(env, []string{"claude"}, []string{filepath.Join(env.Home, ".claude")})
}
func (claude) Supports(m manifest.Module) bool { return m == manifest.ModuleSkills }
func (claude) SkillsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".claude", "skills")
}

// codex: por enquanto só detecção. Respeita $CODEX_HOME.
type codex struct{}

func (codex) ID() string   { return "codex" }
func (codex) Name() string { return "Codex CLI" }
func (codex) Detect(env Env) Detection {
	dir := filepath.Join(env.Home, ".codex")
	if env.Getenv != nil {
		if h := env.Getenv("CODEX_HOME"); h != "" {
			dir = h
		}
	}
	return detectBy(env, []string{"codex"}, []string{dir})
}
func (codex) Supports(manifest.Module) bool { return false }
func (codex) SkillsRoot(Scope, Env) (string, error) {
	return "", ErrUnsupported
}

// gemini: skills em ~/.gemini/skills ou <projeto>/.gemini/skills (Agent Skills nativo).
type gemini struct{}

func (gemini) ID() string   { return "gemini" }
func (gemini) Name() string { return "Gemini CLI" }
func (gemini) Detect(env Env) Detection {
	return detectBy(env, []string{"gemini"}, []string{filepath.Join(env.Home, ".gemini")})
}
func (gemini) Supports(m manifest.Module) bool { return m == manifest.ModuleSkills }
func (gemini) SkillsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".gemini", "skills")
}

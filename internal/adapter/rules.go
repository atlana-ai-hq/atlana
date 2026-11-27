package adapter

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RuleFile é um arquivo de rules do pack (ex.: AGENTS.md), já lido.
type RuleFile struct {
	Rel     string // caminho relativo ao pack, com "/"
	Content []byte
}

// RuleOutput é um arquivo que o adaptador quer gravar para as rules.
type RuleOutput struct {
	Path    string
	Content []byte
}

// RulesAdapter é implementado por adaptadores que instalam rules.
//
// Rules nunca vão para CLAUDE.md, GEMINI.md ou AGENTS.md da pessoa: cada
// adaptador grava só arquivos próprios, dentro de RulesRoot (ADR 0003).
type RulesAdapter interface {
	// RulesRoot é o diretório onde ficam os arquivos de rules (limite da limpeza de pastas vazias).
	RulesRoot(scope Scope, env Env) (string, error)
	// Rules traduz as rules do pack em arquivos e em avisos para a pessoa.
	Rules(pack string, files []RuleFile, scope Scope, env Env) ([]RuleOutput, []string, error)
}

// copyRules grava cada rule sem mudanças em <root>/atlana-<pack>/<rel>.
func copyRules(root, pack string, files []RuleFile) []RuleOutput {
	out := make([]RuleOutput, 0, len(files))
	for _, f := range files {
		out = append(out, RuleOutput{Path: filepath.Join(root, "atlana-"+pack, filepath.FromSlash(f.Rel)), Content: f.Content})
	}
	return out
}

// claude: ~/.claude/rules ou <projeto>/.claude/rules, que o Claude Code carrega sozinho.
func (claude) RulesRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".claude", "rules")
}
func (c claude) Rules(pack string, files []RuleFile, scope Scope, env Env) ([]RuleOutput, []string, error) {
	root, err := c.RulesRoot(scope, env)
	if err != nil {
		return nil, nil, err
	}
	return copyRules(root, pack, files), nil, nil
}

// cursor: <projeto>/.cursor/rules/atlana-<pack>-<slug>.mdc com alwaysApply.
// Rules de usuário do Cursor ficam nas configurações do app, não em arquivo.
func (cursor) RulesRoot(scope Scope, env Env) (string, error) {
	if scope != ScopeProject {
		return "", fmt.Errorf("rules do Cursor só no escopo project (as de usuário ficam nas configurações do app): %w", ErrUnsupported)
	}
	return scopedDir(scope, env, ".cursor", "rules")
}
func (c cursor) Rules(pack string, files []RuleFile, scope Scope, env Env) ([]RuleOutput, []string, error) {
	root, err := c.RulesRoot(scope, env)
	if err != nil {
		return nil, nil, err
	}
	out := make([]RuleOutput, 0, len(files))
	for _, f := range files {
		slug := strings.ReplaceAll(strings.TrimSuffix(f.Rel, filepath.Ext(f.Rel)), "/", "-")
		body := fmt.Sprintf("---\ndescription: %s (%s, instalado pelo atlana)\nalwaysApply: true\n---\n\n%s", f.Rel, pack, stripFrontmatter(f.Content))
		out = append(out, RuleOutput{Path: filepath.Join(root, "atlana-"+pack+"-"+slug+".mdc"), Content: []byte(body)})
	}
	return out, nil, nil
}

// gemini: <base>/.gemini/atlana/<pack>/; a pessoa importa com @ no GEMINI.md.
func (gemini) RulesRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".gemini", "atlana")
}
func (g gemini) Rules(pack string, files []RuleFile, scope Scope, env Env) ([]RuleOutput, []string, error) {
	root, err := g.RulesRoot(scope, env)
	if err != nil {
		return nil, nil, err
	}
	out := copyRules(root, pack, files)
	gem, prefix := "GEMINI.md na raiz do projeto", ".gemini/atlana/"
	if scope == ScopeUser {
		gem, prefix = "~/.gemini/GEMINI.md", "atlana/"
	}
	var notes []string
	for _, f := range files {
		notes = append(notes, fmt.Sprintf("gemini: para ativar, adicione `@%satlana-%s/%s` ao %s", prefix, pack, f.Rel, gem))
	}
	return out, notes, nil
}

// codex: <CODEX_HOME|~/.codex>/atlana ou <projeto>/.codex/atlana. O AGENTS.md do
// Codex não importa outros arquivos, então a pessoa referencia à mão.
func (codex) RulesRoot(scope Scope, env Env) (string, error) {
	if scope == ScopeUser && env.Getenv != nil {
		if h := env.Getenv("CODEX_HOME"); h != "" {
			return filepath.Join(h, "atlana"), nil
		}
	}
	return scopedDir(scope, env, ".codex", "atlana")
}
func (c codex) Rules(pack string, files []RuleFile, scope Scope, env Env) ([]RuleOutput, []string, error) {
	root, err := c.RulesRoot(scope, env)
	if err != nil {
		return nil, nil, err
	}
	out := copyRules(root, pack, files)
	notes := []string{fmt.Sprintf("codex: o AGENTS.md não importa arquivos; referencie ou copie o conteúdo de %s no seu AGENTS.md", filepath.Join(root, "atlana-"+pack))}
	return out, notes, nil
}

// stripFrontmatter remove um frontmatter YAML inicial, para não duplicar o do .mdc.
func stripFrontmatter(b []byte) string {
	s := string(b)
	if !strings.HasPrefix(s, "---\n") {
		return s
	}
	if i := strings.Index(s[4:], "\n---\n"); i >= 0 {
		return strings.TrimLeft(s[4+i+5:], "\n")
	}
	return s
}

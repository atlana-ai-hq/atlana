package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// CommandsAdapter é implementado por adaptadores que instalam slash commands.
type CommandsAdapter interface {
	CommandsRoot(scope Scope, env Env) (string, error)
	// Command traduz o command canônico: nome do arquivo em CommandsRoot,
	// conteúdo e avisos.
	Command(c manifest.Command) (file string, content []byte, notes []string, err error)
}

// markdownCommand: frontmatter (description, argument-hint) + corpo com $ARGUMENTS.
func markdownCommand(c manifest.Command, extra map[string]any) ([]byte, error) {
	front := map[string]any{"description": c.Description}
	order := []string{"description"}
	if c.ArgumentHint != "" {
		front["argument-hint"] = c.ArgumentHint
		order = append(order, "argument-hint")
	}
	for k, v := range extra {
		front[k] = v
		order = append(order, k)
	}
	return renderAgent(front, order, c.Body)
}

// claude: .claude/commands/<nome>.md.
func (claude) CommandsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".claude", "commands")
}
func (claude) Command(c manifest.Command) (string, []byte, []string, error) {
	b, err := markdownCommand(c, nil)
	return c.Name + ".md", b, nil, err
}

// opencode: commands/<nome>.md no diretório de config (o binário lê
// {command,commands}/**/*.md e expande $ARGUMENTS e $1…).
func (opencode) CommandsRoot(scope Scope, env Env) (string, error) {
	return opencodeDir(scope, env, ".opencode", "commands")
}
func (opencode) Command(c manifest.Command) (string, []byte, []string, error) {
	b, err := markdownCommand(manifest.Command{Name: c.Name, Description: c.Description, Body: c.Body}, nil)
	return c.Name + ".md", b, nil, err
}

// cursor: .cursor/commands/<nome>.md, Markdown puro. O Cursor anexa o texto
// digitado depois do comando; não há placeholder documentado.
func (cursor) CommandsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".cursor", "commands")
}
func (cursor) Command(c manifest.Command) (string, []byte, []string, error) {
	var notes []string
	if strings.Contains(c.Body, "$ARGUMENTS") || manifest.Positional.MatchString(c.Body) {
		notes = append(notes, fmt.Sprintf("cursor: o command %s usa $ARGUMENTS/$1…, que o Cursor não expande; o texto digitado depois do comando é anexado ao prompt", c.Name))
	}
	return c.Name + ".md", []byte(c.Body), notes, nil
}

// gemini: .gemini/commands/<nome>.toml com description e prompt; $ARGUMENTS vira {{args}}.
func (gemini) CommandsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".gemini", "commands")
}
func (gemini) Command(c manifest.Command) (string, []byte, []string, error) {
	var notes []string
	if manifest.Positional.MatchString(c.Body) {
		notes = append(notes, fmt.Sprintf("gemini: o command %s usa $1…$9, que o Gemini CLI não expande; use $ARGUMENTS", c.Name))
	}
	prompt := strings.ReplaceAll(c.Body, "$ARGUMENTS", "{{args}}")
	var b strings.Builder
	fmt.Fprintf(&b, "# Gerado pelo atlana a partir de commands/%s.md.\n", c.Name)
	fmt.Fprintf(&b, "description = %s\n", tomlString(c.Description))
	fmt.Fprintf(&b, "prompt = %s\n", tomlString(prompt))
	return c.Name + ".toml", []byte(b.String()), notes, nil
}

// tomlString: string básica do TOML. Os escapes do JSON (\" \\ \n \t \uXXXX)
// valem também no TOML, sem escapar <, > e &.
func tomlString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

package adapter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// writes diz se a lista abstrata pede alguma ferramenta que muda arquivos.
func writes(tools []string) bool {
	return slices.Contains(tools, "edit") || slices.Contains(tools, "write")
}

// cursor: subagents em .cursor/agents/<nome>.md. O Cursor não tem lista de
// ferramentas: o único controle é readonly (sem edição e sem shell que muda
// estado), que vale quando o pack não pede edit, write nem shell.
func (cursor) AgentsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".cursor", "agents")
}
func (cursor) Subagent(s manifest.Subagent) (string, []byte, []string, error) {
	front := map[string]any{"name": s.Name, "description": s.Description, "model": "inherit"}
	var notes []string
	if len(s.Tools) > 0 {
		switch {
		case !writes(s.Tools) && !slices.Contains(s.Tools, "shell"):
			front["readonly"] = true
		case !writes(s.Tools):
			notes = append(notes, fmt.Sprintf("cursor: o subagent %s pede shell sem edit/write; o Cursor não separa os dois, então ele fica com todas as ferramentas da sessão", s.Name))
		default:
			notes = append(notes, fmt.Sprintf("cursor: o Cursor não restringe ferramentas por lista; o subagent %s fica com todas as da sessão", s.Name))
		}
	}
	if s.Model != "inherit" {
		notes = append(notes, fmt.Sprintf("cursor: model %q não tem alias estável; o subagent %s usa o modelo da sessão", s.Model, s.Name))
	}
	b, err := renderAgent(front, []string{"name", "description", "model", "readonly"}, s.Body)
	return s.Name + ".md", b, notes, err
}

// codex: subagents em .codex/agents/<nome>.toml (name, description,
// developer_instructions). O sandbox e as aprovações vêm sempre da sessão pai;
// o arquivo só consegue desligar ferramentas (features.shell_tool).
func (codex) AgentsRoot(scope Scope, env Env) (string, error) {
	return scopedDir(scope, env, ".codex", "agents")
}
func (codex) Subagent(s manifest.Subagent) (string, []byte, []string, error) {
	var notes []string
	body := s.Body
	var b strings.Builder
	fmt.Fprintf(&b, "name = %s\ndescription = %s\n", tomlString(s.Name), tomlString(s.Description))
	if len(s.Tools) > 0 {
		if !writes(s.Tools) {
			body = "Não edite nem crie arquivos.\n\n" + body
			notes = append(notes, fmt.Sprintf("codex: o Codex não bloqueia edição num subagent (o sandbox é o da sessão); o subagent %s recebe a instrução de não editar", s.Name))
		}
		if !slices.Contains(s.Tools, "shell") && !slices.Contains(s.Tools, "shell:readonly") {
			b.WriteString("\n[features]\nshell_tool = false\n")
		} else if slices.Contains(s.Tools, "shell:readonly") && !slices.Contains(s.Tools, "shell") {
			notes = append(notes, fmt.Sprintf("codex: shell:readonly não tem equivalente; o subagent %s fica com o shell da sessão", s.Name))
		}
	}
	if s.Model != "inherit" {
		notes = append(notes, fmt.Sprintf("codex: model %q não tem alias estável; o subagent %s usa o modelo da sessão", s.Model, s.Name))
	}
	// developer_instructions precisa vir antes de qualquer tabela.
	out := strings.Replace(b.String(), "\n[features]", "developer_instructions = "+tomlString(body)+"\n\n[features]", 1)
	if !strings.Contains(out, "developer_instructions") {
		out += "developer_instructions = " + tomlString(body) + "\n"
	}
	return s.Name + ".toml", []byte(out), notes, nil
}

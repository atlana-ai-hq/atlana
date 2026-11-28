package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Command é um slash command canônico (commands/<nome>.md). O corpo usa
// $ARGUMENTS para o texto digitado depois do comando.
type Command struct {
	Name         string `yaml:"-"`
	Description  string `yaml:"description"`
	ArgumentHint string `yaml:"argument-hint"`
	Body         string `yaml:"-"`
}

// Positional casa $1…$9, que só alguns agentes entendem.
var Positional = regexp.MustCompile(`\$[1-9]`)

// CommandFile é o caminho de commands/<nome>.md.
func (p *Pack) CommandFile(name string) string {
	return filepath.Join(p.Dir, "commands", name+".md")
}

// Command lê commands/<nome>.md.
func (p *Pack) Command(name string) (Command, error) {
	return ParseCommand(p.CommandFile(name), name)
}

// ParseCommand lê um arquivo de command; o nome vem do arquivo.
func ParseCommand(path, name string) (Command, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Command{}, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\ufeff"))
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	fm, err := Frontmatter(raw)
	if err != nil {
		return Command{}, fmt.Errorf("o command precisa de frontmatter com description: %w", err)
	}
	c := Command{Name: name}
	dec := yaml.NewDecoder(bytes.NewReader(fm))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Command{}, fmt.Errorf("frontmatter inválido: %w", err)
	}
	c.Body = strings.TrimLeft(string(raw[4+len(fm)+4:]), "\n")
	return c, nil
}

func checkCommand(p *Pack, name string, rep *Report) {
	file := p.CommandFile(name)
	if !isFile(file) {
		rep.errorf(filepath.Join(p.Dir, FileName), "commands: %q declarado, mas commands/%s.md não existe", name, name)
		return
	}
	c, err := ParseCommand(file, name)
	if err != nil {
		rep.errorf(file, "%v", err)
		return
	}
	if strings.TrimSpace(c.Description) == "" {
		rep.errorf(file, "description é obrigatório")
	}
	if strings.TrimSpace(c.Body) == "" {
		rep.errorf(file, "o corpo (prompt do command) está vazio")
	}
	if Positional.MatchString(c.Body) {
		rep.warnf(file, "$1…$9 só funcionam no Claude Code e no OpenCode; prefira $ARGUMENTS")
	}
}

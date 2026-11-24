package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Regras do frontmatter de SKILL.md segundo a especificação Agent Skills
// (https://agentskills.io/specification).
const (
	maxNameLen          = 64
	maxDescriptionLen   = 1024
	maxCompatibilityLen = 500
)

var nameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var knownSkillKeys = map[string]bool{
	"name": true, "description": true, "license": true,
	"compatibility": true, "metadata": true, "allowed-tools": true,
}

// ValidName aplica a regra de nome de Agent Skills (também usada para packs).
func ValidName(s string) bool {
	return len(s) >= 1 && len(s) <= maxNameLen && nameRE.MatchString(s)
}

// Frontmatter extrai o bloco YAML entre os delimitadores "---" do início do arquivo.
func Frontmatter(content []byte) ([]byte, error) {
	content = bytes.TrimPrefix(content, []byte("\ufeff"))
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return nil, fmt.Errorf("SKILL.md precisa começar com frontmatter YAML (---)")
	}
	rest := content[4:]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, fmt.Errorf("frontmatter sem o delimitador de fechamento (---)")
	}
	after := rest[end+4:]
	if len(after) > 0 && after[0] != '\n' {
		return nil, fmt.Errorf("delimitador de fechamento do frontmatter malformado")
	}
	return rest[:end], nil
}

// ValidateSkill valida skillDir/SKILL.md. O nome do diretório precisa bater com `name`.
func ValidateSkill(skillDir string) []Issue {
	file := filepath.Join(skillDir, "SKILL.md")
	var issues []Issue
	errf := func(format string, a ...any) {
		issues = append(issues, Issue{File: file, Message: fmt.Sprintf(format, a...)})
	}
	warnf := func(format string, a ...any) {
		issues = append(issues, Issue{File: file, Message: fmt.Sprintf(format, a...), Warning: true})
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		errf("não foi possível ler: %v", err)
		return issues
	}
	fm, err := Frontmatter(raw)
	if err != nil {
		errf("%v", err)
		return issues
	}
	var meta map[string]any
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		errf("frontmatter YAML inválido: %v", err)
		return issues
	}

	name, ok := meta["name"].(string)
	switch {
	case !ok || name == "":
		errf("campo obrigatório `name` ausente ou não é texto")
	case !ValidName(name):
		errf("`name` %q inválido: use 1-%d caracteres a-z, 0-9 e hífen, sem hífen no início, no fim ou duplo", name, maxNameLen)
	case name != filepath.Base(skillDir):
		errf("`name` %q precisa ser igual ao nome do diretório %q", name, filepath.Base(skillDir))
	}

	desc, ok := meta["description"].(string)
	switch {
	case !ok || desc == "":
		errf("campo obrigatório `description` ausente ou vazio")
	case utf8.RuneCountInString(desc) > maxDescriptionLen:
		errf("`description` tem %d caracteres (máximo %d)", utf8.RuneCountInString(desc), maxDescriptionLen)
	}

	if v, ok := meta["compatibility"]; ok {
		s, isStr := v.(string)
		if !isStr || s == "" || utf8.RuneCountInString(s) > maxCompatibilityLen {
			errf("`compatibility` precisa ser texto de 1-%d caracteres", maxCompatibilityLen)
		}
	}
	if v, ok := meta["metadata"]; ok {
		m, isMap := v.(map[string]any)
		if !isMap {
			errf("`metadata` precisa ser um mapa de texto para texto")
		} else {
			for k, mv := range m {
				if _, isStr := mv.(string); !isStr {
					errf("`metadata.%s` precisa ser texto", k)
				}
			}
		}
	}
	if v, ok := meta["allowed-tools"]; ok {
		if _, isStr := v.(string); !isStr {
			errf("`allowed-tools` precisa ser texto (lista separada por espaço)")
		}
	}
	if v, ok := meta["license"]; ok {
		if _, isStr := v.(string); !isStr {
			errf("`license` precisa ser texto")
		}
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !knownSkillKeys[k] {
			warnf("campo %q não faz parte da spec Agent Skills; alguns agentes vão ignorar", k)
		}
	}
	return issues
}

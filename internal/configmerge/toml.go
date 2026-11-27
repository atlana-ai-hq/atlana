package configmerge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// tomlDoc edita só tabelas [<section>.<nome>] (e subtabelas) de um TOML.
// Outras formas de declarar a seção (inline, chaves com ponto) viram erro, para
// nunca gerar chave duplicada num arquivo que não entendemos por inteiro.
type tomlDoc struct {
	text    string
	section string
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func parseTOML(raw []byte, section string) (*tomlDoc, error) {
	text := string(raw)
	q := regexp.QuoteMeta(section)
	other := regexp.MustCompile(`(?m)^[ \t]*(\[[ \t]*` + q + `[ \t]*\]|` + q + `[ \t]*=|` + q + `\.[^\n=\]]*=)`)
	if other.MatchString(text) {
		return nil, fmt.Errorf("%s está declarado de um jeito que o atlana não edita (use tabelas [%s.<nome>])", section, section)
	}
	return &tomlDoc{text: text, section: section}, nil
}

func quoteKey(name string) string {
	if bareKey.MatchString(name) {
		return name
	}
	b, _ := json.Marshal(name)
	return string(b)
}

// span acha o bloco do servidor: o cabeçalho e as linhas até a próxima tabela que não é dele.
func (d *tomlDoc) span(name string) (int, int, bool) {
	quoted, _ := json.Marshal(name)
	keys := []string{regexp.QuoteMeta(name), regexp.QuoteMeta(string(quoted))}
	head := regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*` + regexp.QuoteMeta(d.section) + `\.(` + strings.Join(keys, "|") + `)[ \t]*\][ \t]*(#.*)?$`)
	loc := head.FindStringIndex(d.text)
	if loc == nil {
		return 0, 0, false
	}
	start := loc[0]
	pos := loc[1]
	sub := regexp.MustCompile(`^[ \t]*\[[ \t]*` + regexp.QuoteMeta(d.section) + `\.(` + strings.Join(keys, "|") + `)\.`)
	for pos < len(d.text) {
		nl := strings.IndexByte(d.text[pos:], '\n')
		lineStart := pos
		if nl < 0 {
			pos = len(d.text)
		} else {
			pos += nl + 1
		}
		line := strings.TrimSpace(d.text[lineStart:pos])
		if lineStart != loc[1] && strings.HasPrefix(line, "[") && !sub.MatchString(line) {
			return start, lineStart, true
		}
	}
	return start, len(d.text), true
}

// canonTOML ignora espaços, linhas vazias e comentários de linha inteira.
func canonTOML(block string) string {
	var lines []string
	for _, l := range strings.Split(block, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}

func (d *tomlDoc) Hash(name string) (string, bool) {
	s, e, ok := d.span(name)
	if !ok {
		return "", false
	}
	return sum(canonTOML(d.text[s:e])), true
}

func (d *tomlDoc) render(name string, v map[string]any) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s.%s]\n", d.section, quoteKey(name))
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val, err := tomlValue(v[k])
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", name, k, err)
		}
		fmt.Fprintf(&b, "%s = %s\n", quoteKey(k), val)
	}
	return b.String(), nil
}

func tomlValue(v any) (string, error) {
	switch x := v.(type) {
	case string:
		b, _ := json.Marshal(x) // string básica TOML é compatível com o escape JSON
		return string(b), nil
	case bool:
		return fmt.Sprint(x), nil
	case int, int64, float64, json.Number:
		return fmt.Sprint(x), nil
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		return tomlValue(items)
	case []any:
		parts := make([]string, 0, len(x))
		for _, it := range x {
			s, err := tomlValue(it)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case map[string]string:
		m := map[string]any{}
		for k, s := range x {
			m[k] = s
		}
		return tomlValue(m)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			s, err := tomlValue(x[k])
			if err != nil {
				return "", err
			}
			parts = append(parts, quoteKey(k)+" = "+s)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	}
	return "", fmt.Errorf("tipo %T não suportado em TOML", v)
}

func (d *tomlDoc) HashOf(name string, v map[string]any) (string, error) {
	block, err := d.render(name, v)
	if err != nil {
		return "", err
	}
	return sum(canonTOML(block)), nil
}

func (d *tomlDoc) Set(name string, v map[string]any) error {
	block, err := d.render(name, v)
	if err != nil {
		return err
	}
	if s, e, ok := d.span(name); ok {
		rest := d.text[e:]
		if rest != "" {
			block += "\n"
		}
		d.text = d.text[:s] + block + rest
		return nil
	}
	switch {
	case d.text == "":
	case strings.HasSuffix(d.text, "\n\n"):
	case strings.HasSuffix(d.text, "\n"):
		d.text += "\n"
	default:
		d.text += "\n\n"
	}
	d.text += block
	return nil
}

func (d *tomlDoc) Delete(name string) {
	s, e, ok := d.span(name)
	if !ok {
		return
	}
	d.text = d.text[:s] + strings.TrimLeft(d.text[e:], "\n")
	d.text = strings.TrimRight(d.text, "\n")
	if d.text != "" {
		d.text += "\n"
	}
}

func (d *tomlDoc) Bytes() []byte { return []byte(d.text) }

package configmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Match diz se um item de array (já decodificado) é o nosso para a chave name.
type Match func(name string, item map[string]any) bool

// arrayDoc edita um item por chave dentro de <section>.<name>: [ ... ] num JSON,
// como hooks.PreToolUse: [{matcher, hooks: [...]}]. Os outros itens do array e
// as outras chaves ficam como estavam; o nosso item é achado por match.
type arrayDoc struct {
	*jsonDoc
	match Match
}

// ParseArrayItems abre raw para editar itens de array em section (só JSON).
func ParseArrayItems(raw []byte, section string, match Match) (Doc, error) {
	d, err := parseJSON(raw, section)
	if err != nil {
		return nil, err
	}
	for _, k := range d.servers.keys {
		var items []json.RawMessage
		if err := json.Unmarshal(d.servers.vals[k], &items); err != nil {
			return nil, fmt.Errorf("%s.%s precisa ser uma lista: %w", section, k, err)
		}
	}
	return &arrayDoc{jsonDoc: d, match: match}, nil
}

func (d *arrayDoc) items(name string) []json.RawMessage {
	var items []json.RawMessage
	if raw, ok := d.servers.vals[name]; ok {
		_ = json.Unmarshal(raw, &items) // validado em ParseArrayItems
	}
	return items
}

func (d *arrayDoc) find(name string) ([]json.RawMessage, int) {
	items := d.items(name)
	for i, it := range items {
		var m map[string]any
		if json.Unmarshal(it, &m) == nil && d.match(name, m) {
			return items, i
		}
	}
	return items, -1
}

func (d *arrayDoc) Hash(name string) (string, bool) {
	items, i := d.find(name)
	if i < 0 {
		return "", false
	}
	c, err := canonJSON(items[i])
	if err != nil {
		return sum(string(items[i])), true
	}
	return sum(c), true
}

func (d *arrayDoc) put(name string, items []json.RawMessage) {
	if len(items) == 0 {
		d.servers.delete(name)
		return
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, it := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(it)
	}
	buf.WriteByte(']')
	d.servers.set(name, buf.Bytes())
}

func (d *arrayDoc) Set(name string, v map[string]any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	items, i := d.find(name)
	if i < 0 {
		items = append(items, b)
	} else {
		items[i] = b
	}
	d.put(name, items)
	return nil
}

func (d *arrayDoc) Delete(name string) {
	items, i := d.find(name)
	if i < 0 {
		return
	}
	d.put(name, append(items[:i], items[i+1:]...))
}

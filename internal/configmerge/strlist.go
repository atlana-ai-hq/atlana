package configmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// stringListDoc edita a presença de strings numa lista do topo de um JSON, como
// "instructions": [...] do OpenCode. O nome de cada item é a própria string;
// os outros itens (strings ou não) e as outras chaves ficam como estavam.
type stringListDoc struct {
	top   *object
	key   string
	items []json.RawMessage
}

// ParseStringList abre raw (nil ou vazio = arquivo novo) para editar a lista key.
func ParseStringList(raw []byte, key string) (Doc, error) {
	top, err := parseObject(raw)
	if err != nil {
		return nil, fmt.Errorf("JSON inválido: %w", err)
	}
	d := &stringListDoc{top: top, key: key}
	if v, ok := top.vals[key]; ok && string(bytes.TrimSpace(v)) != "null" {
		if err := json.Unmarshal(v, &d.items); err != nil {
			return nil, fmt.Errorf("%s precisa ser uma lista: %w", key, err)
		}
	}
	return d, nil
}

func (d *stringListDoc) find(name string) int {
	for i, it := range d.items {
		var s string
		if json.Unmarshal(it, &s) == nil && s == name {
			return i
		}
	}
	return -1
}

func (d *stringListDoc) Hash(name string) (string, bool) {
	if d.find(name) < 0 {
		return "", false
	}
	return sum(name), true
}

func (d *stringListDoc) HashOf(name string, _ map[string]any) (string, error) {
	return sum(name), nil
}

func (d *stringListDoc) Set(name string, _ map[string]any) error {
	if d.find(name) >= 0 {
		return nil
	}
	b, err := json.Marshal(name)
	if err != nil {
		return err
	}
	d.items = append(d.items, b)
	return nil
}

func (d *stringListDoc) Delete(name string) {
	if i := d.find(name); i >= 0 {
		d.items = append(d.items[:i], d.items[i+1:]...)
	}
}

func (d *stringListDoc) Bytes() []byte {
	if len(d.items) == 0 {
		d.top.delete(d.key)
	} else {
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, it := range d.items {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.Write(it)
		}
		buf.WriteByte(']')
		d.top.set(d.key, buf.Bytes())
	}
	var out bytes.Buffer
	if err := json.Indent(&out, d.top.marshal(), "", "  "); err != nil {
		panic(err) // só juntamos JSON já validado
	}
	out.WriteByte('\n')
	return out.Bytes()
}

package configmerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// object é um objeto JSON que lembra a ordem das chaves e guarda os valores crus.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func parseObject(raw []byte) (*object, error) {
	o := &object{vals: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(raw)) == 0 {
		return o, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("o topo precisa ser um objeto JSON")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		o.set(kt.(string), v)
	}
	if _, err := dec.Token(); err != nil { // '}'
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("conteúdo depois do objeto JSON")
	}
	return o, nil
}

func (o *object) set(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *object) delete(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

func (o *object) marshal() []byte {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(o.vals[k])
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

type jsonDoc struct {
	top     *object
	section string
	servers *object
	deleted bool // a seção ficou vazia por remoção nossa: some do arquivo
}

func parseJSON(raw []byte, section string) (*jsonDoc, error) {
	top, err := parseObject(raw)
	if err != nil {
		return nil, fmt.Errorf("JSON inválido: %w", err)
	}
	d := &jsonDoc{top: top, section: section, servers: &object{vals: map[string]json.RawMessage{}}}
	if v, ok := top.vals[section]; ok && string(bytes.TrimSpace(v)) != "null" {
		if d.servers, err = parseObject(v); err != nil {
			return nil, fmt.Errorf("%s inválido: %w", section, err)
		}
	}
	return d, nil
}

// canonJSON normaliza um valor (chaves ordenadas, sem espaços) para o hash.
func canonJSON(raw []byte) (string, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func (d *jsonDoc) Hash(name string) (string, bool) {
	raw, ok := d.servers.vals[name]
	if !ok {
		return "", false
	}
	c, err := canonJSON(raw)
	if err != nil {
		return sum(string(raw)), true
	}
	return sum(c), true
}

func (d *jsonDoc) HashOf(_ string, v map[string]any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	c, err := canonJSON(b)
	return sum(c), err
}

func (d *jsonDoc) Set(name string, v map[string]any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	d.servers.set(name, b)
	return nil
}

func (d *jsonDoc) Delete(name string) { d.servers.delete(name); d.deleted = true }

func (d *jsonDoc) Bytes() []byte {
	if d.deleted && len(d.servers.keys) == 0 {
		// Não deixa "atlana": {} (ou "mcpServers": {}) para trás no uninstall.
		d.top.delete(d.section)
	} else {
		d.top.set(d.section, d.servers.marshal())
	}
	var out bytes.Buffer
	if err := json.Indent(&out, d.top.marshal(), "", "  "); err != nil {
		panic(err) // só juntamos JSON já validado
	}
	out.WriteByte('\n')
	return out.Bytes()
}

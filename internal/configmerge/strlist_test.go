package configmerge

import (
	"strings"
	"testing"
)

func TestStringList(t *testing.T) {
	raw := []byte(`{"$schema":"x","instructions":["CONTRIBUTING.md",{"odd":1}],"theme":"t"}`)
	d, err := ParseStringList(raw, "instructions")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Hash("a/*.md"); ok {
		t.Fatal("ainda não está")
	}
	d.Set("a/*.md", nil)
	d.Set("a/*.md", nil) // idempotente
	h, ok := d.Hash("a/*.md")
	want, _ := d.HashOf("a/*.md", nil)
	if !ok || h != want {
		t.Fatal("hash")
	}
	out := string(d.Bytes())
	if strings.Count(out, "a/*.md") != 1 || !strings.Contains(out, "CONTRIBUTING.md") || !strings.Contains(out, `"odd": 1`) || !strings.Contains(out, `"theme": "t"`) {
		t.Fatalf("merge:\n%s", out)
	}
	if strings.Index(out, "$schema") > strings.Index(out, "instructions") {
		t.Fatal("ordem das chaves")
	}

	d, _ = ParseStringList([]byte(out), "instructions")
	d.Delete("a/*.md")
	d.Delete("CONTRIBUTING.md")
	d.Delete(`{"odd":1}`) // não é string: fica
	if out := string(d.Bytes()); strings.Contains(out, "a/*.md") || !strings.Contains(out, "odd") {
		t.Fatalf("delete:\n%s", out)
	}

	d, _ = ParseStringList(nil, "instructions")
	d.Set("x.md", nil)
	d.Delete("x.md")
	if out := string(d.Bytes()); strings.Contains(out, "instructions") {
		t.Fatalf("lista vazia some: %s", out)
	}
	if _, err := ParseStringList([]byte(`{"instructions":"x"}`), "instructions"); err == nil {
		t.Fatal("instructions precisa ser lista")
	}
}

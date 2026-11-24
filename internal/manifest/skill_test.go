package manifest

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidName(t *testing.T) {
	good := []string{"a", "pdf", "say-hi", "v2-tools", strings.Repeat("a", 64)}
	bad := []string{"", "-a", "a-", "a--b", "Say", "a_b", "a b", "é", strings.Repeat("a", 65)}
	for _, n := range good {
		if !ValidName(n) {
			t.Errorf("%q deveria ser válido", n)
		}
	}
	for _, n := range bad {
		if ValidName(n) {
			t.Errorf("%q deveria ser inválido", n)
		}
	}
}

func TestFrontmatter(t *testing.T) {
	fm, err := Frontmatter([]byte("---\r\nname: x\r\n---\r\nbody"))
	if err != nil || strings.TrimSpace(string(fm)) != "name: x" {
		t.Fatalf("CRLF: %q %v", fm, err)
	}
	for _, in := range []string{"name: x", "---\nname: x\n", "---\nname: x\n----\n"} {
		if _, err := Frontmatter([]byte(in)); err == nil {
			t.Errorf("esperava erro para %q", in)
		}
	}
}

func TestValidateSkill(t *testing.T) {
	cases := []struct {
		name, dir, content, want string
		warning                  bool
	}{
		{"ok", "say-hi", "---\nname: say-hi\ndescription: d\n---\n", "", false},
		{"sem name", "say-hi", "---\ndescription: d\n---\n", "`name` ausente", false},
		{"name != dir", "say-hi", "---\nname: other\ndescription: d\n---\n", "igual ao nome do diretório", false},
		{"name inválido", "Bad", "---\nname: Bad\ndescription: d\n---\n", "inválido", false},
		{"sem description", "say-hi", "---\nname: say-hi\n---\n", "`description` ausente", false},
		{"description longa", "say-hi", "---\nname: say-hi\ndescription: " + strings.Repeat("x", 1025) + "\n---\n", "máximo 1024", false},
		{"metadata não-texto", "say-hi", "---\nname: say-hi\ndescription: d\nmetadata:\n  n: [1]\n---\n", "metadata.n", false},
		{"campo extra", "say-hi", "---\nname: say-hi\ndescription: d\nmodel: x\n---\n", "não faz parte da spec", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), c.dir)
			writeFile(t, filepath.Join(dir, "SKILL.md"), c.content)
			issues := ValidateSkill(dir)
			if c.want == "" {
				if len(issues) != 0 {
					t.Fatalf("esperava nenhum problema, veio %v", issues)
				}
				return
			}
			for _, i := range issues {
				if strings.Contains(i.Message, c.want) {
					if i.Warning != c.warning {
						t.Fatalf("warning = %v, esperado %v", i.Warning, c.warning)
					}
					return
				}
			}
			t.Fatalf("esperava %q, veio %v", c.want, issues)
		})
	}
}

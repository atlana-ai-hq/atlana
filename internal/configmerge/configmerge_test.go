package configmerge

import (
	"strings"
	"testing"
)

var srv = map[string]any{"command": "npx", "args": []any{"-y", "demo"}, "env": map[string]any{"TOKEN": "${TOKEN}"}}

func TestJSONKeepsOtherKeysAndOrder(t *testing.T) {
	raw := []byte(`{"zeta": 1, "mcpServers": {"mine": {"command": "x"}}, "alpha": {"b": [1,2]}}`)
	d, err := Parse(JSON, raw, "mcpServers")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Set("demo", srv); err != nil {
		t.Fatal(err)
	}
	got := string(d.Bytes())
	iz, im, ia := strings.Index(got, `"zeta"`), strings.Index(got, `"mcpServers"`), strings.Index(got, `"alpha"`)
	if iz < 0 || !(iz < im && im < ia) || !strings.Contains(got, `"mine"`) || !strings.Contains(got, `"demo"`) {
		t.Fatalf("ordem ou chaves perdidas:\n%s", got)
	}
	want, _ := d.HashOf("demo", srv)
	re, _ := Parse(JSON, d.Bytes(), "mcpServers")
	if h, ok := re.Hash("demo"); !ok || h != want {
		t.Fatal("hash depois de reler deveria bater")
	}
	re.Delete("demo")
	if _, ok := re.Hash("demo"); ok || !strings.Contains(string(re.Bytes()), `"mine"`) {
		t.Fatal("delete deveria tirar só o nosso servidor")
	}
}

func TestJSONHashIgnoresFormatting(t *testing.T) {
	a, _ := Parse(JSON, []byte(`{"mcpServers":{"s":{"b":1,"a":"x"}}}`), "mcpServers")
	b, _ := Parse(JSON, []byte("{\n  \"mcpServers\": {\n    \"s\": { \"a\": \"x\",\n \"b\": 1 }\n  }\n}"), "mcpServers")
	ha, _ := a.Hash("s")
	hb, _ := b.Hash("s")
	if ha != hb {
		t.Fatal("reformatar não deveria mudar o hash")
	}
}

func TestJSONNewAndInvalid(t *testing.T) {
	d, err := Parse(JSON, nil, "mcpServers")
	if err != nil {
		t.Fatal(err)
	}
	d.Set("demo", srv)
	if !strings.HasPrefix(string(d.Bytes()), "{\n  \"mcpServers\": {\n    \"demo\"") {
		t.Fatalf("arquivo novo inesperado:\n%s", d.Bytes())
	}
	for _, bad := range []string{`[1]`, `{"a": }`, `{} {}`, `{"mcpServers": []}`} {
		if _, err := Parse(JSON, []byte(bad), "mcpServers"); err == nil {
			t.Errorf("%q deveria ser inválido", bad)
		}
	}
}

const codexCfg = `model = "o3"

[mcp_servers.user]
command = "user-cmd"

[mcp_servers.user.env]
A = "1"

[profiles.x]
model = "y"
`

func TestTOMLSetReplaceDelete(t *testing.T) {
	d, err := Parse(TOML, []byte(codexCfg), "mcp_servers")
	if err != nil {
		t.Fatal(err)
	}
	v := map[string]any{"command": "npx", "args": []any{"-y", "demo"}, "env_vars": []any{"TOKEN"}}
	if err := d.Set("demo", v); err != nil {
		t.Fatal(err)
	}
	got := string(d.Bytes())
	if !strings.HasPrefix(got, codexCfg) || !strings.HasSuffix(got, "\n\n[mcp_servers.demo]\nargs = [\"-y\", \"demo\"]\ncommand = \"npx\"\nenv_vars = [\"TOKEN\"]\n") {
		t.Fatalf("append inesperado:\n%s", got)
	}
	want, _ := d.HashOf("demo", v)
	if h, _ := d.Hash("demo"); h != want {
		t.Fatal("hash do bloco escrito deveria bater")
	}

	// o bloco do usuário inclui a subtabela .env e para antes de [profiles.x]
	u1, _ := d.Hash("user")
	d.Set("user", map[string]any{"command": "outro"})
	if strings.Contains(string(d.Bytes()), `A = "1"`) || !strings.Contains(string(d.Bytes()), "[profiles.x]") {
		t.Fatalf("replace deveria trocar o bloco e a subtabela:\n%s", d.Bytes())
	}
	if u2, _ := d.Hash("user"); u1 == u2 {
		t.Fatal("hash deveria mudar")
	}

	d.Delete("demo")
	if strings.Contains(string(d.Bytes()), "demo") || !strings.HasSuffix(string(d.Bytes()), "model = \"y\"\n") {
		t.Fatalf("delete inesperado:\n%s", d.Bytes())
	}
}

func TestTOMLRefusesOtherForms(t *testing.T) {
	for _, raw := range []string{"[mcp_servers]\nx = { command = \"a\" }\n", "mcp_servers = {}\n", "mcp_servers.x.command = \"a\"\n"} {
		if _, err := Parse(TOML, []byte(raw), "mcp_servers"); err == nil {
			t.Errorf("deveria recusar:\n%s", raw)
		}
	}
	if _, err := Parse(TOML, []byte("[mcp_servers.ok]\ncommand = \"a\"\n"), "mcp_servers"); err != nil {
		t.Fatal(err)
	}
}

func TestTOMLQuotedNamesAndComments(t *testing.T) {
	d, _ := Parse(TOML, []byte("[mcp_servers.\"a.b\"]\n# comentário\ncommand = \"x\"\n"), "mcp_servers")
	h1, ok := d.Hash("a.b")
	if !ok {
		t.Fatal("nome entre aspas não achado")
	}
	h2, _ := d.HashOf("a.b", map[string]any{"command": "x"})
	if h1 != h2 {
		t.Fatal("comentários não deveriam mudar o hash")
	}
}

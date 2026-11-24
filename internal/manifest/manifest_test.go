package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validManifest = `apiVersion: atlana/v1alpha1
kind: Pack
name: demo
version: 1.2.3
description: Pack de teste.
modules:
  skills: [say-hi]
`

const validSkill = `---
name: say-hi
description: Diz oi.
---
corpo
`

func newPack(t *testing.T, manifest, skill string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, FileName), manifest)
	if skill != "" {
		writeFile(t, filepath.Join(dir, "skills", "say-hi", "SKILL.md"), skill)
	}
	return dir
}

func hasIssue(rep *Report, substr string) bool {
	for _, i := range rep.Issues {
		if strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

func TestLoadValid(t *testing.T) {
	p, rep, err := Load(newPack(t, validManifest, validSkill))
	if err != nil {
		t.Fatalf("esperava pack válido, veio %v: %v", err, rep.Issues)
	}
	if p.Name != "demo" || p.Version != "1.2.3" || p.Visibility != "public" {
		t.Fatalf("campos inesperados: %+v", p)
	}
	if got := p.Modules.Items(ModuleSkills); len(got) != 1 || got[0] != "say-hi" {
		t.Fatalf("skills = %v", got)
	}
}

func TestLoadSchemaErrors(t *testing.T) {
	cases := map[string]string{
		"apiVersion errada":   strings.Replace(validManifest, "atlana/v1alpha1", "atlana/v9", 1),
		"nome inválido":       strings.Replace(validManifest, "name: demo", "name: Demo_Pack", 1),
		"versão não semver":   strings.Replace(validManifest, "1.2.3", "v1", 1),
		"campo desconhecido":  validManifest + "foo: bar\n",
		"segredo em env":      validManifest + "requires:\n  env: [\"API_KEY=abc\"]\n",
		"módulo desconhecido": strings.Replace(validManifest, "skills: [say-hi]", "plugins: [x]", 1),
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			_, rep, err := Load(newPack(t, m, validSkill))
			if err != ErrInvalid {
				t.Fatalf("esperava ErrInvalid, veio %v", err)
			}
			if !hasIssue(rep, "schema") {
				t.Fatalf("esperava erro de schema, veio %v", rep.Issues)
			}
		})
	}
}

func TestExtensionFieldsAllowed(t *testing.T) {
	if _, rep, err := Load(newPack(t, validManifest+"x-notes: livre\n", validSkill)); err != nil {
		t.Fatalf("campos x- deveriam ser aceitos: %v", rep.Issues)
	}
}

func TestLoadMissingSkill(t *testing.T) {
	_, rep, err := Load(newPack(t, validManifest, ""))
	if err != ErrInvalid || !hasIssue(rep, "não existe") {
		t.Fatalf("esperava skill ausente, veio %v %v", err, rep.Issues)
	}
}

func TestUndeclaredSkillIsWarning(t *testing.T) {
	dir := newPack(t, validManifest, validSkill)
	writeFile(t, filepath.Join(dir, "skills", "extra", "SKILL.md"), "---\nname: extra\ndescription: x\n---\n")
	_, rep, err := Load(dir)
	if err != nil {
		t.Fatalf("aviso não deveria falhar: %v", rep.Issues)
	}
	if !hasIssue(rep, "não está em modules.skills") {
		t.Fatalf("esperava aviso, veio %v", rep.Issues)
	}
}

func TestDeclaredModulesMustExist(t *testing.T) {
	m := validManifest + "  agents: [planner]\n  mcp: [registry]\n  rules: [AGENTS.md]\n"
	_, rep, err := Load(newPack(t, m, validSkill))
	if err != ErrInvalid {
		t.Fatalf("esperava ErrInvalid")
	}
	for _, want := range []string{"agents/planner.md", "mcp.json não existe", `"AGENTS.md" não existe`} {
		if !hasIssue(rep, want) {
			t.Errorf("faltou erro %q em %v", want, rep.Issues)
		}
	}
}

func TestRulesPathEscape(t *testing.T) {
	m := validManifest + "  rules: [\"../fora.md\"]\n"
	_, rep, _ := Load(newPack(t, m, validSkill))
	if !hasIssue(rep, "dentro do pack") {
		t.Fatalf("esperava bloqueio de ../, veio %v", rep.Issues)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "packs", "a", FileName), "x")
	writeFile(t, filepath.Join(root, "packs", "b", FileName), "x")
	writeFile(t, filepath.Join(root, "testdata", "c", FileName), "x")
	got, err := Find(root)
	if err != nil || len(got) != 2 {
		t.Fatalf("Find = %v, %v", got, err)
	}
}

func TestParseModule(t *testing.T) {
	if _, err := ParseModule("skills"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseModule("plugins"); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestTargetsAllows(t *testing.T) {
	tg := Targets{Exclude: []string{"codex"}}
	if tg.Allows("codex") || !tg.Allows("cursor") {
		t.Fatal("exclude não respeitado")
	}
	tg = Targets{Include: []string{"claude"}}
	if !tg.Allows("claude") || tg.Allows("cursor") {
		t.Fatal("include não respeitado")
	}
}

// Os packs versionados no repo precisam ser sempre válidos.
func TestRepoPacksAreValid(t *testing.T) {
	dirs, err := Find(filepath.Join("..", "..", "packs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("nenhum pack encontrado em packs/")
	}
	for _, d := range dirs {
		if _, rep, err := Load(d); err != nil {
			t.Errorf("%s: %v %v", d, err, rep.Issues)
		}
	}
}

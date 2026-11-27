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

func TestMCPSecretsAreOnlyReferences(t *testing.T) {
	m := validManifest + "  mcp: [ok, leak, remote, both]\nrequires:\n  env: [TOKEN]\n"
	dir := newPack(t, m, validSkill)
	writeFile(t, filepath.Join(dir, "mcp.json"), `{"mcpServers": {
  "ok": {"command": "npx", "env": {"TOKEN": "${TOKEN}", "OTHER": "${UNDECLARED}"}},
  "leak": {"command": "npx", "env": {"TOKEN": "sk-123"}},
  "remote": {"url": "https://x", "headers": {"Authorization": "Bearer ${TOKEN}", "X-Key": "abc"}},
  "both": {"command": "a", "url": "b"}
}}`)
	_, rep, _ := Load(dir)
	for _, want := range []string{
		"mcpServers.leak.env.TOKEN: use só referência",
		"mcpServers.remote.headers.X-Key: use só referência",
		"mcpServers.both: precisa de exatamente um entre command e url",
		"usa ${UNDECLARED}, que não está em requires.env",
	} {
		if !hasIssue(rep, want) {
			t.Errorf("faltou %q: %+v", want, rep.Issues)
		}
	}
	if hasIssue(rep, "mcpServers.ok.env.TOKEN") || hasIssue(rep, "Authorization") {
		t.Errorf("referências válidas não deveriam gerar erro: %+v", rep.Issues)
	}
}

func TestSubagentValidation(t *testing.T) {
	m := validManifest + "  agents: [planner, bad]\n"
	dir := newPack(t, m, validSkill)
	writeFile(t, filepath.Join(dir, "agents", "planner.md"), "---\nname: planner\ndescription: Planeja.\ntools: [read, search, shell:readonly]\nmodel: fast\n---\n\nVocê planeja.\n")
	writeFile(t, filepath.Join(dir, "agents", "bad.md"), "---\nname: other\ndescription: \"\"\ntools: [teleport]\nmodel: huge\n---\n")
	p, rep, _ := Load(dir)
	for _, want := range []string{`name "other" precisa ser igual`, "description é obrigatório", `tool "teleport" desconhecida`, `model "huge" inválido`, "corpo (prompt do subagent) está vazio"} {
		if !hasIssue(rep, want) {
			t.Errorf("faltou %q: %+v", want, rep.Issues)
		}
	}
	if hasIssue(rep, "planner.md") {
		t.Errorf("planner é válido: %+v", rep.Issues)
	}
	s, err := p.Subagent("planner")
	if err != nil || s.Model != "fast" || s.Body != "Você planeja.\n" || len(s.Tools) != 3 {
		t.Fatalf("subagent: %+v %v", s, err)
	}
}

func TestHookValidation(t *testing.T) {
	m := validManifest + "  hooks: [guard, bad]\n"
	dir := newPack(t, m, validSkill)
	writeFile(t, filepath.Join(dir, "hooks", "guard", "hook.yaml"), "name: guard\nevent: pre_tool\nmatch:\n  tool: [edit, write]\n  path: [\".plan/**\"]\nrun: ./run.sh\nfallback: rule\n")
	writeFile(t, filepath.Join(dir, "hooks", "guard", "run.sh"), "#!/bin/sh\nexit 0\n")
	os.Chmod(filepath.Join(dir, "hooks", "guard", "run.sh"), 0o755)
	writeFile(t, filepath.Join(dir, "hooks", "guard", "rule.md"), "Não edite .plan/.\n")
	writeFile(t, filepath.Join(dir, "hooks", "bad", "hook.yaml"), "name: bad\nevent: stop\nmatch:\n  tool: [fly]\nrun: ../escape.sh\ntimeout: 99\nfallback: rule\n")
	_, rep, _ := Load(dir)
	for _, want := range []string{`match.tool "fly" inválido`, "match só vale para pre_tool", `run "../escape.sh"`, "timeout precisa", "fallback: rule precisa de hooks/bad/rule.md"} {
		if !hasIssue(rep, want) {
			t.Errorf("faltou %q: %+v", want, rep.Issues)
		}
	}
	for _, is := range rep.Issues {
		if strings.Contains(is.File, "guard") {
			t.Errorf("guard é válido: %v", is)
		}
	}

	writeFile(t, filepath.Join(dir, "hooks", "guard", "run.sh"), "#!/bin/sh\n")
	os.Chmod(filepath.Join(dir, "hooks", "guard", "run.sh"), 0o644)
	if _, rep, _ := Load(dir); !hasIssue(rep, "precisa ser executável") {
		t.Error("run sem +x deveria falhar")
	}
}

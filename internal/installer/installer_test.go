package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atlana-ai-hq/atlana/internal/adapter"
	"github.com/atlana-ai-hq/atlana/internal/lockfile"
	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

var now = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	env  adapter.Env
	pack *manifest.Pack
}

func setup(t *testing.T, skills ...string) fixture {
	t.Helper()
	dir := t.TempDir()
	m := "apiVersion: atlana/v1alpha1\nkind: Pack\nname: demo\nversion: 0.1.0\ndescription: d\nmodules:\n  skills: ["
	for i, s := range skills {
		if i > 0 {
			m += ", "
		}
		m += s
		write(t, filepath.Join(dir, "skills", s, "SKILL.md"), "---\nname: "+s+"\ndescription: d\n---\n")
	}
	m += "]\n"
	write(t, filepath.Join(dir, "atlana.yaml"), m)
	write(t, filepath.Join(dir, "skills", skills[0], "references", "guia.md"), "ref")
	p, rep, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("pack de teste inválido: %v %v", err, rep.Issues)
	}
	env := adapter.Env{
		Home: t.TempDir(), WorkDir: t.TempDir(),
		LookPath: func(string) (string, error) { return "", errors.New("x") },
		Getenv:   func(string) string { return "" },
	}
	return fixture{env: env, pack: p}
}

func agents(t *testing.T, ids ...string) []adapter.Adapter {
	var out []adapter.Adapter
	for _, id := range ids {
		a, err := adapter.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func (f fixture) plan(t *testing.T, ids ...string) *Plan {
	t.Helper()
	p, err := PlanInstall(InstallRequest{Pack: f.pack, Agents: agents(t, ids...), Scope: adapter.ScopeUser, Env: f.env})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func ops(p *Plan) map[Op]int {
	m := map[Op]int{}
	for _, a := range p.Actions {
		m[a.Op]++
	}
	return m
}

func TestPlanDoesNotTouchDisk(t *testing.T) {
	f := setup(t, "alpha")
	p := f.plan(t, "cursor", "claude")
	if got := ops(p)[OpCreate]; got != 4 { // 2 arquivos x 2 agentes
		t.Fatalf("criar = %d, esperado 4 (%+v)", got, p.Actions)
	}
	if _, err := os.Stat(filepath.Join(f.env.Home, ".cursor")); !os.IsNotExist(err) {
		t.Fatal("o plano não pode escrever no disco")
	}
	if _, err := os.Stat(p.LockPath); !os.IsNotExist(err) {
		t.Fatal("o plano não pode criar o lock")
	}
}

func TestApplyWritesFilesAndLock(t *testing.T) {
	f := setup(t, "alpha")
	p := f.plan(t, "cursor", "claude")
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(f.env.Home, ".cursor", "skills", "alpha", "SKILL.md"),
		filepath.Join(f.env.Home, ".claude", "skills", "alpha", "references", "guia.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("faltou %s", path)
		}
	}
	lock, err := lockfile.Load(p.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Entries) != 2 {
		t.Fatalf("lock com %d entradas, esperado 2", len(lock.Entries))
	}
	if e, ok := lock.Find("demo", "cursor", "skills"); !ok || len(e.Files) != 2 || e.InstalledAt != "2026-11-24T12:00:00Z" {
		t.Fatalf("entrada cursor inesperada: %+v", e)
	}

	// Reinstalar sem mudanças = idempotente.
	again := f.plan(t, "cursor", "claude")
	if again.Changes() != 0 || ops(again)[OpUnchanged] != 4 {
		t.Fatalf("esperava tudo igual, veio %v", ops(again))
	}
}

func TestUpdateOwnedFile(t *testing.T) {
	f := setup(t, "alpha")
	if err := Apply(f.plan(t, "cursor"), now); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.pack.Dir, "skills", "alpha", "SKILL.md"), "---\nname: alpha\ndescription: nova\n---\n")
	p := f.plan(t, "cursor")
	if ops(p)[OpUpdate] != 1 || len(p.Conflicts) != 0 {
		t.Fatalf("esperava 1 atualização sem conflito: %v %v", ops(p), p.Conflicts)
	}
}

func TestConflictWithUserFile(t *testing.T) {
	f := setup(t, "alpha")
	userFile := filepath.Join(f.env.Home, ".claude", "skills", "alpha", "SKILL.md")
	write(t, userFile, "minha skill")
	p := f.plan(t, "claude")
	if len(p.Conflicts) != 1 {
		t.Fatalf("esperava 1 conflito, veio %v", p.Conflicts)
	}
	if err := Apply(p, now); !errors.Is(err, ErrConflicts) {
		t.Fatalf("Apply deveria recusar: %v", err)
	}
	if b, _ := os.ReadFile(userFile); string(b) != "minha skill" {
		t.Fatal("arquivo do usuário foi alterado")
	}
	if _, err := os.Stat(filepath.Join(f.env.Home, ".claude", "skills", "alpha", "references")); !os.IsNotExist(err) {
		t.Fatal("nada deveria ser escrito quando há conflito")
	}
}

func TestSkillRemovedFromPackIsCleanedUp(t *testing.T) {
	f := setup(t, "alpha", "beta")
	if err := Apply(f.plan(t, "cursor"), now); err != nil {
		t.Fatal(err)
	}
	f.pack.Modules.Skills = []string{"alpha"}
	p := f.plan(t, "cursor")
	if ops(p)[OpRemove] != 1 {
		t.Fatalf("esperava remover beta: %v", ops(p))
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.env.Home, ".cursor", "skills", "beta")); !os.IsNotExist(err) {
		t.Fatal("diretório vazio de beta deveria sumir")
	}
	if _, err := os.Stat(filepath.Join(f.env.Home, ".cursor", "skills")); err != nil {
		t.Fatal("a raiz de skills não pode ser removida")
	}
}

func TestUninstall(t *testing.T) {
	f := setup(t, "alpha")
	if err := Apply(f.plan(t, "cursor", "claude"), now); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(f.env.Home, ".claude", "skills", "alpha", "SKILL.md")
	write(t, edited, "editado pelo usuário")

	p, err := PlanUninstall("demo", nil, adapter.ScopeUser, f.env)
	if err != nil {
		t.Fatal(err)
	}
	if ops(p)[OpRemove] != 3 || len(p.Notes) != 1 {
		t.Fatalf("esperava 3 remoções e 1 nota: %v %v", ops(p), p.Notes)
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(edited); err != nil {
		t.Fatal("arquivo editado deveria ser mantido")
	}
	if _, err := os.Stat(filepath.Join(f.env.Home, ".cursor", "skills", "alpha")); !os.IsNotExist(err) {
		t.Fatal("skill do cursor deveria ter sido removida")
	}
	lock, _ := lockfile.Load(p.LockPath)
	if len(lock.Entries) != 0 {
		t.Fatalf("lock deveria ficar vazio: %+v", lock.Entries)
	}
	if _, err := PlanUninstall("demo", nil, adapter.ScopeUser, f.env); err == nil {
		t.Fatal("desinstalar de novo deveria dizer que não está instalado")
	}
}

func TestUninstallOnlyOneAgent(t *testing.T) {
	f := setup(t, "alpha")
	if err := Apply(f.plan(t, "cursor", "claude"), now); err != nil {
		t.Fatal(err)
	}
	p, err := PlanUninstall("demo", []string{"claude"}, adapter.ScopeUser, f.env)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	lock, _ := lockfile.Load(p.LockPath)
	if _, ok := lock.Find("demo", "cursor", "skills"); !ok || len(lock.Entries) != 1 {
		t.Fatalf("só a entrada do claude deveria sair: %+v", lock.Entries)
	}
}

// detectOnly é um agente que ainda não instala nada.
type detectOnly struct{}

func (detectOnly) ID() string                           { return "fake" }
func (detectOnly) Name() string                         { return "Fake" }
func (detectOnly) Detect(adapter.Env) adapter.Detection { return adapter.Detection{} }
func (detectOnly) Supports(manifest.Module) bool        { return false }
func (detectOnly) SkillsRoot(adapter.Scope, adapter.Env) (string, error) {
	return "", adapter.ErrUnsupported
}

func TestUnsupportedAndExcludedAgents(t *testing.T) {
	f := setup(t, "alpha")
	f.pack.Targets.Exclude = []string{"claude"}
	ads := append([]adapter.Adapter{detectOnly{}}, agents(t, "claude")...)
	p, err := PlanInstall(InstallRequest{Pack: f.pack, Agents: ads, Scope: adapter.ScopeUser, Env: f.env})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Actions) != 0 || len(p.Notes) != 2 {
		t.Fatalf("esperava 0 ações e 2 notas: %+v %v", p.Actions, p.Notes)
	}
}

func TestStatus(t *testing.T) {
	f := setup(t, "alpha")
	if err := Apply(f.plan(t, "cursor"), now); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(f.env.Home, ".cursor", "skills", "alpha", "references", "guia.md"))
	st, err := Status(f.env)
	if err != nil || len(st) != 1 {
		t.Fatalf("status: %v %v", st, err)
	}
	counts := map[FileState]int{}
	for _, s := range st[0].States {
		counts[s]++
	}
	if counts[StateOK] != 1 || counts[StateMissing] != 1 {
		t.Fatalf("estados inesperados: %v", counts)
	}
}

func TestProjectScopeLock(t *testing.T) {
	f := setup(t, "alpha")
	p, err := PlanInstall(InstallRequest{Pack: f.pack, Agents: agents(t, "cursor"), Scope: adapter.ScopeProject, Env: f.env})
	if err != nil {
		t.Fatal(err)
	}
	if p.LockPath != filepath.Join(f.env.WorkDir, "atlana.lock") {
		t.Fatalf("lock do projeto em %s", p.LockPath)
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.env.WorkDir, ".cursor", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatal("skill deveria estar no projeto")
	}
}

func TestGrokExport(t *testing.T) {
	f := setup(t, "alpha")
	root := filepath.Join(f.env.Home, ".config", "atlana", "exports", "grok", "skills")
	p := f.plan(t, "grok")
	if ops(p)[OpCreate] != 2 || len(p.Targets) != 1 || p.Targets[0].Root != root {
		t.Fatalf("plano inesperado: %v %+v", ops(p), p.Targets)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("o plano não pode escrever no disco")
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(root, "alpha", "SKILL.md")
	if _, err := os.Stat(filepath.Join(root, "alpha", "references", "guia.md")); err != nil {
		t.Fatal("pasta da skill incompleta")
	}

	write(t, skill, "editado")
	st, err := Status(f.env)
	if err != nil || len(st) != 1 || st[0].Entry.Agent != "grok" || st[0].States[skill] != StateModified {
		t.Fatalf("status inesperado: %+v %v", st, err)
	}

	u, err := PlanUninstall("demo", []string{"grok"}, adapter.ScopeUser, f.env)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(u, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatal("arquivo editado deveria ser mantido")
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", "references")); !os.IsNotExist(err) {
		t.Fatal("arquivo intacto deveria ter sido removido")
	}
}

func TestGeminiProjectScope(t *testing.T) {
	f := setup(t, "alpha")
	p, err := PlanInstall(InstallRequest{Pack: f.pack, Agents: agents(t, "gemini"), Scope: adapter.ScopeProject, Env: f.env})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.env.WorkDir, ".gemini", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatal("skill não instalada no gemini")
	}
	if _, err := os.Stat(filepath.Join(f.env.Home, ".gemini")); !os.IsNotExist(err) {
		t.Fatal("escopo project não pode tocar no HOME")
	}
}

func setupRules(t *testing.T) fixture {
	t.Helper()
	f := setup(t, "alpha")
	write(t, filepath.Join(f.pack.Dir, "AGENTS.md"), "# Regras do demo\n")
	f.pack.Modules.Rules = []string{"AGENTS.md"}
	return f
}

func (f fixture) planScope(t *testing.T, scope adapter.Scope, mods []manifest.Module, ids ...string) *Plan {
	t.Helper()
	p, err := PlanInstall(InstallRequest{Pack: f.pack, Agents: agents(t, ids...), Modules: mods, Scope: scope, Env: f.env})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRulesInstallAndUninstall(t *testing.T) {
	f := setupRules(t)
	userClaude := filepath.Join(f.env.WorkDir, "CLAUDE.md")
	write(t, userClaude, "meu CLAUDE.md")
	rules := []manifest.Module{manifest.ModuleRules}

	p := f.planScope(t, adapter.ScopeProject, rules, "claude", "cursor", "gemini", "codex")
	if ops(p)[OpCreate] != 4 || len(p.Conflicts) != 0 {
		t.Fatalf("plano inesperado: %v %v", ops(p), p.Conflicts)
	}
	if len(p.Notes) != 2 { // gemini (@import) e codex (referência manual)
		t.Fatalf("notas inesperadas: %v", p.Notes)
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	claudeRule := filepath.Join(f.env.WorkDir, ".claude", "rules", "atlana-demo", "AGENTS.md")
	if b, _ := os.ReadFile(claudeRule); string(b) != "# Regras do demo\n" {
		t.Fatalf("rule do claude: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(f.env.WorkDir, ".cursor", "rules", "atlana-demo-AGENTS.mdc")); !strings.Contains(string(b), "alwaysApply: true") {
		t.Fatalf("mdc: %q", b)
	}
	if b, _ := os.ReadFile(userClaude); string(b) != "meu CLAUDE.md" {
		t.Fatal("CLAUDE.md da pessoa não pode mudar")
	}
	if again := f.planScope(t, adapter.ScopeProject, rules, "claude", "cursor", "gemini", "codex"); again.Changes() != 0 {
		t.Fatalf("reinstalar deveria ser idempotente: %v", ops(again))
	}

	u, err := PlanUninstall("demo", nil, adapter.ScopeProject, f.env)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(u, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(claudeRule)); !os.IsNotExist(err) {
		t.Fatal("pasta atlana-demo deveria ter sido removida")
	}
	if _, err := os.Stat(filepath.Join(f.env.WorkDir, ".claude", "rules")); err != nil {
		t.Fatal("a raiz .claude/rules não deve ser apagada")
	}
}

func TestRulesConflictAndCursorUserScope(t *testing.T) {
	f := setupRules(t)
	write(t, filepath.Join(f.env.Home, ".claude", "rules", "atlana-demo", "AGENTS.md"), "de outra pessoa")
	p := f.planScope(t, adapter.ScopeUser, []manifest.Module{manifest.ModuleRules}, "claude", "cursor")
	if len(p.Conflicts) != 1 || !errors.Is(Apply(p, now), ErrConflicts) {
		t.Fatalf("esperava 1 conflito: %v", p.Conflicts)
	}
	if len(p.Notes) != 1 || !strings.Contains(p.Notes[0], "cursor") {
		t.Fatalf("cursor no escopo user deveria virar nota: %v", p.Notes)
	}
}

func TestGrokSkipsRules(t *testing.T) {
	f := setupRules(t)
	p := f.plan(t, "grok")
	if ops(p)[OpCreate] != 2 || len(p.Notes) != 1 { // só as skills
		t.Fatalf("grok: %v %v", ops(p), p.Notes)
	}
}

func setupMCP(t *testing.T) fixture {
	t.Helper()
	f := setup(t, "alpha")
	write(t, filepath.Join(f.pack.Dir, "mcp.json"), `{"mcpServers": {"demo": {"command": "npx", "args": ["-y", "demo"], "env": {"TOKEN": "${TOKEN}"}}}}`)
	f.pack.Modules.MCP = []string{"demo"}
	return f
}

var onlyMCP = []manifest.Module{manifest.ModuleMCP}

func TestMCPMergeKeepsUserConfig(t *testing.T) {
	f := setupMCP(t)
	claudeJSON := filepath.Join(f.env.Home, ".claude.json")
	write(t, claudeJSON, `{"numStartups": 7, "mcpServers": {"mine": {"command": "mine"}}, "projects": {}}`)
	if err := os.Chmod(claudeJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	codexTOML := filepath.Join(f.env.Home, ".codex", "config.toml")
	write(t, codexTOML, "model = \"o3\"\n")

	p := f.planScope(t, adapter.ScopeUser, onlyMCP, "claude", "codex", "cursor", "gemini")
	if ops(p)[OpCreate] != 4 || len(p.Conflicts) != 0 {
		t.Fatalf("plano: %v %v %v", ops(p), p.Conflicts, p.Notes)
	}
	if b, _ := os.ReadFile(claudeJSON); strings.Contains(string(b), "demo") {
		t.Fatal("o plano não pode escrever")
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(claudeJSON)
	for _, want := range []string{`"numStartups": 7`, `"mine"`, `"demo"`, `"projects"`, `"${TOKEN}"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf(".claude.json sem %s:\n%s", want, b)
		}
	}
	if st, _ := os.Stat(claudeJSON); st.Mode().Perm() != 0o600 {
		t.Errorf("permissão mudou para %v", st.Mode().Perm())
	}
	if b, _ := os.ReadFile(codexTOML); !strings.HasPrefix(string(b), "model = \"o3\"\n\n[mcp_servers.demo]\n") || !strings.Contains(string(b), `env_vars = ["TOKEN"]`) {
		t.Fatalf("config.toml:\n%s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(f.env.Home, ".cursor", "mcp.json")); !strings.Contains(string(b), "${env:TOKEN}") {
		t.Fatalf("cursor mcp.json:\n%s", b)
	}

	lock, _ := lockfile.Load(p.LockPath)
	if e, ok := lock.Find("demo", "claude", "mcp"); !ok || len(e.Keys) != 1 || len(e.Files) != 0 {
		t.Fatalf("lock deveria guardar a chave, não o arquivo: %+v", e)
	}
	if again := f.planScope(t, adapter.ScopeUser, onlyMCP, "claude", "codex", "cursor", "gemini"); again.Changes() != 0 {
		t.Fatalf("reinstalar deveria ser idempotente: %v", ops(again))
	}

	st, err := Status(f.env)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range st {
		for id, s := range e.States {
			if s != StateOK {
				t.Errorf("%s: %s", id, s)
			}
		}
	}

	u, err := PlanUninstall("demo", nil, adapter.ScopeUser, f.env)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(u, now); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(claudeJSON)
	if strings.Contains(string(b), `"demo"`) || !strings.Contains(string(b), `"mine"`) || !strings.Contains(string(b), `"numStartups": 7`) {
		t.Fatalf("uninstall deveria tirar só o nosso servidor:\n%s", b)
	}
	if b, _ := os.ReadFile(codexTOML); string(b) != "model = \"o3\"\n" {
		t.Fatalf("config.toml depois do uninstall: %q", b)
	}
}

func TestMCPConflictWithUserServer(t *testing.T) {
	f := setupMCP(t)
	write(t, filepath.Join(f.env.WorkDir, ".mcp.json"), `{"mcpServers": {"demo": {"command": "outro"}}}`)
	p := f.planScope(t, adapter.ScopeProject, onlyMCP, "claude")
	if len(p.Conflicts) != 1 || !errors.Is(Apply(p, now), ErrConflicts) {
		t.Fatalf("esperava conflito: %v", p.Conflicts)
	}
}

func TestMCPInvalidConfigIsConflict(t *testing.T) {
	f := setupMCP(t)
	write(t, filepath.Join(f.env.Home, ".gemini", "settings.json"), `{ // comentário
}`)
	p := f.planScope(t, adapter.ScopeUser, onlyMCP, "gemini")
	if len(p.Conflicts) != 1 || !strings.Contains(p.Conflicts[0], "não dá para editar") {
		t.Fatalf("JSON inválido deveria virar conflito: %v", p.Conflicts)
	}
}

func TestMCPUpdateEditedAndStale(t *testing.T) {
	f := setupMCP(t)
	cfg := filepath.Join(f.env.Home, ".cursor", "mcp.json")
	if err := Apply(f.planScope(t, adapter.ScopeUser, onlyMCP, "cursor"), now); err != nil {
		t.Fatal(err)
	}

	// Nova versão do pack: o servidor é nosso e intacto, então atualiza.
	write(t, filepath.Join(f.pack.Dir, "mcp.json"), `{"mcpServers": {"demo": {"command": "npx", "args": ["-y", "demo@2"]}}}`)
	p := f.planScope(t, adapter.ScopeUser, onlyMCP, "cursor")
	if ops(p)[OpUpdate] != 1 {
		t.Fatalf("esperava atualizar: %v %v", ops(p), p.Conflicts)
	}

	// Alguém (o próprio agente) grava a config entre o plano e o apply.
	b, _ := os.ReadFile(cfg)
	write(t, cfg, strings.Replace(string(b), "{", `{"theme": "dark",`, 1))
	if err := Apply(p, now); !errors.Is(err, ErrStale) {
		t.Fatalf("esperava ErrStale, veio %v", err)
	}
	if b, _ := os.ReadFile(cfg); !strings.Contains(string(b), `"theme": "dark"`) {
		t.Fatal("a edição concorrente não pode ser perdida")
	}

	// A pessoa edita o nosso servidor: uninstall mantém e avisa.
	write(t, cfg, `{"mcpServers": {"demo": {"command": "editado"}}}`)
	u, _ := PlanUninstall("demo", nil, adapter.ScopeUser, f.env)
	if ops(u)[OpRemove] != 0 || len(u.Notes) != 1 || !strings.Contains(u.Notes[0], "editado") {
		t.Fatalf("uninstall: %v %v", ops(u), u.Notes)
	}
}

func TestMCPServerRemovedFromPack(t *testing.T) {
	f := setupMCP(t)
	write(t, filepath.Join(f.pack.Dir, "mcp.json"), `{"mcpServers": {"demo": {"command": "a"}, "extra": {"command": "b"}}}`)
	f.pack.Modules.MCP = []string{"demo", "extra"}
	if err := Apply(f.planScope(t, adapter.ScopeUser, onlyMCP, "gemini"), now); err != nil {
		t.Fatal(err)
	}
	f.pack.Modules.MCP = []string{"demo"}
	p := f.planScope(t, adapter.ScopeUser, onlyMCP, "gemini")
	if ops(p)[OpRemove] != 1 || ops(p)[OpUnchanged] != 1 {
		t.Fatalf("esperava remover extra: %v", ops(p))
	}
	if err := Apply(p, now); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(f.env.Home, ".gemini", "settings.json")); strings.Contains(string(b), "extra") {
		t.Fatalf("extra deveria ter saído:\n%s", b)
	}
}

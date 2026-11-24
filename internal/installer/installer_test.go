package installer

import (
	"errors"
	"os"
	"path/filepath"
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

func TestUnsupportedAndExcludedAgents(t *testing.T) {
	f := setup(t, "alpha")
	f.pack.Targets.Exclude = []string{"claude"}
	p := f.plan(t, "codex", "claude")
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

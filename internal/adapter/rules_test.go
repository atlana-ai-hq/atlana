package adapter

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesOutputs(t *testing.T) {
	env := fakeEnv(t)
	files := []RuleFile{{Rel: "AGENTS.md", Content: []byte("---\nx: 1\n---\n\n# Regras\n")}, {Rel: "rules/go.md", Content: []byte("go")}}
	cases := []struct {
		id    string
		scope Scope
		want  []string
		notes int
	}{
		{"claude", ScopeUser, []string{
			filepath.Join(env.Home, ".claude", "rules", "atlana-demo", "AGENTS.md"),
			filepath.Join(env.Home, ".claude", "rules", "atlana-demo", "rules", "go.md")}, 0},
		{"cursor", ScopeProject, []string{
			filepath.Join(env.WorkDir, ".cursor", "rules", "atlana-demo-AGENTS.mdc"),
			filepath.Join(env.WorkDir, ".cursor", "rules", "atlana-demo-rules-go.mdc")}, 0},
		{"gemini", ScopeProject, []string{
			filepath.Join(env.WorkDir, ".gemini", "atlana", "atlana-demo", "AGENTS.md"),
			filepath.Join(env.WorkDir, ".gemini", "atlana", "atlana-demo", "rules", "go.md")}, 2},
		{"codex", ScopeUser, []string{
			filepath.Join(env.Home, ".codex", "atlana", "atlana-demo", "AGENTS.md"),
			filepath.Join(env.Home, ".codex", "atlana", "atlana-demo", "rules", "go.md")}, 1},
	}
	for _, c := range cases {
		ad, _ := Get(c.id)
		out, notes, err := ad.(RulesAdapter).Rules("demo", files, c.scope, env)
		if err != nil || len(out) != len(c.want) || len(notes) != c.notes {
			t.Fatalf("%s: %+v %v %v", c.id, out, notes, err)
		}
		for i, w := range c.want {
			if out[i].Path != w {
				t.Errorf("%s: %s, esperado %s", c.id, out[i].Path, w)
			}
		}
	}
}

func TestCursorRulesMDC(t *testing.T) {
	env := fakeEnv(t)
	out, _, _ := cursor{}.Rules("demo", []RuleFile{{Rel: "AGENTS.md", Content: []byte("---\nx: 1\n---\n\n# Regras\n")}}, ScopeProject, env)
	got := string(out[0].Content)
	if !strings.HasPrefix(got, "---\ndescription: AGENTS.md (demo") || !strings.Contains(got, "alwaysApply: true") || strings.Contains(got, "x: 1") || !strings.HasSuffix(got, "# Regras\n") {
		t.Fatalf("mdc inesperado:\n%s", got)
	}
	if _, _, err := (cursor{}).Rules("demo", nil, ScopeUser, env); !errors.Is(err, ErrUnsupported) {
		t.Fatal("cursor no escopo user deveria ser ErrUnsupported")
	}
}

func TestGeminiUserNoteAndCodexHome(t *testing.T) {
	env := fakeEnv(t)
	_, notes, _ := gemini{}.Rules("demo", []RuleFile{{Rel: "AGENTS.md"}}, ScopeUser, env)
	if !strings.Contains(notes[0], "`@atlana/atlana-demo/AGENTS.md`") || !strings.Contains(notes[0], "~/.gemini/GEMINI.md") {
		t.Fatalf("nota inesperada: %s", notes[0])
	}
	custom := t.TempDir()
	env.Getenv = func(k string) string {
		if k == "CODEX_HOME" {
			return custom
		}
		return ""
	}
	if root, _ := (codex{}).RulesRoot(ScopeUser, env); root != filepath.Join(custom, "atlana") {
		t.Fatalf("CODEX_HOME ignorado: %s", root)
	}
}

func TestStripFrontmatter(t *testing.T) {
	for in, want := range map[string]string{"sem fm": "sem fm", "---\na: 1\n---\n\ncorpo": "corpo", "---\nquebrado": "---\nquebrado"} {
		if got := stripFrontmatter([]byte(in)); got != want {
			t.Errorf("%q → %q, esperado %q", in, got, want)
		}
	}
}

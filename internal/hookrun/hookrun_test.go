package hookrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

func TestGlob(t *testing.T) {
	cases := []struct {
		pat, path string
		want      bool
	}{
		{".plan/**", ".plan/a/b.md", true},
		{".plan/**", ".plan", false},
		{"**/*.go", "main.go", true},
		{"**/*.go", "a/b/c.go", true},
		{"*.md", "a/b.md", false},
		{"docs/?.md", "docs/a.md", true},
		{"a.b", "axb", false},
	}
	for _, c := range cases {
		if got := Glob(c.pat, c.path); got != c.want {
			t.Errorf("Glob(%q, %q) = %v", c.pat, c.path, got)
		}
	}
}

func cat(n string) string {
	return map[string]string{"Edit": "edit", "Bash": "shell"}[n]
}

func TestNormalize(t *testing.T) {
	ev := Normalize("claude", "pre_tool", map[string]any{
		"cwd": "/proj", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": "/proj/.plan/x.md"},
	}, cat)
	if ev.Tool != "edit" || ev.Path != ".plan/x.md" || ev.Cwd != "/proj" {
		t.Fatalf("%+v", ev)
	}
	if ev := Normalize("claude", "pre_tool", map[string]any{"cwd": "/proj", "tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}}, cat); ev.Command != "ls" || ev.Tool != "shell" {
		t.Fatalf("%+v", ev)
	}
	if ev := Normalize("claude", "pre_tool", map[string]any{"cwd": "/proj", "tool_input": map[string]any{"file_path": "/etc/x"}}, cat); ev.Path != "/etc/x" {
		t.Fatalf("fora do cwd fica absoluto: %+v", ev)
	}
}

func installHook(t *testing.T, dir, pack, name, yaml, script string) {
	t.Helper()
	d := filepath.Join(dir, pack, name)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "hook.yaml"), []byte(yaml), 0o644)
	os.WriteFile(filepath.Join(d, "run.sh"), []byte(script), 0o755)
}

func TestDispatch(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "stdin.json")
	installHook(t, dir, "p", "guard", "name: guard\nevent: pre_tool\nmatch:\n  tool: [edit]\n  path: [\".plan/**\"]\nrun: ./run.sh\n",
		"#!/bin/sh\ncat > "+out+"\necho 'não edite .plan' >&2\nexit 2\n")
	installHook(t, dir, "p", "noisy", "name: noisy\nevent: pre_tool\nrun: ./run.sh\n", "#!/bin/sh\necho falhou >&2\nexit 1\n")

	var errb bytes.Buffer
	cwd := t.TempDir()
	if code := Dispatch(dir, Event{Event: "pre_tool", Agent: "claude", Tool: "edit", Path: ".plan/a.md", Cwd: cwd}, &errb); code != 2 {
		t.Fatalf("esperava 2, veio %d: %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "não edite .plan") {
		t.Fatalf("stderr: %s", errb.String())
	}
	if b, _ := os.ReadFile(out); !strings.Contains(string(b), `"path":".plan/a.md"`) || !strings.Contains(string(b), `"agent":"claude"`) {
		t.Fatalf("payload normalizado: %s", b)
	}

	errb.Reset()
	if code := Dispatch(dir, Event{Event: "pre_tool", Agent: "claude", Tool: "edit", Path: "src/a.go"}, &errb); code != 0 {
		t.Fatalf("fora do path não nega: %d", code)
	}
	if !strings.Contains(errb.String(), "noisy") {
		t.Fatalf("erro que não é 2 vira aviso: %q", errb.String())
	}
}

func TestDispatchTimeout(t *testing.T) {
	dir := t.TempDir()
	installHook(t, dir, "p", "slow", "name: slow\nevent: stop\nrun: ./run.sh\ntimeout: 1\n", "#!/bin/sh\nsleep 5\nexit 2\n")
	var errb bytes.Buffer
	if code := Dispatch(dir, Event{Event: "stop"}, &errb); code != 0 || !strings.Contains(errb.String(), "passou de 1s") {
		t.Fatalf("timeout: %d %s", code, errb.String())
	}
}

func TestMatchesEvent(t *testing.T) {
	h := manifest.Hook{Event: "stop"}
	if Matches(h, Event{Event: "pre_tool"}) || !Matches(h, Event{Event: "stop"}) {
		t.Fatal("evento")
	}
}

func TestNormalizeAntigravity(t *testing.T) {
	cat := func(n string) string { return map[string]string{"edit_file": "edit", "run_command": "shell"}[n] }
	ev := Normalize("antigravity", "pre_tool", map[string]any{
		"cwd": "/proj", "lastUserInput": "corrija o teste",
		"toolCall": map[string]any{"name": "edit_file", "args": map[string]any{"TargetFile": "/proj/.plan/x.md"}},
	}, cat)
	if ev.Tool != "edit" || ev.Path != ".plan/x.md" || ev.Prompt != "corrija o teste" {
		t.Fatalf("%+v", ev)
	}
	ev = Normalize("antigravity", "pre_tool", map[string]any{
		"toolCall": map[string]any{"name": "run_command", "args": map[string]any{"CommandLine": "rm -rf /"}},
	}, cat)
	if ev.Tool != "shell" || ev.Command != "rm -rf /" {
		t.Fatalf("%+v", ev)
	}
}

package adapter

import (
	"strings"
	"testing"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

func sub(tools []string, model string) manifest.Subagent {
	return manifest.Subagent{Name: "planner", Description: "Planeja: em etapas.", Tools: tools, Model: model, Body: "Você planeja.\n"}
}

func TestClaudeSubagent(t *testing.T) {
	file, b, notes, err := claude{}.Subagent(sub([]string{"read", "search", "shell:readonly"}, "fast"))
	if err != nil || file != "planner.md" {
		t.Fatal(file, err)
	}
	got := string(b)
	for _, want := range []string{"name: planner\n", "description: 'Planeja: em etapas.'\n", "tools: Read, Grep, Glob\n", "model: haiku\n", "---\n\nVocê planeja.\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("faltou %q:\n%s", want, got)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "shell:readonly") {
		t.Errorf("notas: %v", notes)
	}
	if _, b, _, _ := (claude{}).Subagent(sub(nil, "inherit")); strings.Contains(string(b), "tools:") {
		t.Error("sem tools = herda todas, sem a chave")
	}
	if _, _, _, err := (claude{}).Subagent(sub([]string{"mcp"}, "inherit")); err == nil {
		t.Error("lista que some por inteiro não pode virar 'todas as ferramentas'")
	}
}

func TestGeminiSubagent(t *testing.T) {
	_, b, notes, err := gemini{}.Subagent(sub([]string{"read", "mcp"}, "smart"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"kind: local\n", "- read_file\n", "- mcp_*\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("faltou %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "model:") || len(notes) != 1 {
		t.Errorf("model smart vira aviso, não um nome inventado: %v\n%s", notes, got)
	}
}

func TestHookEventsAndTools(t *testing.T) {
	for _, c := range []struct {
		ad          HooksAdapter
		event, want string
	}{
		{claude{}, "pre_tool", "PreToolUse"}, {claude{}, "prompt_submit", "UserPromptSubmit"},
		{gemini{}, "pre_tool", "BeforeTool"}, {gemini{}, "stop", "AfterAgent"},
	} {
		if got, ok := c.ad.HookEvent(c.event); !ok || got != c.want {
			t.Errorf("%T %s = %s", c.ad, c.event, got)
		}
	}
	if (claude{}).ToolCategory("MultiEdit") != "edit" || (claude{}).ToolCategory("mcp__x__y") != "mcp" || (gemini{}).ToolCategory("run_shell_command") != "shell" {
		t.Error("categorias")
	}
	if _, ok := Adapter(cursor{}).(HooksAdapter); ok {
		t.Error("cursor ainda não liga hooks")
	}
	tool := (claude{}).HookEntry("PreToolUse", "cmd")
	stop := (claude{}).HookEntry("Stop", "cmd")
	if tool["matcher"] != "*" || stop["matcher"] != nil {
		t.Errorf("matcher: %v %v", tool, stop)
	}
}

func TestDispatchCommandQuoting(t *testing.T) {
	got := DispatchCommand("/opt/my bin/atlana", "claude", "pre_tool", "/h/it's/hooks/claude")
	want := `'/opt/my bin/atlana' hook dispatch --agent claude --event pre_tool --dir '/h/it'\''s/hooks/claude'`
	if got != want {
		t.Fatalf("\n%s\n%s", got, want)
	}
}

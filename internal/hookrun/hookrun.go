// Package hookrun é o dispatcher que os agentes chamam quando um hook dispara.
//
// Ele lê o payload nativo do agente, normaliza para um JSON único
// ({"event","agent","tool","path","command","prompt","cwd"}), escolhe os hooks
// instalados que casam e roda cada script. Exit 2 de um script nega a ação e é
// repassado ao agente (Claude Code e Gemini CLI tratam exit 2 como bloqueio).
package hookrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// Event é o payload normalizado que os scripts recebem no stdin.
type Event struct {
	Event   string `json:"event"`
	Agent   string `json:"agent"`
	Tool    string `json:"tool,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
}

// Normalize converte o payload nativo. toolCategory vem do adaptador do agente.
func Normalize(agent, event string, payload map[string]any, toolCategory func(string) string) Event {
	ev := Event{Event: event, Agent: agent}
	str := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	ev.Cwd = str(payload, "cwd")
	ev.Prompt = str(payload, "prompt")
	if name := str(payload, "tool_name"); name != "" {
		ev.Tool = toolCategory(name)
	}
	if in, ok := payload["tool_input"].(map[string]any); ok {
		ev.Path = str(in, "file_path", "path", "absolute_path", "notebook_path")
		ev.Command = str(in, "command")
	}
	if ev.Path != "" && ev.Cwd != "" && filepath.IsAbs(ev.Path) {
		if rel, err := filepath.Rel(ev.Cwd, ev.Path); err == nil && !strings.HasPrefix(rel, "..") {
			ev.Path = filepath.ToSlash(rel)
		}
	}
	return ev
}

// Matches diz se o hook vale para o evento.
func Matches(h manifest.Hook, ev Event) bool {
	if h.Event != ev.Event {
		return false
	}
	if len(h.Match.Tool) > 0 && !contains(h.Match.Tool, ev.Tool) {
		return false
	}
	if len(h.Match.Path) > 0 {
		if ev.Path == "" {
			return false
		}
		ok := false
		for _, g := range h.Match.Path {
			if Glob(g, ev.Path) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Glob casa caminhos com "/" usando *, ? e ** (qualquer profundidade).
func Glob(pattern, path string) bool {
	var re strings.Builder
	re.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '*' && i+1 < len(pattern) && pattern[i+1] == '*':
			i++
			if i+1 < len(pattern) && pattern[i+1] == '/' {
				i++
				re.WriteString("(.*/)?")
			} else {
				re.WriteString(".*")
			}
		case c == '*':
			re.WriteString("[^/]*")
		case c == '?':
			re.WriteString("[^/]")
		default:
			re.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	re.WriteString("$")
	ok, _ := regexp.MatchString(re.String(), path)
	return ok
}

// Hooks lê os hooks instalados em dir (<dir>/<pack>/<hook>/hook.yaml), em ordem estável.
func Hooks(dir string) ([]manifest.Hook, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*", "*", "hook.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []manifest.Hook
	for _, f := range files {
		h, err := manifest.ParseHook(filepath.Dir(f))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, h)
	}
	return out, nil
}

// Dispatch roda os hooks que casam e devolve o exit code para o agente:
// 2 se algum negou, 0 nos outros casos (falhas que não são negação só viram aviso).
func Dispatch(dir string, ev Event, stderr io.Writer) int {
	hooks, err := Hooks(dir)
	if err != nil {
		fmt.Fprintln(stderr, "atlana hook:", err)
		return 0
	}
	input, _ := json.Marshal(ev)
	for _, h := range hooks {
		if !Matches(h, ev) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.Timeout)*time.Second)
		cmd := exec.CommandContext(ctx, filepath.Join(h.Dir, filepath.FromSlash(h.Run)))
		cmd.WaitDelay = time.Second // filhos do script que seguram o stderr não travam o agente
		cmd.Stdin = bytes.NewReader(input)
		var errb bytes.Buffer
		cmd.Stderr = &errb
		cmd.Stdout = io.Discard // o Gemini interpreta o stdout como JSON; não repassamos
		if st, err := os.Stat(ev.Cwd); ev.Cwd != "" && err == nil && st.IsDir() {
			cmd.Dir = ev.Cwd
		}
		err := cmd.Run()
		timedOut := ctx.Err() != nil
		cancel()
		var exit *exec.ExitError
		switch {
		case err == nil:
		case timedOut:
			fmt.Fprintf(stderr, "atlana hook %s: passou de %ds; ignorado\n", h.Name, h.Timeout)
		case errors.As(err, &exit) && exit.ExitCode() == 2:
			msg := strings.TrimSpace(errb.String())
			if msg == "" {
				msg = "bloqueado pelo hook " + h.Name
			}
			fmt.Fprintln(stderr, msg)
			return 2
		default:
			fmt.Fprintf(stderr, "atlana hook %s: %v %s\n", h.Name, err, strings.TrimSpace(errb.String()))
		}
	}
	return 0
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

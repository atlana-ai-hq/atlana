package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/adapter"
	"github.com/atlana-ai-hq/atlana/internal/lockfile"
	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// HooksDir é onde ficam os hooks instalados para um agente: o dispatcher só lê
// esse diretório. user: $XDG_CONFIG_HOME/atlana/hooks/<agente>; project:
// <projeto>/.atlana/hooks/<agente>.
func HooksDir(scope adapter.Scope, env adapter.Env, agent string) (string, error) {
	if scope == adapter.ScopeProject {
		if env.WorkDir == "" {
			return "", errors.New("diretório do projeto vazio")
		}
		return filepath.Join(env.WorkDir, ".atlana", "hooks", agent), nil
	}
	return filepath.Join(filepath.Dir(LockPath(scope, env)), "hooks", agent), nil
}

// dispatcherMatch reconhece, num array hooks.<evento>, o item que chama o nosso dispatcher.
func dispatcherMatch(agent, dir string) func(string, map[string]any) bool {
	marker := fmt.Sprintf(" hook dispatch --agent %s ", agent)
	suffix := "--dir " + adapter.ShellQuote(filepath.Clean(dir))
	return func(_ string, item map[string]any) bool {
		hooks, _ := item["hooks"].([]any)
		for _, h := range hooks {
			m, _ := h.(map[string]any)
			cmd, _ := m["command"].(string)
			if strings.Contains(cmd, marker) && strings.HasSuffix(cmd, suffix) {
				return true
			}
		}
		return false
	}
}

// planAgents traduz cada subagent canônico para o arquivo do agente.
func planAgents(plan *Plan, lock *lockfile.Lock, p *manifest.Pack, ad adapter.Adapter, scope adapter.Scope, env adapter.Env) error {
	aa, ok := ad.(adapter.AgentsAdapter)
	if !ok {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: subagents ainda não são suportados pelo adaptador %s (pulado)", ad.ID(), ad.Name()))
		return nil
	}
	root, err := aa.AgentsRoot(scope, env)
	if err != nil {
		return fmt.Errorf("%s: %w", ad.ID(), err)
	}
	fp := newFilePlanner(plan, lock, p.Name, ad.ID(), manifest.ModuleAgents, root)
	for _, name := range p.Modules.Agents {
		s, err := p.Subagent(name)
		if err != nil {
			return err
		}
		file, content, notes, err := aa.Subagent(s)
		plan.Notes = append(plan.Notes, notes...)
		if err != nil {
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: subagent %s pulado: %v", ad.ID(), name, err))
			continue
		}
		if err := fp.add(Action{Path: filepath.Join(root, file), Content: content, SHA256: hashBytes(content)}); err != nil {
			return err
		}
	}
	return fp.finish()
}

// planHooks copia os hooks para HooksDir e liga um dispatcher por evento na
// config do agente. Agentes sem hooks seguem o fallback de cada hook.
func planHooks(plan *Plan, lock *lockfile.Lock, p *manifest.Pack, ad adapter.Adapter, scope adapter.Scope, env adapter.Env) error {
	hooks := make([]manifest.Hook, 0, len(p.Modules.Hooks))
	for _, name := range p.Modules.Hooks {
		h, err := p.Hook(name)
		if err != nil {
			return err
		}
		hooks = append(hooks, h)
	}
	ha, ok := ad.(adapter.HooksAdapter)
	if !ok || !ad.Supports(manifest.ModuleHooks) {
		return planHookFallbacks(plan, lock, p, ad, hooks, scope, env)
	}
	if env.Exe == "" {
		return errors.New("não sei o caminho do binário atlana para ligar os hooks")
	}
	if strings.Contains(env.Exe, "go-build") {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o atlana está rodando de um binário temporário (%s); os hooks vão quebrar quando ele sumir. Instale com go install", ad.ID(), env.Exe))
	}

	dir, err := HooksDir(scope, env, ad.ID())
	if err != nil {
		return err
	}
	fp := newFilePlanner(plan, lock, p.Name, ad.ID(), manifest.ModuleHooks, dir)
	events := map[string]bool{}
	for _, h := range hooks {
		native, ok := ha.HookEvent(h.Event)
		if !ok {
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o evento %s do hook %s não existe neste agente (pulado)", ad.ID(), h.Event, h.Name))
			continue
		}
		events[h.Event] = true
		desc := h.Event
		if len(h.Match.Tool) > 0 {
			desc += " · " + strings.Join(h.Match.Tool, ",")
		}
		if len(h.Match.Path) > 0 {
			desc += " · " + strings.Join(h.Match.Path, ",")
		}
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o hook %s vai rodar hooks/%s/%s na sua máquina (%s → %s)", ad.ID(), h.Name, h.Name, filepath.Clean(h.Run), desc, native))
		if err := copyTree(fp, h.Dir, filepath.Join(dir, p.Name, h.Name)); err != nil {
			return err
		}
	}
	if err := fp.finish(); err != nil {
		return err
	}

	kc, err := configFor(ad.ID(), manifest.ModuleHooks, scope, env)
	if err != nil {
		return err
	}
	doc, base, err := openConfig(plan, kc.Path, kc.open)
	if err != nil {
		plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s: %v", ad.ID(), err))
		return nil
	}
	prev, _ := lock.Find(p.Name, ad.ID(), string(manifest.ModuleHooks))
	owned := map[string]string{}
	for _, k := range prev.Keys {
		owned[k.Name] = k.SHA256
	}
	var acts []Action
	planned := map[string]bool{}
	evs := make([]string, 0, len(events))
	for e := range events {
		evs = append(evs, e)
	}
	sort.Strings(evs)
	for _, e := range evs {
		native, _ := ha.HookEvent(e)
		item := ha.HookEntry(native, adapter.DispatchCommand(env.Exe, ad.ID(), e, dir))
		want, err := doc.HashOf(native, item)
		if err != nil {
			return err
		}
		planned[native] = true
		a := Action{Agent: ad.ID(), Module: manifest.ModuleHooks, Path: kc.Path, Key: native, SHA256: want, Base: base}
		cur, exists := doc.Hash(native)
		switch {
		case !exists:
			a.Op = OpCreate
		case cur == want:
			a.Op = OpUnchanged
		default:
			// O item é reconhecido pelo comando do dispatcher: é nosso; muda quando o binário muda de lugar.
			a.Op = OpUpdate
		}
		if a.Op != OpUnchanged {
			if err := doc.Set(native, item); err != nil {
				return err
			}
		}
		acts = append(acts, a)
	}
	for _, k := range prev.Keys {
		if planned[k.Name] {
			continue
		}
		if cur, ok := doc.Hash(k.Name); ok && len(lock.KeyOwners(k.Path, k.Name)) <= 1 {
			doc.Delete(k.Name)
			acts = append(acts, Action{Op: OpRemove, Agent: ad.ID(), Module: manifest.ModuleHooks, Path: kc.Path, Key: k.Name, SHA256: cur, Base: base})
		}
	}
	content := doc.Bytes()
	for i := range acts {
		acts[i].Content = content
	}
	plan.Actions = append(plan.Actions, acts...)
	return nil
}

// planHookFallbacks aplica rule | skip | fail para agentes sem hooks.
func planHookFallbacks(plan *Plan, lock *lockfile.Lock, p *manifest.Pack, ad adapter.Adapter, hooks []manifest.Hook, scope adapter.Scope, env adapter.Env) error {
	var rules []adapter.RuleFile
	for _, h := range hooks {
		switch h.Fallback {
		case "fail":
			plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s: o hook %s exige suporte a hooks (fallback: fail), e o %s não tem", ad.ID(), h.Name, ad.Name()))
		case "skip":
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: sem suporte a hooks; o hook %s foi pulado (fallback: skip)", ad.ID(), h.Name))
		case "rule":
			b, err := os.ReadFile(h.RulePath())
			if err != nil {
				return err
			}
			rules = append(rules, adapter.RuleFile{Rel: "hooks/" + h.Name + ".md", Content: b})
		}
	}
	if len(rules) == 0 {
		return nil
	}
	ra, ok := ad.(adapter.RulesAdapter)
	if !ok {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: sem hooks e sem rules; os hooks com fallback: rule foram pulados", ad.ID()))
		return nil
	}
	root, err := ra.RulesRoot(scope, env)
	if errors.Is(err, adapter.ErrUnsupported) {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: fallback rule pulado: %v", ad.ID(), err))
		return nil
	}
	if err != nil {
		return err
	}
	outs, notes, err := ra.Rules(p.Name, rules, scope, env)
	if err != nil {
		return err
	}
	fp := newFilePlanner(plan, lock, p.Name, ad.ID(), manifest.ModuleHooks, root)
	for _, o := range outs {
		if err := fp.add(Action{Path: o.Path, Content: o.Content, SHA256: hashBytes(o.Content)}); err != nil {
			return err
		}
	}
	plan.Notes = append(plan.Notes, fmt.Sprintf("%s: sem suporte a hooks; %d hook(s) instalado(s) como rule (fallback: rule)", ad.ID(), len(outs)))
	plan.Notes = append(plan.Notes, notes...)
	return fp.finish()
}

// copyTree planeja a cópia de src para dst (sem links simbólicos).
func copyTree(fp *filePlanner, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: links simbólicos não são permitidos", path)
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		return fp.add(Action{Path: filepath.Join(dst, rel), Source: path, SHA256: sum})
	})
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Package installer monta o plano (o que vai mudar) e só depois aplica.
// Princípios: plano antes de aplicar, nunca sobrescrever arquivo que não é nosso,
// e registrar tudo no atlana.lock para status/uninstall.
package installer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/atlana-ai-hq/atlana/internal/adapter"
	"github.com/atlana-ai-hq/atlana/internal/configmerge"
	"github.com/atlana-ai-hq/atlana/internal/lockfile"
	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// Op é o tipo de ação num arquivo.
type Op string

const (
	OpCreate    Op = "criar"
	OpUpdate    Op = "atualizar"
	OpUnchanged Op = "igual"
	OpRemove    Op = "remover"
)

// Action é uma mudança planejada num arquivo.
type Action struct {
	Op      Op
	Agent   string
	Module  manifest.Module
	Path    string // destino absoluto
	Source  string // origem (vazio em remoções e em conteúdo gerado)
	Content []byte // conteúdo gerado pelo adaptador (no lugar de Source)
	// Key != "" marca um item dentro de um arquivo que não é nosso (ex.: um
	// servidor MCP). Content é então o arquivo inteiro depois do merge, e Base
	// é o hash do arquivo no momento do plano ("" se ele não existia).
	Key    string
	Base   string
	SHA256 string // hash do conteúdo final (ou do arquivo removido)
}

// Target é uma combinação agente+módulo tocada pelo plano.
type Target struct {
	Agent  string
	Module manifest.Module
	Root   string
}

// Kind diferencia instalação de desinstalação.
type Kind string

const (
	KindInstall   Kind = "install"
	KindUninstall Kind = "uninstall"
)

// Plan é tudo que vai acontecer, calculado sem tocar no disco.
type Plan struct {
	Kind        Kind
	PackName    string
	PackVersion string
	Source      string
	Scope       adapter.Scope
	LockPath    string
	Actions     []Action
	Targets     []Target
	Notes       []string // itens pulados e avisos
	Conflicts   []string // impedem o apply
}

// Changes conta as ações que mexem no disco.
func (p *Plan) Changes() int {
	n := 0
	for _, a := range p.Actions {
		if a.Op != OpUnchanged {
			n++
		}
	}
	return n
}

// ErrStale é devolvido por Apply quando uma config mudou entre o plano e a aplicação.
var ErrStale = errors.New("a configuração mudou desde o plano; nada foi gravado nela, rode de novo")

// ErrConflicts é devolvido por Apply quando o plano tem conflitos.
var ErrConflicts = errors.New("o plano tem conflitos; nada foi alterado")

// InstallRequest descreve um pedido de instalação.
type InstallRequest struct {
	Pack    *manifest.Pack
	Agents  []adapter.Adapter
	Modules []manifest.Module // vazio = todos os módulos declarados no pack
	Scope   adapter.Scope
	Env     adapter.Env
	// Origin é o que vai para o lock como origem (ex.: github:o/r//p@<commit>); vazio = Pack.Dir.
	Origin string
}

// LockPath resolve o lockfile do escopo.
func LockPath(scope adapter.Scope, env adapter.Env) string {
	if scope == adapter.ScopeProject {
		return lockfile.ProjectPath(env.WorkDir)
	}
	xdg := ""
	if env.Getenv != nil {
		xdg = env.Getenv("XDG_CONFIG_HOME")
	}
	return lockfile.UserPath(env.Home, xdg)
}

// PlanInstall calcula o plano de instalação.
func PlanInstall(req InstallRequest) (*Plan, error) {
	p := req.Pack
	plan := &Plan{
		Kind: KindInstall, PackName: p.Name, PackVersion: p.Version, Source: p.Dir,
		Scope: req.Scope, LockPath: LockPath(req.Scope, req.Env),
	}
	if req.Origin != "" {
		plan.Source = req.Origin
	}
	lock, err := lockfile.Load(plan.LockPath)
	if err != nil {
		return nil, err
	}

	explicit := len(req.Modules) > 0
	modules := req.Modules
	if !explicit {
		for _, m := range manifest.AllModules {
			if len(p.Modules.Items(m)) == 0 {
				continue
			}
			if m == manifest.ModuleHooks {
				// Hooks rodam comandos na máquina: nunca entram sem pedido explícito.
				plan.Notes = append(plan.Notes, "o pack tem hooks; eles só são instalados com --only hooks")
				continue
			}
			modules = append(modules, m)
		}
	}

	for _, ad := range req.Agents {
		if !p.Targets.Allows(ad.ID()) {
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o pack exclui este agente (targets)", ad.ID()))
			continue
		}
		for _, m := range modules {
			if len(p.Modules.Items(m)) == 0 {
				if explicit {
					plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o pack não declara %s", ad.ID(), m))
				}
				continue
			}
			if !ad.Supports(m) && m != manifest.ModuleHooks { // hooks têm fallback
				if ad.ID() == "codex" && m == manifest.ModuleCommands {
					plan.Notes = append(plan.Notes, "codex: os custom prompts do Codex foram descontinuados em favor de skills; transforme o command numa skill (pulado)")
				} else {
					plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %s ainda não é suportado pelo adaptador %s (pulado)", ad.ID(), m, ad.Name()))
				}
				continue
			}
			switch m {
			case manifest.ModuleSkills:
				if err := planSkills(plan, lock, p, ad, req.Scope, req.Env); err != nil {
					return nil, err
				}
			case manifest.ModuleRules:
				if err := planRules(plan, lock, p, ad, req.Scope, req.Env); err != nil {
					return nil, err
				}
			case manifest.ModuleMCP:
				if err := planMCP(plan, lock, p, ad, req.Scope, req.Env); err != nil {
					return nil, err
				}
			case manifest.ModuleAgents:
				if err := planAgents(plan, lock, p, ad, req.Scope, req.Env); err != nil {
					return nil, err
				}
			case manifest.ModuleHooks:
				if err := planHooks(plan, lock, p, ad, req.Scope, req.Env); err != nil {
					return nil, err
				}
			case manifest.ModuleCommands:
				if err := planCommands(plan, lock, p, ad, req.Scope, req.Env); err != nil {
					return nil, err
				}
			default:
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %s ainda não implementado (pulado)", ad.ID(), m))
			}
		}
	}
	return plan, nil
}

func planSkills(plan *Plan, lock *lockfile.Lock, p *manifest.Pack, ad adapter.Adapter, scope adapter.Scope, env adapter.Env) error {
	root, err := ad.SkillsRoot(scope, env)
	if err != nil {
		return fmt.Errorf("%s: %w", ad.ID(), err)
	}
	fp := newFilePlanner(plan, lock, p.Name, ad.ID(), manifest.ModuleSkills, root)

	for _, skill := range p.Modules.Skills {
		src := p.SkillDir(skill)
		err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("%s: links simbólicos não são permitidos em skills", path)
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
			return fp.add(Action{Path: filepath.Join(root, skill, rel), Source: path, SHA256: sum})
		})
		if err != nil {
			return err
		}
	}
	return fp.finish()
}

// planRules pede ao adaptador os arquivos de rules e planeja cada um como conteúdo gerado.
func planRules(plan *Plan, lock *lockfile.Lock, p *manifest.Pack, ad adapter.Adapter, scope adapter.Scope, env adapter.Env) error {
	ra, ok := ad.(adapter.RulesAdapter)
	if !ok {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: rules ainda não é suportado pelo adaptador %s (pulado)", ad.ID(), ad.Name()))
		return nil
	}
	root, err := ra.RulesRoot(scope, env)
	if errors.Is(err, adapter.ErrUnsupported) {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %v (pulado)", ad.ID(), err))
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", ad.ID(), err)
	}
	files := make([]adapter.RuleFile, 0, len(p.Modules.Rules))
	for _, r := range p.Modules.Rules {
		b, err := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(r)))
		if err != nil {
			return err
		}
		files = append(files, adapter.RuleFile{Rel: filepath.ToSlash(filepath.Clean(r)), Content: b})
	}
	outs, notes, err := ra.Rules(p.Name, files, scope, env)
	if err != nil {
		return fmt.Errorf("%s: %w", ad.ID(), err)
	}
	fp := newFilePlanner(plan, lock, p.Name, ad.ID(), manifest.ModuleRules, root)
	for _, o := range outs {
		h := sha256.Sum256(o.Content)
		if err := fp.add(Action{Path: o.Path, Content: o.Content, SHA256: hex.EncodeToString(h[:])}); err != nil {
			return err
		}
	}
	if err := fp.finish(); err != nil {
		return err
	}
	plan.Notes = append(plan.Notes, notes...)
	if rc, ok := ad.(adapter.RulesConfigAdapter); ok {
		return planRulesConfig(plan, lock, p, ad, rc, scope, env, len(outs) > 0)
	}
	return nil
}

// planRulesConfig põe (ou tira) da config do agente o item que carrega as rules do pack.
func planRulesConfig(plan *Plan, lock *lockfile.Lock, p *manifest.Pack, ad adapter.Adapter, rc adapter.RulesConfigAdapter, scope adapter.Scope, env adapter.Env, want bool) error {
	cfg, entry, err := rc.RulesConfig(p.Name, scope, env)
	if err != nil {
		return fmt.Errorf("%s: %w", ad.ID(), err)
	}
	kc, err := configFor(ad.ID(), manifest.ModuleRules, scope, env)
	if err != nil {
		return err
	}
	doc, base, err := openConfig(plan, cfg.Path, kc.open)
	if err != nil {
		plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s: %v", ad.ID(), err))
		return nil
	}
	prev, _ := lock.Find(p.Name, ad.ID(), string(manifest.ModuleRules))
	var acts []Action
	if want {
		h, _ := doc.HashOf(entry, nil)
		a := Action{Agent: ad.ID(), Module: manifest.ModuleRules, Path: cfg.Path, Key: entry, SHA256: h, Base: base, Op: OpUnchanged}
		if _, ok := doc.Hash(entry); !ok {
			a.Op = OpCreate
			if err := doc.Set(entry, nil); err != nil {
				return err
			}
		}
		acts = append(acts, a)
	}
	for _, k := range prev.Keys {
		if want && k.Name == entry {
			continue
		}
		if cur, ok := doc.Hash(k.Name); ok && len(lock.KeyOwners(k.Path, k.Name)) <= 1 {
			doc.Delete(k.Name)
			acts = append(acts, Action{Op: OpRemove, Agent: ad.ID(), Module: manifest.ModuleRules, Path: cfg.Path, Key: k.Name, SHA256: cur, Base: base})
		}
	}
	content := doc.Bytes()
	for i := range acts {
		acts[i].Content = content
	}
	plan.Actions = append(plan.Actions, acts...)
	return nil
}

// planMCP faz o merge dos servidores do pack na config do agente, servidor por servidor.
func planMCP(plan *Plan, lock *lockfile.Lock, p *manifest.Pack, ad adapter.Adapter, scope adapter.Scope, env adapter.Env) error {
	ma, ok := ad.(adapter.MCPAdapter)
	if !ok {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: mcp ainda não é suportado pelo adaptador %s (pulado)", ad.ID(), ad.Name()))
		return nil
	}
	cfg, err := ma.MCPConfig(scope, env)
	if errors.Is(err, adapter.ErrUnsupported) {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %v (pulado)", ad.ID(), err))
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", ad.ID(), err)
	}
	defs, err := p.MCPServers()
	if err != nil {
		return err
	}
	kc, err := configFor(ad.ID(), manifest.ModuleMCP, scope, env)
	if err != nil {
		return err
	}
	doc, base, err := openConfig(plan, cfg.Path, kc.open)
	if err != nil {
		plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s: %v", ad.ID(), err))
		return nil
	}
	plan.Targets = append(plan.Targets, Target{Agent: ad.ID(), Module: manifest.ModuleMCP, Root: cfg.Path})

	prev, _ := lock.Find(p.Name, ad.ID(), string(manifest.ModuleMCP))
	owned := map[string]string{}
	for _, k := range prev.Keys {
		if k.Path == cfg.Path {
			owned[k.Name] = k.SHA256
		}
	}
	var acts []Action
	planned := map[string]bool{}
	for _, name := range p.Modules.MCP {
		val, err := ma.MCPServer(defs[name], p.Dir)
		if err != nil {
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: servidor %s pulado: %v", ad.ID(), name, err))
			continue
		}
		want, err := doc.HashOf(name, val)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", ad.ID(), name, err)
		}
		planned[name] = true
		a := Action{Agent: ad.ID(), Module: manifest.ModuleMCP, Path: cfg.Path, Key: name, SHA256: want, Base: base}
		cur, exists := doc.Hash(name)
		switch {
		case !exists:
			a.Op = OpCreate
		case cur == want:
			a.Op = OpUnchanged
		case owned[name] == cur:
			a.Op = OpUpdate
		default:
			plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s: o servidor %q já existe em %s e não foi instalado pelo atlana (ou foi editado depois)", ad.ID(), name, cfg.Path))
			continue
		}
		if a.Op != OpUnchanged {
			if err := doc.Set(name, val); err != nil {
				return err
			}
		}
		acts = append(acts, a)
	}
	for _, k := range prev.Keys {
		if k.Path != cfg.Path || planned[k.Name] {
			continue
		}
		cur, exists := doc.Hash(k.Name)
		switch {
		case !exists:
		case cur == k.SHA256:
			doc.Delete(k.Name)
			acts = append(acts, Action{Op: OpRemove, Agent: ad.ID(), Module: manifest.ModuleMCP, Path: cfg.Path, Key: k.Name, SHA256: cur, Base: base})
		default:
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o servidor %q saiu do pack mas foi editado; mantido", ad.ID(), k.Name))
		}
	}
	content := doc.Bytes()
	for i := range acts {
		acts[i].Content = content
	}
	plan.Actions = append(plan.Actions, acts...)
	return nil
}

// keyedConfig diz como abrir a config que guarda os itens (keys) de um módulo.
type keyedConfig struct {
	adapter.ConfigFile
	open func(raw []byte) (configmerge.Doc, error)
}

// configFor resolve, pelo adaptador, a config de mcp ou de hooks de um agente.
func configFor(agent string, module manifest.Module, scope adapter.Scope, env adapter.Env) (keyedConfig, error) {
	ad, err := adapter.Get(agent)
	if err != nil {
		return keyedConfig{}, err
	}
	switch module {
	case manifest.ModuleMCP:
		if ma, ok := ad.(adapter.MCPAdapter); ok {
			cfg, err := ma.MCPConfig(scope, env)
			return keyedConfig{cfg, func(raw []byte) (configmerge.Doc, error) {
				return configmerge.Parse(configmerge.Format(cfg.Format), raw, cfg.Section)
			}}, err
		}
	case manifest.ModuleRules:
		if rc, ok := ad.(adapter.RulesConfigAdapter); ok {
			cfg, _, err := rc.RulesConfig("", scope, env)
			return keyedConfig{cfg, func(raw []byte) (configmerge.Doc, error) {
				return configmerge.ParseStringList(raw, cfg.Section)
			}}, err
		}
	case manifest.ModuleHooks:
		if ha, ok := ad.(adapter.HooksAdapter); ok {
			cfg, err := ha.HooksConfig(scope, env)
			if err != nil {
				return keyedConfig{}, err
			}
			dir, err := HooksDir(scope, env, agent)
			return keyedConfig{cfg, func(raw []byte) (configmerge.Doc, error) {
				return configmerge.ParseArrayItems(raw, cfg.Section, dispatcherMatch(agent, dir))
			}}, err
		}
	}
	return keyedConfig{}, fmt.Errorf("%s: não sei editar a config de %s", agent, module)
}

// openConfig abre a config para um plano. Se o plano já mexeu no arquivo (ex.:
// mcp e hooks no mesmo settings.json), parte do conteúdo pendente, para que um
// merge não apague o outro. Devolve também o hash do arquivo no disco ("" se não existe).
func openConfig(plan *Plan, path string, open func([]byte) (configmerge.Doc, error)) (configmerge.Doc, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	base := ""
	if err == nil {
		h := sha256.Sum256(raw)
		base = hex.EncodeToString(h[:])
	}
	for _, a := range plan.Actions {
		if a.Key != "" && a.Path == path {
			raw = a.Content
		}
	}
	doc, err := open(raw)
	if err != nil {
		return nil, "", fmt.Errorf("não dá para editar %s: %w", path, err)
	}
	return doc, base, nil
}

// filePlanner decide criar/atualizar/igual/conflito por arquivo e remove o que saiu do pack.
type filePlanner struct {
	plan    *Plan
	agent   string
	module  manifest.Module
	prev    lockfile.Entry
	owned   map[string]string
	planned map[string]bool
}

func newFilePlanner(plan *Plan, lock *lockfile.Lock, pack, agent string, module manifest.Module, root string) *filePlanner {
	plan.Targets = append(plan.Targets, Target{Agent: agent, Module: module, Root: root})
	prev, _ := lock.Find(pack, agent, string(module))
	owned := map[string]string{}
	for _, f := range prev.Files {
		owned[f.Path] = f.SHA256
	}
	return &filePlanner{plan: plan, agent: agent, module: module, prev: prev, owned: owned, planned: map[string]bool{}}
}

// add classifica a ação a partir do arquivo atual no destino (sem escrever nada).
func (fp *filePlanner) add(a Action) error {
	a.Agent, a.Module = fp.agent, fp.module
	fp.planned[a.Path] = true
	cur, err := hashFile(a.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		a.Op = OpCreate
	case err != nil:
		return err
	case cur == a.SHA256:
		a.Op = OpUnchanged
	case fp.owned[a.Path] == cur:
		a.Op = OpUpdate
	default:
		fp.plan.Conflicts = append(fp.plan.Conflicts, fmt.Sprintf("%s: %s já existe e não foi instalado pelo atlana (ou foi editado depois)", fp.agent, a.Path))
		return nil
	}
	fp.plan.Actions = append(fp.plan.Actions, a)
	return nil
}

// finish planeja a remoção de arquivos que eram nossos e saíram do pack, se ninguém editou.
func (fp *filePlanner) finish() error {
	for _, f := range fp.prev.Files {
		if fp.planned[f.Path] {
			continue
		}
		cur, err := hashFile(f.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return err
		case cur == f.SHA256:
			fp.plan.Actions = append(fp.plan.Actions, Action{Op: OpRemove, Agent: fp.agent, Module: fp.module, Path: f.Path, SHA256: cur})
		default:
			fp.plan.Notes = append(fp.plan.Notes, fmt.Sprintf("%s: %s saiu do pack mas foi editado; mantido", fp.agent, f.Path))
		}
	}
	return nil
}

// PlanUninstall calcula a remoção exata do que o lock registrou para o pack.
// agents vazio = todos os agentes do lock.
func PlanUninstall(pack string, agents []string, scope adapter.Scope, env adapter.Env) (*Plan, error) {
	plan := &Plan{Kind: KindUninstall, PackName: pack, Scope: scope, LockPath: LockPath(scope, env)}
	lock, err := lockfile.Load(plan.LockPath)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, a := range agents {
		want[a] = true
	}
	for _, e := range lock.Entries {
		if e.Pack != pack || (len(want) > 0 && !want[e.Agent]) {
			continue
		}
		plan.PackVersion = e.Version
		plan.Source = e.Source
		plan.Targets = append(plan.Targets, Target{Agent: e.Agent, Module: manifest.Module(e.Module), Root: e.Root})
		for _, f := range e.Files {
			cur, err := hashFile(f.Path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %s já não existe", e.Agent, f.Path))
			case err != nil:
				return nil, err
			case cur != f.SHA256:
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %s foi editado depois da instalação; mantido", e.Agent, f.Path))
			case len(lock.Owners(f.Path)) > 1:
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %s também pertence a outro pack; mantido", e.Agent, f.Path))
			default:
				plan.Actions = append(plan.Actions, Action{Op: OpRemove, Agent: e.Agent, Module: manifest.Module(e.Module), Path: f.Path, SHA256: cur})
			}
		}
		if err := planKeyRemovals(plan, lock, e, scope, env); err != nil {
			return nil, err
		}
	}
	if len(plan.Targets) == 0 {
		return nil, fmt.Errorf("o pack %q não está instalado no escopo %s (lock: %s)", pack, scope, plan.LockPath)
	}
	return plan, nil
}

// planKeyRemovals tira das configs os itens que a entrada mantém, se ninguém os editou.
func planKeyRemovals(plan *Plan, lock *lockfile.Lock, e lockfile.Entry, scope adapter.Scope, env adapter.Env) error {
	if len(e.Keys) == 0 {
		return nil
	}
	kc, err := configFor(e.Agent, manifest.Module(e.Module), scope, env)
	if err != nil {
		return err
	}
	byPath := map[string][]lockfile.Key{}
	var paths []string
	for _, k := range e.Keys {
		if _, seen := byPath[k.Path]; !seen {
			paths = append(paths, k.Path)
		}
		byPath[k.Path] = append(byPath[k.Path], k)
	}
	for _, path := range paths {
		doc, base, err := openConfig(plan, path, kc.open)
		if err != nil {
			plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s: %v", e.Agent, err))
			continue
		}
		var acts []Action
		for _, k := range byPath[path] {
			cur, exists := doc.Hash(k.Name)
			switch {
			case !exists:
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o servidor %q já não está em %s", e.Agent, k.Name, path))
			case cur != k.SHA256:
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o servidor %q em %s foi editado depois da instalação; mantido", e.Agent, k.Name, path))
			case len(lock.KeyOwners(path, k.Name)) > 1:
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: o servidor %q também pertence a outro pack; mantido", e.Agent, k.Name))
			default:
				doc.Delete(k.Name)
				acts = append(acts, Action{Op: OpRemove, Agent: e.Agent, Module: manifest.Module(e.Module), Path: path, Key: k.Name, SHA256: cur, Base: base})
			}
		}
		content := doc.Bytes()
		for i := range acts {
			acts[i].Content = content
		}
		plan.Actions = append(plan.Actions, acts...)
	}
	return nil
}

// Apply executa o plano e atualiza o lockfile. Recusa planos com conflitos.
func Apply(plan *Plan, now time.Time) error {
	if len(plan.Conflicts) > 0 {
		return ErrConflicts
	}
	lock, err := lockfile.Load(plan.LockPath)
	if err != nil {
		return err
	}
	roots := map[string]string{}
	for _, t := range plan.Targets {
		roots[t.Agent+"|"+string(t.Module)] = t.Root
	}

	// Configs editadas por merge: confere se ninguém mexeu desde o plano e grava uma vez.
	merged := map[string]bool{}
	for _, a := range plan.Actions {
		if a.Key == "" || a.Op == OpUnchanged || merged[a.Path] {
			continue
		}
		merged[a.Path] = true
		cur, err := hashFile(a.Path)
		if errors.Is(err, fs.ErrNotExist) {
			cur, err = "", nil
		}
		if err != nil {
			return err
		}
		if cur != a.Base {
			return fmt.Errorf("%w: %s", ErrStale, a.Path)
		}
	}
	// A última ação de cada arquivo tem o conteúdo mais completo (openConfig encadeia).
	final := map[string][]byte{}
	for _, a := range plan.Actions {
		if a.Key != "" && merged[a.Path] {
			final[a.Path] = a.Content
		}
	}
	for path, content := range final {
		if err := writeConfig(path, content); err != nil {
			return err
		}
	}

	for _, a := range plan.Actions {
		if a.Key != "" {
			continue
		}
		switch a.Op {
		case OpCreate, OpUpdate:
			if a.Content != nil {
				err = writeFile(a.Path, a.Content)
			} else {
				err = copyFile(a.Source, a.Path)
			}
			if err != nil {
				return err
			}
		case OpRemove:
			if err := os.Remove(a.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			pruneEmptyDirs(filepath.Dir(a.Path), roots[a.Agent+"|"+string(a.Module)])
		}
	}

	for _, t := range plan.Targets {
		if plan.Kind == KindUninstall {
			lock.Delete(plan.PackName, t.Agent, string(t.Module))
			continue
		}
		e := lockfile.Entry{
			Pack: plan.PackName, Version: plan.PackVersion, Source: plan.Source,
			Agent: t.Agent, Module: string(t.Module), Root: t.Root,
			InstalledAt: now.UTC().Format(time.RFC3339),
		}
		for _, a := range plan.Actions {
			if a.Agent != t.Agent || a.Module != t.Module || a.Op == OpRemove {
				continue
			}
			if a.Key != "" {
				e.Keys = append(e.Keys, lockfile.Key{Path: a.Path, Name: a.Key, SHA256: a.SHA256})
			} else {
				e.Files = append(e.Files, lockfile.File{Path: a.Path, SHA256: a.SHA256})
			}
		}
		if len(e.Files) == 0 && len(e.Keys) == 0 {
			lock.Delete(plan.PackName, t.Agent, string(t.Module))
		} else {
			lock.Put(e)
		}
	}
	return lock.Save(plan.LockPath)
}

// FileState é o estado de um arquivo registrado no lock.
type FileState string

const (
	StateOK       FileState = "ok"
	StateModified FileState = "editado"
	StateMissing  FileState = "ausente"
)

// StatusEntry resume uma entrada do lock.
type StatusEntry struct {
	Scope  adapter.Scope
	Entry  lockfile.Entry
	States map[string]FileState
}

// Status lê os lockfiles de usuário e de projeto e confere cada arquivo.
func Status(env adapter.Env) ([]StatusEntry, error) {
	var out []StatusEntry
	for _, scope := range []adapter.Scope{adapter.ScopeUser, adapter.ScopeProject} {
		lock, err := lockfile.Load(LockPath(scope, env))
		if err != nil {
			return nil, err
		}
		for _, e := range lock.Entries {
			se := StatusEntry{Scope: scope, Entry: e, States: map[string]FileState{}}
			for _, f := range e.Files {
				cur, err := hashFile(f.Path)
				switch {
				case errors.Is(err, fs.ErrNotExist):
					se.States[f.Path] = StateMissing
				case err != nil:
					return nil, err
				case cur != f.SHA256:
					se.States[f.Path] = StateModified
				default:
					se.States[f.Path] = StateOK
				}
			}
			if err := keyStates(&se, e, scope, env); err != nil {
				return nil, err
			}
			out = append(out, se)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Entry.Key() < out[j].Entry.Key() })
	return out, nil
}

// keyStates preenche o estado de cada item mantido em config (chave "<arquivo>#<nome>").
func keyStates(se *StatusEntry, e lockfile.Entry, scope adapter.Scope, env adapter.Env) error {
	if len(e.Keys) == 0 {
		return nil
	}
	kc, err := configFor(e.Agent, manifest.Module(e.Module), scope, env)
	if err != nil {
		return err
	}
	docs := map[string]configmerge.Doc{}
	for _, k := range e.Keys {
		id := k.Path + "#" + k.Name
		doc, ok := docs[k.Path]
		if !ok {
			raw, err := os.ReadFile(k.Path)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if doc, err = kc.open(raw); err != nil {
				se.States[id] = StateModified
				continue
			}
			docs[k.Path] = doc
		}
		cur, exists := doc.Hash(k.Name)
		switch {
		case !exists:
			se.States[id] = StateMissing
		case cur != k.SHA256:
			se.States[id] = StateModified
		default:
			se.States[id] = StateOK
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile grava de forma atômica (temp + rename), preservando o bit de execução.
func copyFile(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	mode := fs.FileMode(0o644)
	if st.Mode()&0o111 != 0 {
		mode = 0o755
	}
	return writeAtomic(dst, in, mode)
}

// writeConfig grava uma config editada por merge, mantendo as permissões do arquivo.
func writeConfig(dst string, content []byte) error {
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(dst); err == nil {
		mode = st.Mode().Perm()
	}
	return writeAtomic(dst, bytes.NewReader(content), mode)
}

// writeFile grava conteúdo gerado de forma atômica.
func writeFile(dst string, content []byte) error {
	return writeAtomic(dst, bytes.NewReader(content), 0o644)
}

func writeAtomic(dst string, r io.Reader, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".atlana-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// pruneEmptyDirs remove diretórios vazios de dir até root (sem remover root).
func pruneEmptyDirs(dir, root string) {
	if root == "" {
		return
	}
	root = filepath.Clean(root)
	for {
		dir = filepath.Clean(dir)
		if dir == root || !strings.HasPrefix(dir, root+string(filepath.Separator)) {
			return
		}
		if err := os.Remove(dir); err != nil { // falha se não estiver vazio
			return
		}
		dir = filepath.Dir(dir)
	}
}

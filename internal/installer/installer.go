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
	SHA256  string // hash do conteúdo final (ou do arquivo removido)
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

// ErrConflicts é devolvido por Apply quando o plano tem conflitos.
var ErrConflicts = errors.New("o plano tem conflitos; nada foi alterado")

// InstallRequest descreve um pedido de instalação.
type InstallRequest struct {
	Pack    *manifest.Pack
	Agents  []adapter.Adapter
	Modules []manifest.Module // vazio = todos os módulos declarados no pack
	Scope   adapter.Scope
	Env     adapter.Env
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
	lock, err := lockfile.Load(plan.LockPath)
	if err != nil {
		return nil, err
	}

	explicit := len(req.Modules) > 0
	modules := req.Modules
	if !explicit {
		for _, m := range manifest.AllModules {
			if len(p.Modules.Items(m)) > 0 {
				modules = append(modules, m)
			}
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
			if !ad.Supports(m) {
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %s ainda não é suportado pelo adaptador %s (pulado)", ad.ID(), m, ad.Name()))
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
	return nil
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
	}
	if len(plan.Targets) == 0 {
		return nil, fmt.Errorf("o pack %q não está instalado no escopo %s (lock: %s)", pack, scope, plan.LockPath)
	}
	return plan, nil
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

	for _, a := range plan.Actions {
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
			if a.Agent == t.Agent && a.Module == t.Module && a.Op != OpRemove {
				e.Files = append(e.Files, lockfile.File{Path: a.Path, SHA256: a.SHA256})
			}
		}
		if len(e.Files) == 0 {
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
			out = append(out, se)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Entry.Key() < out[j].Entry.Key() })
	return out, nil
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

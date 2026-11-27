// Package cli implementa os comandos da atlana com a biblioteca padrão (flag).
package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/atlana-ai-hq/atlana/internal/adapter"
	"github.com/atlana-ai-hq/atlana/internal/hookrun"
	"github.com/atlana-ai-hq/atlana/internal/installer"
	"github.com/atlana-ai-hq/atlana/internal/manifest"
	"github.com/atlana-ai-hq/atlana/internal/registry"
	"github.com/atlana-ai-hq/atlana/internal/source"
	"github.com/atlana-ai-hq/atlana/spec"
)

// Version é sobrescrita no build de release via -ldflags "-X .../internal/cli.Version=x.y.z".
var Version = "0.1.0-dev"

// App carrega as dependências do processo; nos testes, tudo é falso.
type App struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Env    adapter.Env
	Now    func() time.Time
	// Interactive diz se podemos perguntar "Aplicar?" no terminal.
	Interactive bool
	// Fetcher busca origens remotas; nil = git da pessoa, com cache em ~/.cache/atlana.
	Fetcher *source.Fetcher
}

// ErrUsage indica erro de uso (exit 2).
var ErrUsage = errors.New("uso inválido")

const usage = `atlana: um formato, todos os agentes.

Uso:
  atlana <comando> [argumentos]

Comandos:
  version                       mostra a versão
  doctor                        detecta agentes instalados e o que cada adaptador suporta
  validate [caminho]            valida packs (atlana.yaml + SKILL.md); padrão: .
  install <pack> [flags]        planeja e instala um pack: caminho local, github:owner/repo//caminho@ref
                                ou <nome-no-registry>//caminho@ref
  uninstall <nome-do-pack>      remove exatamente o que o atlana.lock registrou
  status                        mostra o que está instalado e se foi editado
  registry list [--all]         projetos do registry local (privados só com acesso)
  registry add <nome> <origem> [--private]
  registry remove <nome>
  hook dispatch [flags]         uso interno: chamado pelos agentes quando um hook dispara

Flags de install/uninstall:
  --agent cursor,claude         agentes alvo (padrão: os detectados)
  --only skills                 módulos (rules,skills,agents,hooks,commands,mcp);
                                hooks só entram quando pedidos aqui
  --scope user|project          onde instalar (padrão: user)
  --dry-run                     só mostra o plano
  --yes                         aplica sem perguntar (CI)
`

// Run executa a CLI e devolve o código de saída.
func (a *App) Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "version", "--version", "-v":
		err = a.version()
	case "doctor":
		err = a.doctor()
	case "validate":
		err = a.validate(args[1:])
	case "install":
		err = a.install(args[1:])
	case "uninstall":
		err = a.uninstall(args[1:])
	case "hook":
		return a.hook(args[1:])
	case "registry":
		err = a.registry(args[1:])
	case "status":
		err = a.status()
	case "help", "--help", "-h":
		fmt.Fprint(a.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(a.Stderr, "comando desconhecido %q\n\n%s", args[0], usage)
		return 2
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(a.Stderr, "atlana:", err)
	if errors.Is(err, ErrUsage) {
		return 2
	}
	return 1
}

func (a *App) version() error {
	commit := ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				commit = " (" + s.Value[:7] + ")"
			}
		}
	}
	fmt.Fprintf(a.Stdout, "atlana %s%s · spec %s\n", Version, commit, spec.APIVersion)
	return nil
}

func (a *App) doctor() error {
	tw := tabwriter.NewWriter(a.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENTE\tID\tDETECTADO\tMÓDULOS SUPORTADOS\tEVIDÊNCIA")
	for _, ad := range adapter.All() {
		d := ad.Detect(a.Env)
		found := "não"
		if d.Installed {
			found = "sim"
		}
		if adapter.IsExporter(ad) {
			found += " (só exporta)"
		}
		var mods []string
		for _, m := range manifest.AllModules {
			if ad.Supports(m) {
				mods = append(mods, string(m))
			}
		}
		supported := strings.Join(mods, ",")
		if supported == "" {
			supported = "(só detecção por enquanto)"
		}
		ev := strings.Join(d.Evidence, "; ")
		if ev == "" {
			ev = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ad.Name(), ad.ID(), found, supported, ev)
	}
	return tw.Flush()
}

func (a *App) validate(args []string) error {
	root := "."
	if len(args) > 1 {
		return fmt.Errorf("%w: validate aceita um caminho", ErrUsage)
	}
	if len(args) == 1 {
		root = args[0]
	}
	dirs, err := manifest.Find(root)
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		return fmt.Errorf("nenhum %s encontrado em %s", manifest.FileName, root)
	}
	failed := 0
	for _, dir := range dirs {
		p, rep, err := manifest.Load(dir)
		if err != nil && !errors.Is(err, manifest.ErrInvalid) {
			return err
		}
		if rep.OK() {
			fmt.Fprintf(a.Stdout, "ok    %s (%s@%s)\n", dir, p.Name, p.Version)
		} else {
			failed++
			fmt.Fprintf(a.Stdout, "FALHA %s (%d erro(s))\n", dir, rep.Errors())
		}
		for _, is := range rep.Issues {
			fmt.Fprintln(a.Stdout, "  "+is.String())
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d de %d pack(s) inválido(s)", failed, len(dirs))
	}
	return nil
}

type planFlags struct {
	agents string
	only   string
	scope  string
	dryRun bool
	yes    bool
}

// parseInterspersed aceita flags antes ou depois do argumento posicional.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUsage, err)
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func newPlanFlagSet(name string, pf *planFlags, withOnly bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&pf.agents, "agent", "", "agentes alvo separados por vírgula")
	if withOnly {
		fs.StringVar(&pf.only, "only", "", "módulos separados por vírgula")
	}
	fs.StringVar(&pf.scope, "scope", "user", "user ou project")
	fs.BoolVar(&pf.dryRun, "dry-run", false, "só mostra o plano")
	fs.BoolVar(&pf.yes, "yes", false, "aplica sem perguntar")
	return fs
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) install(args []string) error {
	var pf planFlags
	pos, err := parseInterspersed(newPlanFlagSet("install", &pf, true), args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("%w: use `atlana install <caminho-do-pack> [flags]`", ErrUsage)
	}
	scope, err := adapter.ParseScope(pf.scope)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}
	var modules []manifest.Module
	for _, m := range splitList(pf.only) {
		mod, err := manifest.ParseModule(m)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUsage, err)
		}
		modules = append(modules, mod)
	}
	agents, err := a.resolveAgents(splitList(pf.agents))
	if err != nil {
		return err
	}

	dir, origin, err := a.resolvePack(pos[0])
	if err != nil {
		return err
	}
	pack, rep, err := manifest.Load(dir)
	if err != nil {
		if rep != nil {
			for _, is := range rep.Issues {
				fmt.Fprintln(a.Stderr, "  "+is.String())
			}
		}
		return fmt.Errorf("não dá para instalar %s: %w", pos[0], err)
	}

	plan, err := installer.PlanInstall(installer.InstallRequest{
		Pack: pack, Agents: agents, Modules: modules, Scope: scope, Env: a.Env, Origin: origin,
	})
	if err != nil {
		return err
	}
	return a.confirmAndApply(plan, pf)
}

func (a *App) uninstall(args []string) error {
	var pf planFlags
	pos, err := parseInterspersed(newPlanFlagSet("uninstall", &pf, false), args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("%w: use `atlana uninstall <nome-do-pack> [flags]`", ErrUsage)
	}
	scope, err := adapter.ParseScope(pf.scope)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}
	agents := splitList(pf.agents)
	for _, id := range agents {
		if _, err := adapter.Get(id); err != nil {
			return fmt.Errorf("%w: %v", ErrUsage, err)
		}
	}
	plan, err := installer.PlanUninstall(pos[0], agents, scope, a.Env)
	if err != nil {
		return err
	}
	return a.confirmAndApply(plan, pf)
}

func (a *App) resolveAgents(ids []string) ([]adapter.Adapter, error) {
	if len(ids) > 0 {
		var out []adapter.Adapter
		for _, id := range ids {
			ad, err := adapter.Get(id)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrUsage, err)
			}
			out = append(out, ad)
		}
		return out, nil
	}
	var out []adapter.Adapter
	for _, ad := range adapter.All() {
		if ad.Detect(a.Env).Installed {
			out = append(out, ad)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("nenhum agente detectado; use --agent (veja `atlana doctor`)")
	}
	return out, nil
}

func (a *App) confirmAndApply(plan *installer.Plan, pf planFlags) error {
	printPlan(a.Stdout, plan)
	if len(plan.Conflicts) > 0 {
		return installer.ErrConflicts
	}
	if pf.dryRun {
		fmt.Fprintln(a.Stdout, "\n--dry-run: nada foi alterado.")
		return nil
	}
	if plan.Changes() == 0 && plan.Kind == installer.KindInstall {
		// Ainda assim atualiza o lock (ex.: arquivos idênticos que passam a ser nossos).
		if err := installer.Apply(plan, a.Now()); err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, "\nTudo já está atualizado.")
		printExports(a.Stdout, plan)
		return nil
	}
	if !pf.yes {
		if !a.Interactive {
			return fmt.Errorf("%w: sem terminal interativo; revise o plano e rode de novo com --yes (ou use --dry-run)", ErrUsage)
		}
		fmt.Fprint(a.Stdout, "\nAplicar? [s/N] ")
		line, _ := bufio.NewReader(a.Stdin).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "s", "sim", "y", "yes":
		default:
			fmt.Fprintln(a.Stdout, "Cancelado; nada foi alterado.")
			return nil
		}
	}
	if err := installer.Apply(plan, a.Now()); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "\nFeito. Lock: %s\n", plan.LockPath)
	printExports(a.Stdout, plan)
	return nil
}

// printExports avisa onde ficaram as skills de agentes que só exportam (ex.: Grok Bot).
func printExports(w io.Writer, p *installer.Plan) {
	if p.Kind != installer.KindInstall {
		return
	}
	for _, t := range p.Targets {
		ad, err := adapter.Get(t.Agent)
		if err != nil || !adapter.IsExporter(ad) {
			continue
		}
		fmt.Fprintf(w, "%s: skills exportadas em %s; envie cada pasta pelo próprio %s.\n", t.Agent, t.Root, ad.Name())
	}
}

func printPlan(w io.Writer, p *installer.Plan) {
	verb := "Instalar"
	if p.Kind == installer.KindUninstall {
		verb = "Desinstalar"
	}
	ver := ""
	if p.PackVersion != "" {
		ver = "@" + p.PackVersion
	}
	fmt.Fprintf(w, "Plano: %s %s%s (escopo %s)\n", verb, p.PackName, ver, p.Scope)
	counts := map[installer.Op]int{}
	for _, act := range p.Actions {
		counts[act.Op]++
		target := act.Path
		if act.Key != "" {
			target += " → " + act.Key
		}
		fmt.Fprintf(w, "  %-9s [%s/%s] %s\n", act.Op, act.Agent, act.Module, target)
	}
	for _, n := range p.Notes {
		fmt.Fprintln(w, "  nota:", n)
	}
	for _, c := range p.Conflicts {
		fmt.Fprintln(w, "  CONFLITO:", c)
	}
	var parts []string
	for _, op := range []installer.Op{installer.OpCreate, installer.OpUpdate, installer.OpRemove, installer.OpUnchanged} {
		if counts[op] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[op], op))
		}
	}
	if len(parts) == 0 {
		parts = []string{"nenhuma mudança"}
	}
	fmt.Fprintf(w, "Resumo: %s · lock: %s\n", strings.Join(parts, ", "), p.LockPath)
}

func (a *App) status() error {
	entries, err := installer.Status(a.Env)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.Stdout, "Nada instalado pelo atlana.")
		return nil
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PACK\tVERSÃO\tAGENTE\tMÓDULO\tESCOPO\tITENS\tESTADO")
	for _, e := range entries {
		counts := map[installer.FileState]int{}
		for _, s := range e.States {
			counts[s]++
		}
		state := "ok"
		var bad []string
		for _, s := range []installer.FileState{installer.StateModified, installer.StateMissing} {
			if counts[s] > 0 {
				bad = append(bad, fmt.Sprintf("%d %s", counts[s], s))
			}
		}
		if len(bad) > 0 {
			sort.Strings(bad)
			state = strings.Join(bad, ", ")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n", e.Entry.Pack, e.Entry.Version, e.Entry.Agent, e.Entry.Module, e.Scope, len(e.Entry.Files)+len(e.Entry.Keys), state)
	}
	return tw.Flush()
}

// IsTerminal verifica, sem dependências, se f é um terminal.
func IsTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// hook dispatch: lê o payload do agente no stdin e roda os hooks instalados.
// Devolve o exit code direto (2 = negar), sem passar pelo tratamento de erro comum.
func (a *App) hook(args []string) int {
	if len(args) == 0 || args[0] != "dispatch" {
		fmt.Fprintln(a.Stderr, "uso: atlana hook dispatch --agent <id> --event <evento> --dir <diretório>")
		return 2
	}
	fs := flag.NewFlagSet("hook dispatch", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	agent := fs.String("agent", "", "agente que chamou")
	event := fs.String("event", "", "evento canônico (pre_tool…)")
	dir := fs.String("dir", "", "diretório dos hooks instalados")
	if err := fs.Parse(args[1:]); err != nil || *agent == "" || *event == "" || *dir == "" {
		fmt.Fprintln(a.Stderr, "uso: atlana hook dispatch --agent <id> --event <evento> --dir <diretório>")
		return 2
	}
	ad, err := adapter.Get(*agent)
	if err != nil {
		fmt.Fprintln(a.Stderr, "atlana hook:", err)
		return 0 // nunca travar o agente por erro nosso
	}
	ha, ok := ad.(adapter.HooksAdapter)
	if !ok {
		fmt.Fprintln(a.Stderr, "atlana hook: o agente não tem hooks:", *agent)
		return 0
	}
	payload := map[string]any{}
	if raw, err := io.ReadAll(a.Stdin); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			fmt.Fprintln(a.Stderr, "atlana hook: payload inválido:", err)
		}
	}
	ev := hookrun.Normalize(*agent, *event, payload, ha.ToolCategory)
	return hookrun.Dispatch(*dir, ev, a.Stderr)
}

// resolvePack devolve o diretório local do pack e a origem para o lock ("" = local).
// Ordem: origem github:…, caminho local, nome no registry.
func (a *App) resolvePack(ref string) (string, string, error) {
	if source.IsOrigin(ref) {
		o, err := source.Parse(ref)
		if err != nil {
			return "", "", fmt.Errorf("%w: %v", ErrUsage, err)
		}
		return a.fetch(o)
	}
	dir := ref
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.Env.WorkDir, dir)
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return dir, "", nil
	}
	reg, err := registry.Load(a.registryPath())
	if err != nil {
		return "", "", err
	}
	o, ok, err := reg.Resolve(ref)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if !ok {
		return "", "", fmt.Errorf("%q não é um diretório, uma origem github: nem um nome do registry (veja `atlana registry list`)", ref)
	}
	return a.fetch(o)
}

func (a *App) fetch(o source.Origin) (string, string, error) {
	f := a.fetcher()
	fmt.Fprintf(a.Stdout, "Buscando %s…\n", o)
	res, err := f.Fetch(o)
	if err != nil {
		return "", "", err
	}
	fmt.Fprintf(a.Stdout, "Origem: %s @ %s\n\n", o, res.Commit[:12])
	return res.Dir, res.Pinned(o), nil
}

func (a *App) fetcher() source.Fetcher {
	if a.Fetcher != nil {
		return *a.Fetcher
	}
	cache := ""
	if a.Env.Getenv != nil {
		cache = a.Env.Getenv("XDG_CACHE_HOME")
	}
	if cache == "" {
		cache = filepath.Join(a.Env.Home, ".cache")
	}
	f := source.Fetcher{CacheDir: filepath.Join(cache, "atlana", "sources")}
	if a.Env.LookPath != nil {
		if _, err := a.Env.LookPath("gh"); err == nil {
			f.UseGH = true
		}
	}
	return f
}

func (a *App) registryPath() string {
	xdg := ""
	if a.Env.Getenv != nil {
		xdg = a.Env.Getenv("XDG_CONFIG_HOME")
	}
	return registry.Path(a.Env.Home, xdg)
}

func (a *App) registry(args []string) error {
	const use = "use `atlana registry list [--all] | add <nome> <origem> [--private] | remove <nome>`"
	if len(args) == 0 {
		return fmt.Errorf("%w: %s", ErrUsage, use)
	}
	path := a.registryPath()
	reg, err := registry.Load(path)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("registry list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		all := fs.Bool("all", false, "mostra também os privados sem acesso")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() > 0 {
			return fmt.Errorf("%w: %s", ErrUsage, use)
		}
		if len(reg.Projects) == 0 {
			fmt.Fprintf(a.Stdout, "Registry vazio (%s). Adicione com `atlana registry add <nome> github:owner/repo`.\n", path)
			return nil
		}
		f := a.fetcher()
		tw := tabwriter.NewWriter(a.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "NOME\tORIGEM\tVISIBILIDADE\tACESSO")
		hidden := 0
		for _, p := range reg.Projects {
			access := "-"
			if p.Visibility == "private" {
				o, _ := source.Parse(p.Origin)
				if err := f.Access(o); err != nil {
					if !*all {
						hidden++
						continue
					}
					access = "sem acesso"
				} else {
					access = "ok"
				}
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, p.Origin, p.Visibility, access)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if hidden > 0 {
			fmt.Fprintf(a.Stdout, "\n%d projeto(s) privado(s) escondido(s): sem acesso com o seu gh/SSH (veja com --all).\n", hidden)
		}
		return nil
	case "add":
		fs := flag.NewFlagSet("registry add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		private := fs.Bool("private", false, "repo privado (só aparece com acesso)")
		pos, err := parseInterspersed(fs, args[1:])
		if err != nil || len(pos) != 2 {
			return fmt.Errorf("%w: %s", ErrUsage, use)
		}
		vis := "public"
		if *private {
			vis = "private"
		}
		if err := reg.Add(registry.Project{Name: pos[0], Origin: pos[1], Visibility: vis}); err != nil {
			return fmt.Errorf("%w: %v", ErrUsage, err)
		}
		if err := reg.Save(path); err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "%s → %s (%s) salvo em %s\n", pos[0], pos[1], vis, path)
		return nil
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("%w: %s", ErrUsage, use)
		}
		if !reg.Remove(args[1]) {
			return fmt.Errorf("%q não está no registry", args[1])
		}
		if err := reg.Save(path); err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "%s removido do registry\n", args[1])
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUsage, use)
}

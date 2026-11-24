// Package adapter traduz o formato Atlana para os caminhos e formatos de cada agente.
//
// Adicionar um agente novo = criar um arquivo com um Adapter e registrá-lo em All().
package adapter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// Scope define onde instalar: no usuário (~) ou no projeto atual.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// ParseScope converte texto em Scope.
func ParseScope(s string) (Scope, error) {
	switch Scope(s) {
	case ScopeUser, ScopeProject:
		return Scope(s), nil
	}
	return "", fmt.Errorf("escopo inválido %q (use user ou project)", s)
}

// Env isola tudo que vem do sistema, para que os adaptadores sejam testáveis com um HOME falso.
type Env struct {
	Home     string
	WorkDir  string
	LookPath func(string) (string, error)
	Getenv   func(string) string
}

// SystemEnv monta o Env a partir do processo atual.
func SystemEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return Env{}, err
	}
	return Env{Home: home, WorkDir: wd, LookPath: exec.LookPath, Getenv: os.Getenv}, nil
}

// Detection é o resultado de Detect: se o agente parece instalado e por quê.
type Detection struct {
	Installed bool
	Evidence  []string
}

// Adapter é o contrato de cada agente.
type Adapter interface {
	// ID é o identificador usado em --agent (ex.: "cursor").
	ID() string
	// Name é o nome para humanos.
	Name() string
	// Detect verifica binários e diretórios de configuração, sem efeitos colaterais.
	Detect(env Env) Detection
	// Supports diz se este adaptador já sabe instalar o módulo.
	Supports(m manifest.Module) bool
	// SkillsRoot devolve o diretório onde as skills ficam para o escopo.
	SkillsRoot(scope Scope, env Env) (string, error)
}

// All devolve todos os adaptadores conhecidos, ordenados por ID.
func All() []Adapter {
	list := []Adapter{cursor{}, claude{}, codex{}, gemini{}}
	sort.Slice(list, func(i, j int) bool { return list[i].ID() < list[j].ID() })
	return list
}

// Get busca um adaptador por ID.
func Get(id string) (Adapter, error) {
	ids := make([]string, 0, 4)
	for _, a := range All() {
		if a.ID() == id {
			return a, nil
		}
		ids = append(ids, a.ID())
	}
	return nil, fmt.Errorf("agente desconhecido %q (conhecidos: %s)", id, strings.Join(ids, ", "))
}

// detectBy procura binários no PATH e diretórios de configuração.
func detectBy(env Env, bins []string, dirs []string) Detection {
	var d Detection
	if env.LookPath != nil {
		for _, b := range bins {
			if p, err := env.LookPath(b); err == nil {
				d.Evidence = append(d.Evidence, "binário "+p)
			}
		}
	}
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			d.Evidence = append(d.Evidence, "diretório "+dir)
		}
	}
	d.Installed = len(d.Evidence) > 0
	return d
}

// scopedDir resolve <home>/<sub> ou <workdir>/<sub>.
func scopedDir(scope Scope, env Env, sub ...string) (string, error) {
	var base string
	switch scope {
	case ScopeUser:
		base = env.Home
	case ScopeProject:
		base = env.WorkDir
	default:
		return "", fmt.Errorf("escopo inválido %q", scope)
	}
	if base == "" {
		return "", fmt.Errorf("diretório base vazio para o escopo %s", scope)
	}
	return filepath.Join(append([]string{base}, sub...)...), nil
}

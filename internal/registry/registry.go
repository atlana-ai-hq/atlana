// Package registry é o índice local do Hub (hub.yaml): nomes de projetos Atlana
// e as origens github:owner/repo de cada um. Não guarda tokens nem conteúdo:
// repos privados só ficam visíveis quando a autenticação da pessoa alcança.
package registry

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
	"github.com/atlana-ai-hq/atlana/internal/source"
	"github.com/atlana-ai-hq/atlana/spec"
)

// FileName é o nome do arquivo do registry.
const FileName = "hub.yaml"

// Kind do documento.
const Kind = "HubRegistry"

// Project é uma entrada do registry.
type Project struct {
	Name       string `yaml:"name"`
	Origin     string `yaml:"origin"`
	Visibility string `yaml:"visibility"`
}

// Registry é o conteúdo de hub.yaml.
type Registry struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Projects   []Project `yaml:"projects"`
}

// Path devolve $XDG_CONFIG_HOME/atlana/hub.yaml (padrão ~/.config/atlana/hub.yaml).
func Path(home, xdgConfigHome string) string {
	base := xdgConfigHome
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "atlana", FileName)
}

// Load lê o registry; se não existir, devolve um vazio.
func Load(path string) (*Registry, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{APIVersion: spec.APIVersion, Kind: Kind}, nil
	}
	if err != nil {
		return nil, err
	}
	r := &Registry{}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(r); err != nil {
		return nil, fmt.Errorf("%s inválido: %w", path, err)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// Validate confere apiVersion, kind e cada projeto.
func (r *Registry) Validate() error {
	if r.APIVersion != spec.APIVersion {
		return fmt.Errorf("apiVersion %q não suportada (esperado %s)", r.APIVersion, spec.APIVersion)
	}
	if r.Kind != Kind {
		return fmt.Errorf("kind %q inválido (esperado %s)", r.Kind, Kind)
	}
	seen := map[string]bool{}
	for i, p := range r.Projects {
		if !manifest.ValidName(p.Name) {
			return fmt.Errorf("projects[%d]: nome %q inválido (minúsculas, números e hífens)", i, p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("projects[%d]: nome %q repetido", i, p.Name)
		}
		seen[p.Name] = true
		o, err := source.Parse(p.Origin)
		if err != nil {
			return fmt.Errorf("projects[%d] (%s): %w", i, p.Name, err)
		}
		if o.Path != "" || o.Ref != "" {
			return fmt.Errorf("projects[%d] (%s): a origem do registry é só o repo (sem //caminho nem @ref)", i, p.Name)
		}
		if p.Visibility != "public" && p.Visibility != "private" {
			return fmt.Errorf("projects[%d] (%s): visibility precisa ser public ou private", i, p.Name)
		}
	}
	return nil
}

// Get busca um projeto pelo nome.
func (r *Registry) Get(name string) (Project, bool) {
	for _, p := range r.Projects {
		if p.Name == name {
			return p, true
		}
	}
	return Project{}, false
}

// Add insere ou substitui um projeto.
func (r *Registry) Add(p Project) error {
	if p.Visibility == "" {
		p.Visibility = "public"
	}
	next := *r
	next.Projects = nil
	for _, x := range r.Projects {
		if x.Name != p.Name {
			next.Projects = append(next.Projects, x)
		}
	}
	next.Projects = append(next.Projects, p)
	sort.Slice(next.Projects, func(i, j int) bool { return next.Projects[i].Name < next.Projects[j].Name })
	if err := next.Validate(); err != nil {
		return err
	}
	*r = next
	return nil
}

// Remove tira um projeto; devolve false se ele não existia.
func (r *Registry) Remove(name string) bool {
	out := r.Projects[:0]
	found := false
	for _, p := range r.Projects {
		if p.Name == name {
			found = true
			continue
		}
		out = append(out, p)
	}
	r.Projects = out
	return found
}

// Save grava de forma atômica.
func (r *Registry) Save(path string) error {
	if err := r.Validate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# Registry local do Atlana Hub. Sem tokens: repos privados usam o seu gh ou SSH.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(r); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hub.yaml-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Resolve traduz "<nome>[//caminho][@ref]" para uma origem, se o nome está no registry.
func (r *Registry) Resolve(ref string) (source.Origin, bool, error) {
	name, rest := ref, ""
	for i := 0; i < len(ref); i++ {
		if ref[i] == '/' || ref[i] == '@' {
			name, rest = ref[:i], ref[i:]
			break
		}
	}
	p, ok := r.Get(name)
	if !ok {
		return source.Origin{}, false, nil
	}
	o, err := source.Parse(p.Origin + rest)
	return o, true, err
}

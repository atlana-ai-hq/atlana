// Package lockfile guarda o que o atlana instalou, onde e com qual hash.
// É o que torna status e uninstall confiáveis: só removemos o que é nosso e não foi editado.
package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// FileName é o nome do lockfile.
const FileName = "atlana.lock"

// SchemaVersion é a versão do formato do lockfile.
const SchemaVersion = 1

// File é um arquivo gerado pelo atlana.
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Entry registra um pack instalado num agente para um módulo.
type Entry struct {
	Pack        string `json:"pack"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	Agent       string `json:"agent"`
	Module      string `json:"module"`
	Root        string `json:"root"`
	InstalledAt string `json:"installedAt"`
	Files       []File `json:"files"`
}

// Key identifica uma entrada.
func (e Entry) Key() string { return e.Pack + "|" + e.Agent + "|" + e.Module }

// Lock é o conteúdo de atlana.lock.
type Lock struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Path devolve o lockfile de usuário: $XDG_CONFIG_HOME/atlana/atlana.lock ou ~/.config/atlana/atlana.lock.
func UserPath(home, xdgConfigHome string) string {
	base := xdgConfigHome
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "atlana", FileName)
}

// ProjectPath devolve o lockfile do projeto.
func ProjectPath(workDir string) string { return filepath.Join(workDir, FileName) }

// Load lê o lockfile; se não existir, devolve um lock vazio.
func Load(path string) (*Lock, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Lock{Version: SchemaVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	l := &Lock{}
	if err := json.Unmarshal(raw, l); err != nil {
		return nil, fmt.Errorf("%s corrompido: %w", path, err)
	}
	if l.Version != SchemaVersion {
		return nil, fmt.Errorf("%s: versão de lockfile %d não suportada (esperado %d)", path, l.Version, SchemaVersion)
	}
	return l, nil
}

// Find devolve a entrada com a chave, se existir.
func (l *Lock) Find(pack, agent, module string) (Entry, bool) {
	for _, e := range l.Entries {
		if e.Pack == pack && e.Agent == agent && e.Module == module {
			return e, true
		}
	}
	return Entry{}, false
}

// Put insere ou substitui a entrada.
func (l *Lock) Put(e Entry) {
	for i := range l.Entries {
		if l.Entries[i].Key() == e.Key() {
			l.Entries[i] = e
			return
		}
	}
	l.Entries = append(l.Entries, e)
}

// Delete remove a entrada.
func (l *Lock) Delete(pack, agent, module string) {
	out := l.Entries[:0]
	for _, e := range l.Entries {
		if !(e.Pack == pack && e.Agent == agent && e.Module == module) {
			out = append(out, e)
		}
	}
	l.Entries = out
}

// Owners conta quantas entradas listam o caminho.
func (l *Lock) Owners(path string) []string {
	var keys []string
	for _, e := range l.Entries {
		for _, f := range e.Files {
			if f.Path == path {
				keys = append(keys, e.Key())
			}
		}
	}
	return keys
}

// Save grava o lockfile de forma atômica e ordenada (diffs estáveis).
func (l *Lock) Save(path string) error {
	l.Version = SchemaVersion
	sort.Slice(l.Entries, func(i, j int) bool { return l.Entries[i].Key() < l.Entries[j].Key() })
	for i := range l.Entries {
		files := l.Entries[i].Files
		sort.Slice(files, func(a, b int) bool { return files[a].Path < files[b].Path })
	}
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".atlana.lock-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

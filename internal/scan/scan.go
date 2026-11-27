// Package scan procura, antes de publicar, o que não pode ir para um repositório
// público: nomes de uma lista local (empregador, projetos internos) e packs
// marcados como privados. Segredos ficam com o gitleaks no CI.
//
// A lista de nomes nunca fica no repo: vem de ~/.config/atlana/private-names.txt
// ou de ATLANA_PRIVATE_NAMES (vírgulas), que no CI é um secret.
package scan

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/atlana-ai-hq/atlana/internal/manifest"
)

// NamesFile é o arquivo local com um nome por linha (# comenta).
const NamesFile = "private-names.txt"

// EnvVar é a variável com nomes separados por vírgula.
const EnvVar = "ATLANA_PRIVATE_NAMES"

// Finding é um achado. Index é a posição do nome na lista (para relatório sem expor o nome).
type Finding struct {
	File  string
	Line  int
	Index int
	Name  string
	Kind  string // "nome" ou "pack-privado"
}

// LoadNames junta os nomes do arquivo local (se existir) e da variável.
func LoadNames(configDir string, getenv func(string) string) ([]string, error) {
	var names []string
	f, err := os.Open(filepath.Join(configDir, NamesFile))
	switch {
	case err == nil:
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if l := strings.TrimSpace(sc.Text()); l != "" && !strings.HasPrefix(l, "#") {
				names = append(names, l)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	if getenv != nil {
		for _, n := range strings.Split(getenv(EnvVar), ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
	}
	return names, nil
}

var skipDirs = map[string]bool{".git": true, "dist": true, "node_modules": true, "bin": true}

// Scan percorre root e devolve os achados, em ordem de arquivo e linha.
func Scan(root string, names []string) ([]Finding, error) {
	res := make([]*regexp.Regexp, len(names))
	for i, n := range names {
		res[i] = regexp.MustCompile(`(?i)(^|[^\pL\pN_])` + regexp.QuoteMeta(n) + `($|[^\pL\pN_])`)
	}
	var out []Finding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrPermission) && path != root {
			return filepath.SkipDir // pasta sem permissão de leitura: não é conteúdo do repo
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.Name() == manifest.FileName {
			if p, _, err := manifest.Load(filepath.Dir(path)); err == nil && p.Visibility == "private" {
				out = append(out, Finding{File: rel, Line: 1, Index: -1, Kind: "pack-privado"})
			}
		}
		if len(res) == 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 2<<20 {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
			return nil // binário
		}
		for ln, line := range strings.Split(string(b), "\n") {
			for i, re := range res {
				if re.MatchString(line) {
					out = append(out, Finding{File: rel, Line: ln + 1, Index: i, Name: names[i], Kind: "nome"})
				}
			}
		}
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, err
}

package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Marker identifica um diretório gerado pelo atlana build. Só diretórios com
// ele podem ser substituídos; qualquer outro conteúdo é da pessoa.
const Marker = ".atlana-build.json"

// Dir é onde o artefato fica: <out>/<target>/<nome>.
func Dir(out string, r *Result) string { return filepath.Join(out, string(r.Target), r.Name) }

// Write grava o artefato de forma atômica (pasta temporária + rename).
func Write(r *Result, out, version string) (string, error) {
	dst := Dir(out, r)
	if _, err := os.Stat(dst); err == nil {
		if _, err := os.Stat(filepath.Join(dst, Marker)); err != nil {
			return "", fmt.Errorf("%s já existe e não foi gerado pelo atlana build; apague ou use outro --out", dst)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dst), "."+r.Name+"-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	for _, f := range r.Files {
		if strings.Contains(f.Path, "..") || filepath.IsAbs(f.Path) {
			return "", fmt.Errorf("caminho inválido no artefato: %s", f.Path)
		}
		path := filepath.Join(tmp, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if f.Source != "" {
			err = copyFile(f.Source, path)
		} else {
			err = os.WriteFile(path, f.Content, 0o644)
		}
		if err != nil {
			return "", err
		}
	}
	marker, _ := json.MarshalIndent(map[string]string{"pack": r.Name, "target": string(r.Target), "atlana": version}, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, Marker), append(marker, '\n'), 0o644); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dst); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

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
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

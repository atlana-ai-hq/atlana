// Package source resolve origens remotas de packs (github:owner/repo//path@ref)
// para um diretório local, usando o git da própria pessoa.
//
// A autenticação é a que ela já tem: credenciais do git, `gh` (como credential
// helper, se estiver logado) ou SSH. O atlana nunca lê nem grava tokens.
package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Origin é uma origem remota parseada.
type Origin struct {
	Owner, Repo string
	Path        string // subdiretório dentro do repo ("" = raiz)
	Ref         string // branch, tag ou commit ("" = branch padrão)
}

var (
	originRE = regexp.MustCompile(`^github:([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(//([^@]+))?(@(.+))?$`)
	refRE    = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
)

// IsOrigin diz se s parece uma origem remota (e não um caminho local).
func IsOrigin(s string) bool { return strings.HasPrefix(s, "github:") }

// Parse lê "github:owner/repo[//path][@ref]".
func Parse(s string) (Origin, error) {
	m := originRE.FindStringSubmatch(s)
	if m == nil {
		return Origin{}, fmt.Errorf("origem inválida %q (use github:owner/repo//caminho@ref)", s)
	}
	o := Origin{Owner: m[1], Repo: strings.TrimSuffix(m[2], ".git"), Path: strings.Trim(m[4], "/"), Ref: m[6]}
	if o.Path != "" {
		clean := filepath.ToSlash(filepath.Clean(o.Path))
		if clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(o.Path) {
			return Origin{}, fmt.Errorf("origem %q: o caminho precisa ficar dentro do repo", s)
		}
		o.Path = clean
	}
	if o.Ref != "" && (!refRE.MatchString(o.Ref) || strings.HasPrefix(o.Ref, "-") || strings.Contains(o.Ref, "..")) {
		return Origin{}, fmt.Errorf("origem %q: ref inválida", s)
	}
	return o, nil
}

// RepoID devolve "github:owner/repo", sem caminho nem ref.
func (o Origin) RepoID() string { return "github:" + o.Owner + "/" + o.Repo }

// String devolve a forma canônica.
func (o Origin) String() string {
	s := o.RepoID()
	if o.Path != "" {
		s += "//" + o.Path
	}
	if o.Ref != "" {
		s += "@" + o.Ref
	}
	return s
}

// Fetcher busca repos com o git.
type Fetcher struct {
	CacheDir string
	// URLs devolve as URLs a tentar, em ordem (padrão: HTTPS e depois SSH do GitHub).
	URLs func(owner, repo string) []string
	// UseGH liga o `gh auth git-credential` como credential helper.
	UseGH   bool
	Timeout time.Duration
}

// DefaultURLs tenta HTTPS (público, credential helper ou gh) e depois SSH.
func DefaultURLs(owner, repo string) []string {
	return []string{
		fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		fmt.Sprintf("git@github.com:%s/%s.git", owner, repo),
	}
}

// Result é um pack resolvido.
type Result struct {
	Dir    string // diretório local do pack (repo + Path)
	Commit string // commit exato usado
}

// Pinned devolve a origem com o commit exato, para o lock.
func (r Result) Pinned(o Origin) string {
	o.Ref = r.Commit
	return o.String()
}

func (f Fetcher) urls(o Origin) []string {
	if f.URLs != nil {
		return f.URLs(o.Owner, o.Repo)
	}
	return DefaultURLs(o.Owner, o.Repo)
}

// git roda um comando git sem nunca pedir senha no terminal.
func (f Fetcher) git(dir string, args ...string) (string, error) {
	timeout := f.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var full []string
	if f.UseGH {
		// Soma o gh aos helpers que a pessoa já tem (não substitui).
		full = append(full, "-c", "credential.https://github.com.helper=!gh auth git-credential")
	}
	if dir != "" {
		full = append(full, "-C", dir)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if ctx.Err() != nil {
			msg = "tempo esgotado"
		}
		return "", fmt.Errorf("git %s: %s", args[0], firstLine(msg))
	}
	return strings.TrimSpace(out.String()), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Access diz se o repo está acessível com a autenticação atual (git ls-remote).
func (f Fetcher) Access(o Origin) error {
	var errs []string
	for _, u := range f.urls(o) {
		if _, err := f.git("", "ls-remote", "--heads", "--", u); err == nil {
			return nil
		} else {
			errs = append(errs, err.Error())
		}
	}
	return fmt.Errorf("%s sem acesso: %s", o.RepoID(), strings.Join(errs, "; "))
}

// Fetch clona (ou atualiza) o repo no cache e faz checkout do ref pedido.
func (f Fetcher) Fetch(o Origin) (Result, error) {
	if f.CacheDir == "" {
		return Result{}, errors.New("diretório de cache vazio")
	}
	dir := filepath.Join(f.CacheDir, "github", o.Owner, o.Repo)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return Result{}, err
		}
		var errs []string
		cloned := false
		for _, u := range f.urls(o) {
			os.RemoveAll(dir)
			if _, err := f.git("", "clone", "--quiet", "--no-checkout", "--", u, dir); err == nil {
				cloned = true
				break
			} else {
				errs = append(errs, err.Error())
			}
		}
		if !cloned {
			os.RemoveAll(dir)
			return Result{}, fmt.Errorf("não deu para clonar %s (para repos privados, faça login com `gh auth login` ou configure SSH): %s", o.RepoID(), strings.Join(errs, "; "))
		}
	} else if _, err := f.git(dir, "fetch", "--quiet", "--tags", "--force", "origin"); err != nil {
		return Result{}, fmt.Errorf("não deu para atualizar %s: %w", o.RepoID(), err)
	}

	commit, err := f.resolve(dir, o.Ref)
	if err != nil {
		return Result{}, err
	}
	if _, err := f.git(dir, "checkout", "--quiet", "--force", "--detach", commit); err != nil {
		return Result{}, err
	}
	pack := dir
	if o.Path != "" {
		pack = filepath.Join(dir, filepath.FromSlash(o.Path))
	}
	if st, err := os.Stat(pack); err != nil || !st.IsDir() {
		return Result{}, fmt.Errorf("%s: o caminho %q não existe no commit %s", o.RepoID(), o.Path, commit[:12])
	}
	return Result{Dir: pack, Commit: commit}, nil
}

// resolve acha o commit de um ref: branch remota, tag ou commit; vazio = branch padrão.
func (f Fetcher) resolve(dir, ref string) (string, error) {
	candidates := []string{"origin/HEAD"}
	if ref != "" {
		candidates = []string{"origin/" + ref, "refs/tags/" + ref, ref}
	}
	for _, c := range candidates {
		if sha, err := f.git(dir, "rev-parse", "--verify", "--quiet", c+"^{commit}"); err == nil && sha != "" {
			return sha, nil
		}
	}
	if ref == "" {
		return "", errors.New("não achei a branch padrão do repo")
	}
	return "", fmt.Errorf("ref %q não existe no repo", ref)
}

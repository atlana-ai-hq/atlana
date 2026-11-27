# Release e publicação

## Checklist antes de publicar

```bash
gofmt -l .                      # precisa sair vazio
go vet ./...
go test -race ./...
go run ./cmd/atlana validate packs
go run ./cmd/atlana validate templates
go run ./cmd/atlana scan .      # nomes privados e packs private
```

O CI também roda o **gitleaks** para segredos.

## Nomes privados

`atlana scan` procura nomes que não podem ir para um repositório público (empregador, projetos internos, clientes). A lista **nunca** fica no repo:

- **Local:** `~/.config/atlana/private-names.txt`, um nome por linha (`#` comenta).
- **CI:** o secret `ATLANA_PRIVATE_NAMES`, com nomes separados por vírgula.

O relatório mostra só `arquivo:linha` e o número do nome na lista, porque log de CI de repo público também é público. Use `--show` só localmente.

Para ligar no CI, adicione um passo ao `.github/workflows/ci.yml`:

```yaml
      - name: atlana scan
        env:
          ATLANA_PRIVATE_NAMES: ${{ secrets.ATLANA_PRIVATE_NAMES }}
        run: go run ./cmd/atlana scan .
```

## Binários e Homebrew

A configuração está em [`.goreleaser.yaml`](../.goreleaser.yaml):

- **Builds:** macOS, Linux e Windows (amd64 e arm64), sem CGO. A versão vai para `atlana version` via `-ldflags`.
- **Arquivos:** `.tar.gz` (`.zip` no Windows), com `checksums.txt`.
- **Homebrew:** cask no tap `atlana-ai-hq/homebrew-tap`.

Antes do primeiro release:

1. Crie o repositório `atlana-ai-hq/homebrew-tap`.
2. Crie um token com escrita nesse repositório e guarde como secret `HOMEBREW_TAP_GITHUB_TOKEN`.
3. **macOS:** assine e notarize o binário (seção `notarize` do GoReleaser). Sem isso, o Gatekeeper bloqueia o binário baixado. Não contorne removendo a quarentena.
4. Teste local, sem publicar: `goreleaser release --snapshot --clean`.

Um workflow de release que dispara em tags `v*` pode rodar `goreleaser/goreleaser-action` com `args: release --clean`, passando `GITHUB_TOKEN` e `HOMEBREW_TAP_GITHUB_TOKEN`.

## Versões

- Tag `vX.Y.Z` no `main`, em SemVer. Mensagens em Conventional Commits geram o changelog.
- `go install github.com/atlana-ai-hq/atlana/cmd/atlana@vX.Y.Z` continua funcionando sem release.

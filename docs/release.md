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

O CI (`.github/workflows/ci.yml`) e o release rodam `atlana scan .` com esse secret.

## Binários e Homebrew

A configuração está em [`.goreleaser.yaml`](../.goreleaser.yaml):

- **Builds:** macOS, Linux e Windows (amd64 e arm64), sem CGO. A versão vai para `atlana version` via `-ldflags`.
- **Arquivos:** `.tar.gz` (`.zip` no Windows), com `checksums.txt`.
- **Homebrew:** cask no tap `atlana-ai-hq/homebrew-tap`.

O workflow `.github/workflows/release.yml` roda em cada tag `v*`: `atlana scan` e depois `goreleaser release --clean`. O CI roda `goreleaser check` em todo push.

### Secrets

| Secret | Para quê | Sem ele |
|---|---|---|
| `GITHUB_TOKEN` | publicar o release (automático) | n/a |
| `ATLANA_PRIVATE_NAMES` | lista do `atlana scan` | o scan só checa packs `private` |
| `HOMEBREW_TAP_GITHUB_TOKEN` | publicar o cask no `atlana-ai-hq/homebrew-tap` (token com escrita só nesse repo) | o cask não é publicado |
| `MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD` | certificado Developer ID Application (`.p12` em base64) e a senha | o binário de macOS sai sem assinatura |
| `MACOS_NOTARY_ISSUER_ID`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_KEY` | chave da App Store Connect API (`.p8` em base64) para notarizar | sem notarização |

Sem assinatura e notarização, o Gatekeeper bloqueia o binário baixado no macOS. Não contorne removendo a quarentena: ligue os secrets da Apple antes de divulgar o `brew install`. Linux, Windows e `go install` não dependem disso.

### Passo a passo

1. Teste local, sem publicar: `goreleaser release --snapshot --clean`.
2. Crie o repositório `atlana-ai-hq/homebrew-tap` e o secret `HOMEBREW_TAP_GITHUB_TOKEN`.
3. Configure os secrets da Apple (precisa de conta no Apple Developer Program).
4. `git tag -a v0.1.0 -m "v0.1.0" && git push origin v0.1.0`.
5. Confira o release no GitHub e, com o tap, `brew install --cask atlana-ai-hq/tap/atlana`.

## Versões

- Tag `vX.Y.Z` no `main`, em SemVer. Mensagens em Conventional Commits geram o changelog.
- `go install github.com/atlana-ai-hq/atlana/cmd/atlana@vX.Y.Z` continua funcionando sem release.

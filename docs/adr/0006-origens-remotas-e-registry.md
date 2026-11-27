# ADR 0006: Origens remotas pelo git da pessoa e registry local

- Status: aceita
- Data: 2026-11-27

## Contexto

O F10 do PRD (registry, primeira fatia do Hub) pede instalar packs de `github:owner/repo//caminho`, inclusive de repos privados (dotfiles, packs pessoais), sem serviço hospedado e sem token no repositório. O PRD do Hub pede ainda um registry local que só mostre os privados quando a pessoa tem acesso.

## Decisão

- **O git da pessoa é o transporte.** `internal/source` roda o binário `git`: HTTPS primeiro, depois SSH. Se o `gh` está no PATH, o `gh auth git-credential` é **somado** aos credential helpers existentes (não os substitui). `GIT_TERMINAL_PROMPT=0` e `ssh -o BatchMode=yes` garantem que a CLI nunca fica presa pedindo senha.
- **Cache, não cópia solta.** Cada repo é clonado uma vez em `~/.cache/atlana/sources/github/<owner>/<repo>`, atualizado com `fetch` e posto no commit pedido (`checkout --detach`). O lock grava a origem com o commit exato, para `status` e auditoria saberem de onde veio cada arquivo.
- **O cache é a única escrita antes do plano.** Buscar a origem escreve no cache mesmo com `--dry-run`, porque o pack precisa ser lido para virar plano. As pastas dos agentes continuam intocadas até o `Apply`.
- **Registry = `hub.yaml` local** (`kind: HubRegistry`): nome, origem (só o repo) e `visibility`. O `list` testa os privados com `git ls-remote` e esconde os inacessíveis. Campos desconhecidos são erro, então não há onde guardar um token.
- **Resolução no `install`:** origem `github:` → caminho local → nome do registry.

## Alternativas

- **API do GitHub e tarball:** evita depender do binário `git`, mas exige token próprio para repos privados, contrariando o "sem token".
- **Biblioteca git em Go (go-git):** seria uma dependência grande, e mesmo assim precisaria reproduzir a autenticação do `gh` e do SSH que a pessoa já tem configurada.

## Consequências

- O `git` precisa estar instalado para origens remotas (caminhos locais continuam funcionando sem ele).
- `${PLUGIN_ROOT}` de servidores MCP de packs remotos aponta para o cache. Atualizar o cache (outro `install` do mesmo repo em outro commit) muda os arquivos que o servidor usa; reinstalar o pack deixa tudo consistente de novo.
- Só GitHub por enquanto. Outros hosts entram como outro prefixo (`gitlab:`…) no mesmo `Fetcher`.

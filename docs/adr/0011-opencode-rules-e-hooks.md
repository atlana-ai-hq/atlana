# ADR 0011: rules e hooks no OpenCode

- Status: aceita
- Data: 2026-11-27
- Complementa as ADRs 0003, 0005 e 0010

## Contexto

O OpenCode não tem pasta de rules lida sozinha (só `AGENTS.md`, que o Atlana não edita, pela ADR 0003) nem hooks de shell: hooks são plugins JS. Conferido no binário 1.18.31:

- **`instructions`:** um item absoluto expande o glob **só no último trecho** (`glob(basename, {cwd: dirname})`), então `…/atlana-pack/**/*.md` não carrega nada. Um item relativo é procurado a partir do diretório atual, subindo até a raiz do projeto.
- **Plugins:** os de `~/.config/opencode/plugins/` (e `.opencode/plugins/`) carregam ao iniciar uma sessão. A 1.18 usa a API V1: `tool.execute.before(input, output)`, com `input = {tool, sessionID, callID}` e `output = {args}`. Um `throw` nega a ferramenta.
- **Argumentos:** `bash` recebe `command`; `edit` e `write` recebem `filePath`.

## Decisão

- **Rules:** cada rule vira um arquivo **plano** em `atlana/atlana-<pack>/`. `docs/x.md` vira `docs-x.md`, e um nome repetido é erro.
  - Entra **um item por pack** em `instructions`: o caminho absoluto `…/atlana-<pack>/*.md` no escopo user, ou `.opencode/atlana/atlana-<pack>/*.md` no project, para o repo continuar portátil.
  - O item tem dono no lock (as `keys`, como na ADR 0004). O merge (`configmerge.ParseStringList`) mexe só nesse item; o resto da lista e da config fica como estava.
- **Hooks:** um plugin gerado por pack (`plugins/atlana-<pack>.js`), registrado como arquivo do pack no lock. Ele chama `atlana hook dispatch --agent opencode --event … --dir … --pack <pack>`, e o `--pack` faz o dispatcher rodar só os hooks daquele pack.
  - Os scripts dos hooks continuam no diretório de hooks do Atlana (ADR 0005).
  - **Eventos:** `pre_tool` (exit 2 nega), `post_tool`, `session_start` (`session.created`) e `stop` (`session.idle`).
  - `prompt_submit` fica de fora, porque o formato de `chat.message` não foi verificado.
- **Ferramentas:** `bash` → shell; `edit`, `multiedit`, `patch` e `apply_patch` → edit; `write` → write; `read`, `glob`, `grep` e `list` → read.
  - Nomes com `_` que não são nativos (`<servidor>_<ferramenta>`) → mcp.

## Consequências

- O plugin depende do `atlana` no caminho gravado. Reinstalar atualiza o plugin, e o OpenCode precisa ser reiniciado para carregar a mudança.
- `apply_patch` não traz `filePath`, então hooks com `match.path` não casam com ele, só com `match.tool`.
- **Verificado:**
  - O OpenCode real carrega os plugins gerados.
  - Chamado como o OpenCode chama, o plugin nega `git push --force` e edições em `.plan/`, e libera o resto.
- **Não verificado:**
  - Uma chamada de ferramenta de ponta a ponta numa sessão com modelo (exigiria uma chamada paga ao provedor).
  - A leitura das rules no prompt: o `debug` do OpenCode não mostra as `instructions` carregadas.

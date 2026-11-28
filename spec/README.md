# Especificação Atlana v1

> `apiVersion: atlana/v1` · status: **estável**. Mudanças incompatíveis só numa `atlana/v2`. `atlana/v1alpha1` (mesmo formato) ainda é aceita, com aviso ([ADR 0008](../docs/adr/0008-apiversion-v1.md)).

O formato Atlana é uma camada **fina** por cima de padrões abertos. A regra principal é: **não inventar formato onde já existe padrão**.

| Conteúdo | Padrão usado | Status na v0.1 |
|---|---|---|
| Skills | [Agent Skills](https://agentskills.io/specification) (`SKILL.md`), sem mudanças | validado e instalado (Cursor, Claude Code, Codex, Gemini CLI, Antigravity; exportado para o Grok Bot) |
| Rules | `AGENTS.md` e outros `.md` (fonte), em arquivos próprios por agente | validado e instalado (Claude Code, Cursor, Gemini CLI, Codex, OpenCode, Antigravity) |
| MCP | `mcp.json` no formato de Agent Plugins (`mcpServers`) | validado (chaves e segredos) e instalado por merge (Claude Code, Cursor, Gemini CLI, Codex, OpenCode, Antigravity) |
| Subagents | formato Atlana (`agents/<nome>.md`) | validado e instalado (Claude Code, Gemini CLI, OpenCode) |
| Hooks | formato Atlana (`hooks/<nome>/hook.yaml`) | validado e instalado (Claude Code, Gemini CLI, OpenCode); fallback nos outros |
| Commands | formato Atlana (`commands/<nome>.md`) | validado e instalado (Claude Code, Gemini CLI, Cursor, OpenCode, Antigravity) |

## 1. Pack

A unidade de distribuição é o **pack**: um diretório com um `atlana.yaml` na raiz.

```
packs/<nome>/
├── atlana.yaml                # manifesto (obrigatório)
├── AGENTS.md                  # rules (opcional)
├── skills/<nome>/SKILL.md     # Agent Skills
├── agents/<nome>.md           # subagents
├── hooks/<nome>/hook.yaml     # hooks + script
├── commands/<nome>.md         # slash commands
└── mcp.json                   # servidores MCP
```

## 2. `atlana.yaml`

Schema: [`schemas/atlana.schema.json`](schemas/atlana.schema.json) (JSON Schema 2020-12). A CLI embute **este mesmo arquivo**, então o schema publicado e o validado nunca divergem.

```yaml
apiVersion: atlana/v1    # obrigatório
kind: Pack                     # obrigatório
name: hello                    # obrigatório; regras de nome de Agent Skills
version: 0.1.0                 # obrigatório; SemVer 2.0
description: O que o pack faz. # obrigatório; 1-1024 caracteres
license: MIT
visibility: public             # public | private
authors: [atlana-ai-hq]

modules:                       # cada módulo é instalável sozinho (--only)
  rules: [AGENTS.md]           # caminhos relativos ao pack
  skills: [conventional-commit-message]
  agents: []
  hooks: []
  commands: []
  mcp: []                      # chaves de mcp.json → mcpServers

requires:
  packs: [outro-pack@^1]
  bins: [git]
  env: [GITHUB_TOKEN]          # só NOMES de variáveis; valores nunca

targets:
  include: []                  # vazio = todos os agentes
  exclude: [gemini]
  overrides: {}                # ajustes por agente (reservado)
```

Regras adicionais (checadas pela CLI, além do schema):

- Campos desconhecidos na raiz são erro. Use o prefixo `x-` para extensões.
- Cada item de `modules` precisa existir no pack (`skills/<n>/SKILL.md`, `agents/<n>.md`, `hooks/<n>/hook.yaml`, `commands/<n>.md`, chave em `mcp.json`).
- Caminhos de `rules` precisam ser relativos e ficar dentro do pack.
- Um diretório em `skills/` que não está em `modules.skills` gera **aviso** (não é instalado).

## 3. Skills

Seguem a spec Agent Skills sem extensões:

- `name`: obrigatório, 1-64 caracteres `a-z`, `0-9` e `-`, sem hífen no início, no fim ou duplo, e **igual ao nome do diretório**.
- `description`: obrigatório, 1-1024 caracteres.
- Opcionais: `license`, `compatibility` (≤ 500), `metadata` (mapa texto → texto), `allowed-tools` (texto).
- Outros campos geram aviso: alguns agentes aceitam extras, mas não é portável.

## 4. Hooks

```yaml
name: protect-plan-files     # igual ao diretório hooks/<nome>/
event: pre_tool              # pre_tool | post_tool | prompt_submit | session_start | stop
match:                       # só em pre_tool e post_tool; vazio = tudo
  tool: [edit, write]        # shell | edit | write | read | mcp
  path: [".plan/**"]         # globs relativos ao projeto (*, ?, **)
run: ./run.sh                # relativo ao hook, executável; recebe o JSON normalizado no stdin
timeout: 10                  # 1 a 60 segundos (padrão 30)
fallback: rule               # agente sem hooks: rule (usa rule.md ao lado) | skip (padrão) | fail
```

O script recebe no stdin `{"event","agent","tool","path","command","prompt","cwd"}`. **Exit 2 nega a ação**, e o stderr vira o motivo mostrado ao agente; qualquer outro erro ou estouro de tempo só gera aviso. O stdout é descartado.

**Hooks nunca entram em silêncio.** Um `install` sem `--only` pula os hooks com um aviso; é preciso `--only hooks`, e o plano diz o que vai rodar.

## 5. Subagents

```markdown
---
name: planner                           # igual ao nome do arquivo agents/<nome>.md
description: Roda o planejamento em contexto isolado.
tools: [read, search, shell:readonly]   # read | search | edit | write | shell | shell:readonly | web | mcp
model: inherit                          # inherit (padrão) | fast | smart
---

Prompt do subagent (obrigatório).
```

Sem `tools`, o subagent herda todas as ferramentas. Uma ferramenta sem equivalente seguro no agente é deixada de fora, com aviso. Se nenhuma sobra, o subagent não é instalado, porque no agente uma lista vazia significa "todas".

## 6. Instalação: garantias

Toda implementação da CLI precisa respeitar:

1. **Plano antes de aplicar.** `install` e `uninstall` mostram cada arquivo que vão criar, atualizar ou remover. `--dry-run` para aí; sem terminal, só aplica com `--yes`.
2. **Nunca sobrescrever o que não é do Atlana.** Um arquivo que já existe só é atualizado se o `atlana.lock` diz que é nosso **e** o hash bate (ninguém editou). Senão é conflito e nada é escrito.
3. **Lockfile.** `~/.config/atlana/atlana.lock` (escopo `user`, respeita `XDG_CONFIG_HOME`) ou `./atlana.lock` (escopo `project`), com pack, versão, origem, agente, módulo e `sha256` de cada arquivo. Itens mantidos dentro de arquivos que não são do Atlana (servidores MCP) ficam em `keys`: caminho, nome e `sha256` do valor normalizado.
4. **Uninstall exato.** Remove só arquivos do lock cujo hash ainda bate. Arquivos editados são mantidos e avisados.
5. **Segredos nunca são gravados.** Manifestos listam nomes de variáveis, e em `mcp.json` os valores de `env` e `headers` só podem ser `${NOME}` (headers aceitam também `Bearer ${NOME}`). Valor literal é erro no `validate`.
6. **Merge não destrutivo.** Configs de agentes são editadas servidor a servidor; as outras chaves e servidores ficam como estavam. Se o arquivo mudou entre o plano e o apply, nada é gravado nele.

## 7. Onde cada agente recebe skills (v0.1)

| Agente | `--scope user` | `--scope project` |
|---|---|---|
| Cursor (`cursor`) | `~/.cursor/skills/<skill>/` | `.cursor/skills/<skill>/` |
| Claude Code (`claude`) | `~/.claude/skills/<skill>/` | `.claude/skills/<skill>/` |
| Codex CLI (`codex`) | `~/.agents/skills/<skill>/` | `.agents/skills/<skill>/` |
| Gemini CLI (`gemini`) | `~/.gemini/skills/<skill>/` | `.gemini/skills/<skill>/` |
| Antigravity (`antigravity`) | `~/.gemini/config/skills/<skill>/` | `.agents/skills/<skill>/` (a mesma pasta do Codex) |
| Grok Bot (`grok`), só exporta | `$XDG_CONFIG_HOME/atlana/exports/grok/skills/<skill>/` (padrão `~/.config/...`) | `.atlana/exports/grok/skills/<skill>/` |
| Crush (`crush`) | `$CRUSH_SKILLS_DIR` ou `$XDG_CONFIG_HOME/crush/skills/<skill>/` | `.crush/skills/<skill>/` |
| OpenCode (`opencode`) | `$XDG_CONFIG_HOME/opencode/skills/<skill>/` | `.opencode/skills/<skill>/` |

O Grok Bot não tem pasta local de skills: o adaptador exporta as skills sem mudanças, e a pessoa envia cada pasta pelo próprio Grok. O lock registra os arquivos exportados como qualquer outra instalação.

## 8. Onde cada agente recebe rules (v0.1)

Cada arquivo de `modules.rules` é instalado como arquivo próprio do Atlana. O `CLAUDE.md`, o `GEMINI.md` e o `AGENTS.md` da pessoa nunca são tocados (ADR 0003).

| Agente | `--scope user` | `--scope project` | Ativação |
|---|---|---|---|
| Claude Code (`claude`) | `~/.claude/rules/atlana-<pack>/<arquivo>` | `.claude/rules/atlana-<pack>/<arquivo>` | automática |
| Cursor (`cursor`) | não suportado (rules de usuário ficam no app) | `.cursor/rules/atlana-<pack>-<slug>.mdc` | automática (`alwaysApply: true`) |
| Gemini CLI (`gemini`) | `~/.gemini/atlana/atlana-<pack>/<arquivo>` | `.gemini/atlana/atlana-<pack>/<arquivo>` | a CLI mostra a linha `@...` para o `GEMINI.md` |
| Antigravity (`antigravity`) | `~/.gemini/config/rules/atlana-<pack>-<arquivo>.md` | `.agents/rules/atlana-<pack>-<arquivo>.md` | arquivo plano com `trigger: always_on`; carregado sozinho |
| OpenCode (`opencode`) | `$XDG_CONFIG_HOME/opencode/atlana/atlana-<pack>/<arquivo>.md` (plano: `docs/x.md` vira `docs-x.md`) | `.opencode/atlana/atlana-<pack>/<arquivo>.md` | um item por pack em `instructions` do `opencode.json(c)`: caminho absoluto `…/atlana-<pack>/*.md` (user) ou `.opencode/atlana/atlana-<pack>/*.md` (project) |
| Codex CLI (`codex`) | `$CODEX_HOME/atlana/atlana-<pack>/<arquivo>` (padrão `~/.codex`) | `.codex/atlana/atlana-<pack>/<arquivo>` | manual, no `AGENTS.md` |

No `.mdc` do Cursor, o `<slug>` é o caminho sem extensão com `/` trocado por `-`, e um frontmatter do arquivo de origem é substituído pelo do Cursor.

## 9. Onde cada agente recebe servidores MCP (v0.1)

Cada servidor de `modules.mcp` entra por merge (ADR 0004). `${PLUGIN_ROOT}` vira o diretório do pack.

| Agente | `--scope user` | `--scope project` | Tradução |
|---|---|---|---|
| Claude Code (`claude`) | `~/.claude.json` → `mcpServers` | `.mcp.json` → `mcpServers` | igual; `url` sem `type` ganha `type: http` |
| Cursor (`cursor`) | `~/.cursor/mcp.json` → `mcpServers` | `.cursor/mcp.json` → `mcpServers` | `${VAR}` vira `${env:VAR}` |
| Gemini CLI (`gemini`) | `~/.gemini/settings.json` → `mcpServers` | `.gemini/settings.json` → `mcpServers` | `url` vira `httpUrl` (exceto `type: sse`) |
| Antigravity (`antigravity`) | `~/.gemini/config/mcp_config.json` → `mcpServers` | não suportado | não expande `${VAR}`: `"NOME": "${NOME}"` em `env` sai da entrada (o servidor herda a variável do `agy`); outro `${VAR}` (ex.: `Bearer ${TOKEN}` em `headers`) recusa o servidor; `sse` é recusado |
| OpenCode (`opencode`) | `$XDG_CONFIG_HOME/opencode/opencode.json` (ou `.jsonc`, se existir) → `mcp` | `opencode.json` na raiz → `mcp` | stdio vira `type: local` com `command` em array e `environment`; HTTP/SSE vira `type: remote`; `${VAR}` vira `{env:VAR}`; `cwd` é recusado |
| Codex CLI (`codex`) | `$CODEX_HOME/config.toml` (padrão `~/.codex`) → `[mcp_servers.<nome>]` | `.codex/config.toml` | `env` vira `env_vars` (a variável precisa ter o mesmo nome); `Authorization: Bearer ${X}` vira `bearer_token_env_var`; outros headers, `env_http_headers` |

No Codex, se o `config.toml` declara `mcp_servers` de outro jeito (tabela `[mcp_servers]`, inline ou chave com ponto), a instalação para com conflito.

## 10. Onde cada agente recebe subagents e hooks (v0.1)

| Agente | Subagents | Hooks |
|---|---|---|
| Claude Code (`claude`) | `.claude/agents/<nome>.md` (tools `Read, Grep…`; model `inherit/haiku/opus`) | `.claude/settings.json` → `hooks` (`PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `Stop`) |
| Gemini CLI (`gemini`) | `.gemini/agents/<nome>.md` (`kind: local`; tools `read_file…`; `fast`/`smart` usam o modelo da sessão, com aviso) | `.gemini/settings.json` → `hooks` (`BeforeTool`, `AfterTool`, `BeforeAgent`, `SessionStart`, `AfterAgent`) |
| OpenCode (`opencode`) | `$XDG_CONFIG_HOME/opencode/agents/<nome>.md` ou `.opencode/agents/<nome>.md` (`mode: subagent`; `permission` com `edit`, `bash`, `webfetch`; `shell:readonly` vira `bash: ask`; leitura e busca sempre liberadas; `fast`/`smart` usam o modelo da sessão) | plugin gerado por pack em `$XDG_CONFIG_HOME/opencode/plugins/atlana-<pack>.js` (ou `.opencode/plugins/`), que chama `atlana hook dispatch --pack <pack>`: `tool.execute.before` (nega com exit 2), `tool.execute.after`, `session.created`, `session.idle`; sem `prompt_submit` |
| Cursor, Codex | pulado, com aviso | `fallback` de cada hook |
| Grok Bot | pulado, com aviso | `skip` (não tem rules) |

Os caminhos valem para `--scope user` (em `~`) e `--scope project` (no projeto). Os scripts são copiados para `$XDG_CONFIG_HOME/atlana/hooks/<agente>/<pack>/<hook>/` (user) ou `.atlana/hooks/<agente>/<pack>/<hook>/` (project). Na config do agente entra **um item por evento**, compartilhado entre packs, que chama `atlana hook dispatch`. O binário precisa continuar no caminho em que estava no `install` (use `go install`, não `go run`).

Shell `shell:readonly` não tem equivalente nos dois agentes e fica de fora. No Claude Code, `mcp` também fica de fora (não há curinga portável); no Gemini vira `mcp_*`.

## 11. Origens remotas e registry (v0.1)

`atlana install` aceita, nesta ordem:

1. **Origem GitHub:** `github:owner/repo[//caminho][@ref]`. O `ref` pode ser branch, tag ou commit; sem ele, vale a branch padrão.
2. **Caminho local** de um pack.
3. **Nome do registry:** `<nome>[//caminho][@ref]`.

A busca usa o `git` da pessoa, com o prompt de senha desligado. Primeiro HTTPS (com o `gh auth git-credential` somado aos credential helpers, se o `gh` estiver no PATH) e depois SSH (`BatchMode`). O repo fica em cache em `$XDG_CACHE_HOME/atlana/sources/github/<owner>/<repo>` (padrão `~/.cache/...`). O lock grava a origem com o **commit exato** (`github:o/r//caminho@<sha>`). Nenhum token é lido ou gravado.

O registry fica em `$XDG_CONFIG_HOME/atlana/hub.yaml`:

```yaml
apiVersion: atlana/v1
kind: HubRegistry
projects:
  - name: atlana                 # nome de pack (minúsculas e hífens), único
    origin: github:atlana-ai-hq/atlana   # só o repo, sem //caminho nem @ref
    visibility: public           # public | private
```

`atlana registry list` só mostra projetos `private` quando `git ls-remote` consegue acessá-los com a autenticação atual (`--all` mostra todos, com o estado). Campos desconhecidos são erro, e isso inclui qualquer tentativa de guardar um token.

## 12. Plugins nativos: `atlana build` (v0.1)

`atlana build <pack> --target <t>[,<t>] [--out dist] [--only …]` gera um artefato por target em `<out>/<target>/<nome>/`, para quem não quer a CLI. Os hooks só entram com `--only hooks`, como no `install`.

| Target | Manifesto | Skills | Rules | Subagents | Hooks | MCP |
|---|---|---|---|---|---|---|
| `claude-plugin` | `.claude-plugin/plugin.json` | `skills/` | não (plugins não carregam `CLAUDE.md`) | `agents/` | `hooks/hooks.json` | `.mcp.json` (`${CLAUDE_PLUGIN_ROOT}`) |
| `gemini-extension` | `gemini-extension.json` | `skills/` | `GEMINI.md` (`contextFileName`) | `agents/` | `hooks/hooks.json` | no manifesto (`${extensionPath}`) |
| `cursor-plugin` | `.cursor-plugin/plugin.json` | `skills/` | `rules/*.mdc` | não | `fallback` (`rule` vira `.mdc`) | `mcp.json` (sem servidores que usam `${PLUGIN_ROOT}`) |
| `agent-plugin` | `plugin.json` (Agent Plugins 1.0.0) | `skills/` | não | não | não | `mcp.json` (só servidores sem `${VAR}`) |

- **Hooks em plugin:** chamam `atlana hook dispatch` com a pasta `hooks/atlana` do plugin, então quem instala precisa do `atlana` no PATH.
- **`agent-plugin`:** segue os campos fechados da spec e os `$schema` exatos. Como a spec não expande variáveis de ambiente e proíbe segredos, servidores que dependem de `${VAR}` ficam de fora, com aviso.
- **Arquivos de apoio:** arquivos e pastas do pack fora dos módulos (ex.: `bin/`, `LICENSE`, `README.md`) são copiados para a raiz do artefato. Sem `README.md` no pack, o build gera um.
- **Sobrescrita:** o diretório de saída só é substituído se tiver o marcador `.atlana-build.json`; qualquer outro conteúdo é recusado.

## Commands

`commands/<nome>.md` é um slash command: frontmatter com `description` (obrigatório) e `argument-hint` (opcional), e o corpo é o prompt. `$ARGUMENTS` é o texto digitado depois do comando. O nome vem do arquivo.

| Agente | Destino (user / project) | Tradução |
|---|---|---|
| Claude Code | `~/.claude/commands/` / `.claude/commands/` | igual |
| Gemini CLI | `~/.gemini/commands/<nome>.toml` / `.gemini/commands/` | `description` + `prompt`; `$ARGUMENTS` vira `{{args}}` |
| Antigravity | `~/.gemini/config/skills/<nome>/SKILL.md` / `.agents/skills/` | vira skill (no Antigravity a skill é o `/<nome>`; workflows estão descontinuados); `$ARGUMENTS` vira uma instrução |
| Cursor | `~/.cursor/commands/` / `.cursor/commands/` | Markdown puro; o Cursor anexa o texto digitado (aviso se o corpo usa `$ARGUMENTS`) |
| OpenCode | `$XDG_CONFIG_HOME/opencode/commands/` / `.opencode/commands/` | `description` + corpo; sem `argument-hint` |
| Codex, Crush, Grok | não instalado | os custom prompts do Codex foram descontinuados: use uma skill |

`$1`…`$9` só funcionam no Claude Code e no OpenCode; o `validate` avisa. No `atlana build`, commands entram no plugin do Claude e na extensão do Gemini.

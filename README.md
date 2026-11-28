# Atlana

**Um formato, todos os agentes.** Escreva skills, subagents, hooks e servidores MCP uma vez, no formato Atlana, e instale só o que quiser no Cursor, no Claude Code, no Codex, no Gemini CLI, no Antigravity e em outros agentes.

> Status: **v0.1, em construção.** Hoje a CLI valida packs e instala **skills**, **rules** e **servidores MCP** no Cursor, no Claude Code, no Codex e no Gemini CLI (e exporta skills para o Grok Bot), **skills** no Crush e no OpenCode, e **rules**, **MCP**, **subagents** e **hooks** no OpenCode, além de **subagents** e **hooks** no Claude Code e no Gemini CLI e **subagents** no Cursor e no Codex. No Antigravity: **skills**, **rules**, **MCP**, **subagents**, **hooks** e **commands** (como skills). Os outros módulos e agentes estão no [roadmap](#roadmap).

[English below](#english)

## Por que

Cada agente de IA guarda as mesmas coisas em lugares e formatos diferentes (`.cursor/`, `.claude/`, `.codex/`, `.gemini/`…). Quem mantém um conjunto de skills acaba copiando arquivo à mão, sem saber o que instalou onde e sem um jeito seguro de desinstalar.

O Atlana resolve isso com três peças:

1. **Formato aberto.** Usa os padrões que já existem ([Agent Skills](https://agentskills.io), `AGENTS.md`, `mcp.json` de Agent Plugins) e só define o que falta (hooks e subagents genéricos) num `atlana.yaml` pequeno por pack. Veja a [spec v0.1](spec/README.md).
2. **Adaptadores por agente.** Cada agente é um arquivo em Go que sabe detectar o agente e traduzir o pack para os caminhos dele. Suportar um agente novo é escrever um adaptador.
3. **Instalação segura.** Mostra o plano antes de aplicar, nunca sobrescreve arquivo que não é dele e registra tudo num `atlana.lock`, então `status` e `uninstall` são exatos.

## Instalação

| Sistema | Como instalar |
|---|---|
| macOS | `go install` (abaixo). O binário do release ainda não é assinado pela Apple, e o Gatekeeper bloqueia a execução. Não remova a quarentena para contornar. |
| Linux e Windows | `go install` ou o binário do [release](https://github.com/atlana-ai-hq/atlana/releases/latest) (confira com `checksums.txt`) |

Com Go 1.24+:

```bash
go install github.com/atlana-ai-hq/atlana/cmd/atlana@latest
```

O `brew install --cask atlana-ai-hq/tap/atlana` volta a ser recomendado quando o binário de macOS for assinado e notarizado ([docs/release.md](docs/release.md#secrets)).

## Uso

```bash
atlana doctor                         # quais agentes estão instalados e o que cada adaptador suporta
atlana validate packs                 # valida atlana.yaml + SKILL.md de todos os packs

# ver o plano sem mexer em nada
atlana install packs/hello --only skills --agent cursor,claude --dry-run

# aplicar (pergunta antes; em CI use --yes)
atlana install packs/hello --only skills --agent cursor,claude

atlana status                         # o que está instalado e se alguém editou
atlana uninstall hello                # remove exatamente o que o lock registrou
```

Flags de `install` e `uninstall`:

| Flag | O que faz |
|---|---|
| `--agent cursor,claude` | agentes alvo (padrão: os detectados pelo `doctor`) |
| `--only skills` | módulos: `rules`, `skills`, `agents`, `hooks`, `commands`, `mcp` |
| `--scope user\|project` | `~` (padrão) ou o diretório atual |
| `--dry-run` | só mostra o plano |
| `--yes` | aplica sem perguntar |

Exemplo de plano:

```text
Plano: Instalar hello@0.1.0 (escopo user)
  criar     [cursor/skills] ~/.cursor/skills/conventional-commit-message/SKILL.md
  criar     [claude/skills] ~/.claude/skills/conventional-commit-message/SKILL.md
Resumo: 2 criar · lock: ~/.config/atlana/atlana.lock
```

## Escrevendo um pack

```
packs/hello/
├── atlana.yaml
└── skills/conventional-commit-message/SKILL.md
```

```yaml
apiVersion: atlana/v1
kind: Pack
name: hello
version: 0.1.0
description: Pack de exemplo com uma skill genérica.
license: MIT
modules:
  skills: [conventional-commit-message]
```

Para criar o seu, parta de [`templates/pack`](templates/pack) e siga o [guia de autoria](docs/autoria.md). Já tem skills e regras soltas nos agentes? Veja o [guia de migração](docs/migracao.md).

Rode `atlana validate packs/hello` antes de abrir um PR. O schema completo está em [`spec/schemas/atlana.schema.json`](spec/schemas/atlana.schema.json).

## Suporte atual

| Agente | Detecção | Skills | Rules | Subagents | Hooks | MCP | Commands |
|---|---|---|---|---|---|---|---|
| Cursor | ✅ | ✅ | ✅ (project) | ✅ | fallback | ✅ | ✅ |
| Claude Code | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Codex CLI | ✅ | ✅ | ✅ (manual) | ✅ (TOML) | fallback | ✅ | ➖ (use skills) |
| Gemini CLI | ✅ | ✅ | ✅ (`@import`) | ✅ | ✅ | ✅ | ✅ (TOML) |
| Antigravity (`agy`) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ (user) | ✅ (vira skill) |
| Grok Bot | ✅ | ✅ (exporta) | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| Crush | ✅ | ✅ | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| OpenCode | ✅ | ✅ | ✅ (`instructions`) | ✅ | ✅ (plugin) | ✅ | ✅ |

No Grok Bot, `install --agent grok` exporta as pastas `SKILL.md` (formato Agent Skills) para `~/.config/atlana/exports/grok/skills/` (ou `.atlana/exports/grok/skills/` com `--scope project`), e você envia cada pasta pelo Grok. Lock, `status` e `uninstall` funcionam igual. Sem MCP remoto nem sync automático no v1 ([ADR 0002](docs/adr/0002-grok-exporta-skills.md)).

Rules nunca mexem no seu `CLAUDE.md`, `GEMINI.md` ou `AGENTS.md`: o Atlana grava só arquivos próprios. No Claude Code e no Cursor eles são carregados sozinhos; no Gemini CLI a CLI mostra a linha `@...` para importar no `GEMINI.md`; no Codex você referencia o arquivo no seu `AGENTS.md` ([ADR 0003](docs/adr/0003-rules-em-arquivos-proprios.md)).

Servidores MCP entram por merge na config do agente (`~/.claude.json`/`.mcp.json`, `.cursor/mcp.json`, `.gemini/settings.json`, `.codex/config.toml`): o resto do arquivo fica como estava, e o lock guarda o hash de cada servidor, não do arquivo. Em `env` e `headers` o pack só pode usar `${VAR}` (ou `Bearer ${VAR}`); valores literais são erro no `validate` ([ADR 0004](docs/adr/0004-mcp-merge-com-posse-no-lock.md)).

Packs remotos: `atlana install github:owner/repo//caminho@ref` busca com o seu `git` (repos privados funcionam com `gh auth login` ou SSH), guarda em cache e grava no lock o commit exato. O registry local (`atlana registry add|list|remove`, em `~/.config/atlana/hub.yaml`) dá nomes curtos às origens (`atlana install meus-packs//packs/work`) e esconde os privados que você não acessa ([ADR 0006](docs/adr/0006-origens-remotas-e-registry.md)).

Quem não quer a CLI pode receber plugins nativos: `atlana build meu-pack --target claude-plugin,gemini-extension,cursor-plugin,agent-plugin` gera cada um em `dist/` ([ADR 0007](docs/adr/0007-build-de-plugins-nativos.md)).

Hooks só entram com `--only hooks`. Os scripts vão para uma pasta do Atlana, e a config do agente ganha um item por evento que chama `atlana hook dispatch`. O dispatcher normaliza o payload, aplica `match` e repassa `exit 2` (negar). Instale a CLI com `go install`, porque o caminho do binário fica gravado na config. Agentes sem hooks seguem o `fallback` do hook (`rule`, `skip` ou `fail`) ([ADR 0005](docs/adr/0005-subagents-e-hooks-com-dispatcher.md)).

## Roadmap

- [x] Spec v0.1 + JSON Schema, `validate`, `doctor`
- [x] Planner, lockfile, `install`/`status`/`uninstall` de skills (Cursor, Claude Code)
- [x] Exportação de skills para o Grok Bot
- [x] Skills no Crush e no OpenCode ([ADR 0009](docs/adr/0009-crush-e-opencode.md))
- [x] MCP e subagents no OpenCode ([ADR 0010](docs/adr/0010-opencode-mcp-e-subagents.md))
- [x] Rules e hooks no OpenCode ([ADR 0011](docs/adr/0011-opencode-rules-e-hooks.md))
- [ ] Rules, MCP, subagents e hooks no Crush
- [x] Skills no Gemini CLI
- [x] Antigravity: skills, rules, MCP, subagents, hooks e commands
- [x] Skills no Codex
- [x] Rules em arquivos próprios (Claude Code, Cursor, Gemini CLI, Codex)
- [x] MCP com merge não destrutivo de JSON/TOML (sem gravar segredos)
- [x] Hooks com dispatcher de eventos genéricos e subagents (Claude Code, Gemini CLI)
- [x] `atlana build --target claude-plugin|cursor-plugin|gemini-extension|agent-plugin`
- [x] Origens `github:owner/repo//caminho@ref` e registry local (`atlana registry`)
- [x] `atlana scan`, configuração de release (GoReleaser + Homebrew), template e guias de autoria/migração
- [x] Primeiro release publicado (workflow, binários, tap)
- [ ] Assinatura e notarização no macOS (Apple Developer Program)

## Desenvolvimento

```bash
go test ./...
go vet ./...
go run ./cmd/atlana validate packs
```

Decisões de arquitetura ficam em [`docs/adr/`](docs/adr/). Instruções para agentes de código estão em [`AGENTS.md`](AGENTS.md).

## Licença

[MIT](LICENSE)

---

## English

**One format, every agent.** Atlana lets you write skills, subagents, hooks and MCP servers once and install only what you want into Cursor, Claude Code, Codex, Gemini CLI and others.

- **Open standards first:** [Agent Skills](https://agentskills.io) `SKILL.md`, `AGENTS.md` and Agent Plugins `mcp.json`, plus a small `atlana.yaml` per pack for what has no common standard yet. See the [v0.1 spec](spec/README.md) (in Portuguese).
- **Per-agent adapters** written in Go: adding an agent means adding one adapter.
- **Safe installs:** every `install`/`uninstall` shows a plan first, never overwrites files it doesn't own, and records everything in `atlana.lock` so `status` and `uninstall` are exact.

```bash
go install github.com/atlana-ai-hq/atlana/cmd/atlana@latest
atlana doctor
atlana install packs/hello --only skills --agent cursor,claude --dry-run
```

On macOS, install with `go install`: the release binary isn't signed by Apple yet, so Gatekeeper blocks it (don't remove the quarantine flag to work around it). On Linux and Windows, `go install` or the [release binaries](https://github.com/atlana-ai-hq/atlana/releases/latest) both work. Homebrew will be recommended once the macOS binary is signed and notarized.

Status: v0.1. Skills, rules and MCP servers install into Cursor, Claude Code, Codex, Gemini CLI, OpenCode and Antigravity (and skills export for Grok Bot); subagents install into Claude Code, Gemini CLI, OpenCode, Antigravity, Cursor and Codex, and hooks into the first four; slash commands into Claude Code, Gemini CLI, Cursor, OpenCode and Antigravity. MIT licensed.

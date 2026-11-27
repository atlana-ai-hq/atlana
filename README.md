# Atlana

**Um formato, todos os agentes.** Escreva skills, subagents, hooks e servidores MCP uma vez, no formato Atlana, e instale só o que quiser no Cursor, no Claude Code, no Codex, no Gemini CLI e em outros agentes.

> Status: **v0.1, em construção.** Hoje a CLI valida packs e instala **skills** no Cursor, no Claude Code e no Gemini CLI (e exporta para o Grok Bot). Os outros módulos e agentes estão no [roadmap](#roadmap).

[English below](#english)

## Por que

Cada agente de IA guarda as mesmas coisas em lugares e formatos diferentes (`.cursor/`, `.claude/`, `.codex/`, `.gemini/`…). Quem mantém um conjunto de skills acaba copiando arquivo à mão, sem saber o que instalou onde e sem um jeito seguro de desinstalar.

O Atlana resolve isso com três peças:

1. **Formato aberto.** Usa os padrões que já existem ([Agent Skills](https://agentskills.io), `AGENTS.md`, `mcp.json` de Agent Plugins) e só define o que falta (hooks e subagents genéricos) num `atlana.yaml` pequeno por pack. Veja a [spec v0.1](spec/README.md).
2. **Adaptadores por agente.** Cada agente é um arquivo em Go que sabe detectar o agente e traduzir o pack para os caminhos dele. Suportar um agente novo é escrever um adaptador.
3. **Instalação segura.** Mostra o plano antes de aplicar, nunca sobrescreve arquivo que não é dele e registra tudo num `atlana.lock`, então `status` e `uninstall` são exatos.

## Instalação

Requer Go 1.24+.

```bash
go install github.com/atlana-ai-hq/atlana/cmd/atlana@latest
```

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
apiVersion: atlana/v1alpha1
kind: Pack
name: hello
version: 0.1.0
description: Pack de exemplo com uma skill genérica.
license: MIT
modules:
  skills: [conventional-commit-message]
```

Rode `atlana validate packs/hello` antes de abrir um PR. O schema completo está em [`spec/schemas/atlana.schema.json`](spec/schemas/atlana.schema.json).

## Suporte atual

| Agente | Detecção | Skills | Rules | Subagents | Hooks | MCP |
|---|---|---|---|---|---|---|
| Cursor | ✅ | ✅ | ⏳ | ⏳ | ⏳ | ⏳ |
| Claude Code | ✅ | ✅ | ⏳ | ⏳ | ⏳ | ⏳ |
| Codex CLI | ✅ | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| Gemini CLI | ✅ | ✅ | ⏳ | ⏳ | ⏳ | ⏳ |
| Grok Bot | ✅ | ✅ (exporta) | ⏳ | ⏳ | ⏳ | ⏳ |

No Grok Bot, `install --agent grok` exporta as pastas `SKILL.md` (formato Agent Skills) para `~/.config/atlana/exports/grok/skills/` (ou `.atlana/exports/grok/skills/` com `--scope project`), e você envia cada pasta pelo Grok. Lock, `status` e `uninstall` funcionam igual. Sem MCP remoto nem sync automático no v1 ([ADR 0002](docs/adr/0002-grok-exporta-skills.md)).

## Roadmap

- [x] Spec v0.1 + JSON Schema, `validate`, `doctor`
- [x] Planner, lockfile, `install`/`status`/`uninstall` de skills (Cursor, Claude Code)
- [x] Exportação de skills para o Grok Bot
- [x] Skills no Gemini CLI
- [ ] Skills no Codex; rules (`AGENTS.md` → `CLAUDE.md`/`GEMINI.md`/`.mdc`)
- [ ] MCP com merge não destrutivo de JSON/TOML (sem gravar segredos)
- [ ] Hooks com shim de eventos genéricos; subagents
- [ ] `atlana build --target claude-plugin|cursor-plugin|gemini-extension|agent-plugin`
- [ ] Releases com binários e `brew tap`

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

Status: v0.1. Skills install into Cursor, Claude Code and Gemini CLI today (and export for Grok Bot); Codex, rules, MCP, hooks and subagents are on the roadmap. MIT licensed.

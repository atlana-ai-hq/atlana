# Especificação Atlana v0.1 (rascunho)

> `apiVersion: atlana/v1alpha1` · status: **rascunho**, pode mudar sem aviso até a v1.

O formato Atlana é uma camada **fina** por cima de padrões abertos. A regra principal é: **não inventar formato onde já existe padrão**.

| Conteúdo | Padrão usado | Status na v0.1 |
|---|---|---|
| Skills | [Agent Skills](https://agentskills.io/specification) (`SKILL.md`), sem mudanças | validado e instalado (Cursor, Claude Code) |
| Rules | `AGENTS.md` (fonte); `CLAUDE.md`, `GEMINI.md` e `.mdc` são gerados | validado (existência), instalação futura |
| MCP | `mcp.json` no formato de Agent Plugins (`mcpServers`) | validado (chaves), instalação futura |
| Subagents | formato Atlana (`agents/<nome>.md`) | validado (existência), instalação futura |
| Hooks | formato Atlana (`hooks/<nome>/hook.yaml`) | validado (existência), instalação futura |
| Commands | formato Atlana (`commands/<nome>.md`) | validado (existência), instalação futura |

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
apiVersion: atlana/v1alpha1    # obrigatório
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

## 4. Hooks (rascunho, ainda não instalados)

```yaml
name: protect-plan-files
event: pre_tool          # pre_tool | post_tool | prompt_submit | session_start | stop
match:
  tool: [edit, write]    # shell | edit | write | read | mcp
  path: [".plan/**"]
run: ./run.sh            # recebe JSON normalizado no stdin; exit 2 = negar
timeout: 10
fallback: rule           # agente sem hooks: rule | skip | fail
```

O adaptador vai gerar um shim que converte o payload nativo de cada agente em um JSON único (`{"event","tool","path","command","agent"}`) e a resposta de volta.

## 5. Subagents (rascunho, ainda não instalados)

```markdown
---
name: planner
description: Roda o planejamento em contexto isolado.
tools: [read, search, shell:readonly]   # capacidades abstratas
model: inherit                          # inherit | fast | smart
---
```

## 6. Instalação: garantias

Toda implementação da CLI precisa respeitar:

1. **Plano antes de aplicar.** `install` e `uninstall` mostram cada arquivo que vão criar, atualizar ou remover. `--dry-run` para aí; sem terminal, só aplica com `--yes`.
2. **Nunca sobrescrever o que não é do Atlana.** Um arquivo que já existe só é atualizado se o `atlana.lock` diz que é nosso **e** o hash bate (ninguém editou). Senão é conflito e nada é escrito.
3. **Lockfile.** `~/.config/atlana/atlana.lock` (escopo `user`, respeita `XDG_CONFIG_HOME`) ou `./atlana.lock` (escopo `project`), com pack, versão, origem, agente, módulo e `sha256` de cada arquivo.
4. **Uninstall exato.** Remove só arquivos do lock cujo hash ainda bate. Arquivos editados são mantidos e avisados.
5. **Segredos nunca são gravados.** Manifestos listam nomes de variáveis; merges de config (JSON/TOML) futuros vão referenciar variáveis, nunca valores.

## 7. Onde cada agente recebe skills (v0.1)

| Agente | `--scope user` | `--scope project` |
|---|---|---|
| Cursor (`cursor`) | `~/.cursor/skills/<skill>/` | `.cursor/skills/<skill>/` |
| Claude Code (`claude`) | `~/.claude/skills/<skill>/` | `.claude/skills/<skill>/` |
| Codex CLI (`codex`) | só detecção | só detecção |
| Gemini CLI (`gemini`) | só detecção | só detecção |
| Grok Bot (`grok`), só exporta | `$XDG_CONFIG_HOME/atlana/exports/grok/skills/<skill>/` (padrão `~/.config/...`) | `.atlana/exports/grok/skills/<skill>/` |

O Grok Bot não tem pasta local de skills: o adaptador exporta as skills sem mudanças, e a pessoa envia cada pasta pelo próprio Grok. O lock registra os arquivos exportados como qualquer outra instalação.

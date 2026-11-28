# Como migrar o que você já tem para o formato Atlana

Se você já tem skills, regras ou servidores MCP espalhados em `.claude/`, `.cursor/`, `.gemini/` ou `.codex/`, dá para juntar tudo num pack e instalar de um lugar só.

## 1. Crie o pack

```bash
cp -R templates/pack ~/meus-packs/meu-pack
```

Apague os exemplos que você não vai usar (pastas e entradas em `modules`).

## 2. Skills

As skills dos agentes já seguem o padrão Agent Skills, então é só copiar a pasta:

```bash
cp -R ~/.claude/skills/minha-skill ~/meus-packs/meu-pack/skills/
```

Liste `minha-skill` em `modules.skills`. Confira se o `name` do frontmatter é igual ao nome da pasta. O `validate` acusa campos fora do padrão (eles funcionam em alguns agentes, mas não são portáveis).

## 3. Rules

Junte o conteúdo que hoje está em `CLAUDE.md`, `GEMINI.md`, `.cursor/rules/*.mdc` ou `AGENTS.md` num `AGENTS.md` do pack (ou em vários `.md`) e liste-os em `modules.rules`.

- Tire o frontmatter dos `.mdc`; o Atlana gera um novo para o Cursor.
- Tire referências a caminhos da sua máquina.

Depois de instalar com o Atlana, apague a cópia antiga do seu `CLAUDE.md`, para o agente não ler a mesma regra duas vezes.

## 4. Subagents

De `.claude/agents/<nome>.md` ou `.gemini/agents/<nome>.md` para `agents/<nome>.md`:

| Antes (Claude) | Antes (Gemini) | No pack |
|---|---|---|
| `Read` | `read_file` | `read` |
| `Grep`, `Glob` | `glob`, `search_file_content` | `search` |
| `Edit` | `replace` | `edit` |
| `Write` | `write_file` | `write` |
| `Bash` | `run_shell_command` | `shell` |
| `WebFetch`, `WebSearch` | `web_fetch`, `google_web_search` | `web` |
| `model: haiku` / `opus` | nome do modelo | `model: fast` / `smart` |

O corpo (o prompt) fica igual.

## 5. MCP

Copie a entrada de `mcpServers` da sua config para o `mcp.json` do pack e troque cada valor secreto por `${NOME}`:

```json
"env": { "GITHUB_TOKEN": "${GITHUB_TOKEN}" }
```

Liste `GITHUB_TOKEN` em `requires.env` e a chave do servidor em `modules.mcp`.

Na primeira instalação, o servidor que já está na sua config com o mesmo nome dá **conflito**, porque o Atlana não sobrescreve o que não instalou. Remova a entrada antiga da config e instale de novo.

## 6. Hooks

Um hook de `settings.json` vira `hooks/<nome>/hook.yaml` + script. O script passa a receber o JSON normalizado (`tool`, `path`, `command`…), em vez do payload de cada agente. Assim o mesmo hook funciona no Claude Code e no Gemini CLI. Mantenha `exit 2` para negar.

## 7. Commands

De `.claude/commands/<nome>.md` (ou `.cursor/commands/`) para `commands/<nome>.md`. Mantenha `description`, troque `$1`… por `$ARGUMENTS` e tire `allowed-tools`/`model`, que não são portáveis. Um `.gemini/commands/<nome>.toml` vira `description` no frontmatter e o `prompt` no corpo, com `{{args}}` trocado por `$ARGUMENTS`. Custom prompts do Codex viram skills.

## 8. Instale e confira

```bash
atlana validate ~/meus-packs/meu-pack
atlana install ~/meus-packs/meu-pack --dry-run
```

O plano mostra o que vai ser criado e os conflitos com o que já existe. Resolva os conflitos (apagando a cópia antiga) e aplique.

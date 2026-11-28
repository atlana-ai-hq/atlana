# 0016. Subagents no Cursor e no Codex

## Contexto

O Cursor lê subagents em `.cursor/agents/<nome>.md` (frontmatter `name`, `description`, `model`, `readonly`, `is_background`) e também em `.claude/agents/`, com `.cursor/` vencendo em nome repetido. Não há lista de ferramentas: o único controle é `readonly`, que bloqueia edição e shell que muda estado.

O Codex lê `.codex/agents/<nome>.toml` com `name`, `description` e `developer_instructions`. O sandbox e as aprovações vêm sempre da sessão pai; o arquivo só consegue desligar recursos, como `features.shell_tool`.

Nenhum dos dois tem um alias estável para `fast`/`smart`.

## Decisão

- **Cursor:** `model: inherit`. Com `tools` no pack e sem `edit`, `write` nem `shell`, o subagent ganha `readonly: true`. Em qualquer outro caso fica com as ferramentas da sessão, e o plano avisa.
- **Cursor sem duplicata:** se o pack vai para o Claude Code no mesmo escopo (no plano ou no lock), o Cursor já lê `.claude/agents`; nada é gravado em `.cursor/agents`, e as cópias antigas do atlana lá são removidas (como no ADR 0015).
- **Codex:** sem `edit`/`write`, o prompt começa com "Não edite nem crie arquivos." e o plano avisa que isso não é bloqueio. Sem `shell` nem `shell:readonly`, `[features] shell_tool = false`.
- `fast`/`smart` usam o modelo da sessão nos dois, com aviso.
- `atlana build` ainda não gera subagents no plugin do Cursor.

## Consequências

No Codex, um subagent "só leitura" depende do modelo seguir a instrução, ou do sandbox da sessão. Quem precisa de garantia deve rodar o Codex com sandbox read-only.

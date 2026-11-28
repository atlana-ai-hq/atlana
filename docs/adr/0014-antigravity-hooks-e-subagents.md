# 0014. Hooks e subagents no Antigravity

## Contexto

O ADR 0013 deixou hooks e subagents do Antigravity para depois. Pela documentação embutida no `agy` 1.5.18:

- Subagents são `agents/<nome>/agent.md` na raiz de customização, com frontmatter (`subagent`, `mainAgent`, `tools`) e o prompt depois de um título H1. Sem `tools`, o agente recebe só um conjunto pequeno de leitura.
- Hooks ficam em `hooks.json`, num objeto de hooks nomeados. Cada um tem eventos (`PreToolUse`, `PostToolUse`, `SessionStart`, `PreInvocation`, `PostInvocation`, `Stop`); eventos de ferramenta usam `{matcher, hooks}`, os outros o handler direto. O script recebe `toolCall.{name,args}` e responde em JSON no stdout; `PreToolUse` aceita `decision: allow | deny | ask | force_ask`.

## Decisão

- Subagents: `agents/<nome>/agent.md` com `subagent: true` e `mainAgent: false`. As ferramentas abstratas viram `read_file`, `edit_file`, `write_file`, `run_command`/`manage_task` e `search_web`; `shell:readonly` e `mcp` ficam de fora com aviso. Sem `tools` no pack (= todas), o arquivo recebe a lista completa. `model` não tem equivalente e fica o da sessão.
- Hooks: um hook nomeado `atlana` em `hooks.json`, editado pelo mesmo merge de itens de array dos outros agentes (os hooks da pessoa, como `herdr`, ficam intactos). `prompt_submit` não é mapeado, porque `PreInvocation` roda antes de cada chamada ao modelo.
- O dispatcher ganha uma saída própria para o Antigravity: `{"decision":"deny","reason":…}` quando um hook nega em `pre_tool` e `{}` em todos os outros casos. Nunca `allow`, que aprovaria a ferramenta sem passar pelas permissões da pessoa. O exit code fica sempre 0.
- `Normalize` lê `toolCall.name`, `args.TargetFile | AbsolutePath | Path`, `args.CommandLine` e `lastUserInput`.

## Consequências

O mesmo hook do pack nega a ação no Claude Code, no Gemini CLI, no OpenCode e no Antigravity. Não verificado ao vivo: se o Antigravity trata `{}` como "sem decisão" e se encontra os subagents de `~/.gemini/config/agents` (`agy agents` não lista nada fora de uma sessão).

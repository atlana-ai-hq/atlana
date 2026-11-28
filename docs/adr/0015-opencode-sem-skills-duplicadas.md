# 0015. OpenCode sem skills duplicadas

## Contexto

O OpenCode 1.18.31 carrega skills de `~/.claude/skills` e `~/.agents/skills` (e, no projeto, de `.claude/skills` e `.agents/skills` subindo até a raiz), além de `skills/` na própria config. Instalar um pack no Claude Code, no Codex e no OpenCode gravava três cópias de cada skill, e o OpenCode registrava `duplicate skill name` para cada uma. As varreduras externas só desligam com `OPENCODE_DISABLE_EXTERNAL_SKILLS` ou `OPENCODE_DISABLE_CLAUDE_CODE(_SKILLS)`.

## Decisão

- Se o pack também vai para o Claude Code ou o Codex no mesmo escopo, no mesmo plano ou já registrado no lock, o atlana não grava skills em `skills/` do OpenCode. O plano mostra uma nota e remove as cópias que o próprio atlana gravou ali antes (arquivos do lock com o mesmo hash).
- As variáveis de ambiente do OpenCode são respeitadas: com a varredura externa desligada, a cópia própria volta.
- Commands, rules, MCP, subagents e hooks do OpenCode não mudam.

## Consequências

Com Claude Code e Codex juntos, o OpenCode ainda vê duas cópias (as pastas são desses agentes), então o aviso cai de duas duplicatas para uma por skill. Se a pessoa desinstalar o pack do Claude Code e do Codex, precisa reinstalar com `--agent opencode` para o OpenCode voltar a ter as skills.

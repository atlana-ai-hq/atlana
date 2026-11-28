# 0013. Adaptador Antigravity

## Contexto

O Google Antigravity (CLI `agy`) também usa `~/.gemini`, mas não lê os arquivos do Gemini CLI (`settings.json`, `skills/`, `commands/`). Sua raiz de customização é `~/.gemini/config/` (user) ou `<projeto>/.agents/` (project), com `skills/<n>/SKILL.md`, `rules/*.md`, `mcp_config.json` e `hooks.json`. O detector do Gemini CLI aceitava só a pasta `~/.gemini` e marcava como Gemini CLI quem tinha Antigravity.

Fontes: documentação embutida no `agy` 1.5.18 e testes com `agy mcp list` num HOME temporário.

## Decisão

- Gemini CLI passa a ser detectado pelo binário `gemini` ou por `~/.gemini/settings.json`. Antigravity, pelo binário `agy` ou por `~/.gemini/antigravity-cli`/`antigravity`.
- Skills em `skills/` da raiz. No projeto, `.agents/skills` é a mesma pasta do Codex; o lock já trata o arquivo com dono duplo (a desinstalação de um agente mantém o arquivo do outro).
- Rules viram arquivos planos `rules/atlana-<pack>-<slug>.md` com `trigger: always_on`.
- MCP por merge em `~/.gemini/config/mcp_config.json`, só no escopo user (a doc não traz MCP de projeto fora de plugins). O Antigravity não expande `${VAR}`: o repasse `"NOME": "${NOME}"` em `env` sai da entrada, e o servidor herda a variável do ambiente; qualquer outro `${VAR}` recusa o servidor, porque o atlana nunca grava o valor de um segredo. SSE é recusado.
- Commands viram skills, porque no Antigravity a skill é o slash command e os workflows estão descontinuados. `$ARGUMENTS` vira uma instrução no texto.
- Hooks e subagents ficam para depois. Hooks do Antigravity respondem com JSON no stdout (`decision: deny`), não com exit 2, e pedem um modo novo no dispatcher; até lá, valem os fallbacks.
- Plugins do Antigravity (`plugins/<nome>/` com `plugin.json`) seriam o encaixe natural de um pack, mas as interfaces de adaptador não recebem o nome do pack. Fica como evolução.

## Consequências

Quem tem Antigravity deixa de ganhar arquivos do Gemini CLI que nada lê. Servidores MCP com token em header não instalam no Antigravity até ele expandir variáveis ou o atlana usar plugins.

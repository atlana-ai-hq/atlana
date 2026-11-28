# 0012. Módulo commands

## Contexto

O spec já aceitava `commands/<nome>.md`, mas nada era instalado. Cada agente tem um formato: Claude Code e OpenCode usam Markdown com frontmatter e `$ARGUMENTS`; o Gemini CLI usa TOML com `{{args}}`; o Cursor usa Markdown puro e anexa o texto digitado; o Codex descontinuou os custom prompts em favor de skills.

## Decisão

- Formato canônico: frontmatter com `description` (obrigatório) e `argument-hint` (opcional), corpo com `$ARGUMENTS`. Campos desconhecidos são erro (`allowed-tools`, `model` não são portáveis).
- `CommandsAdapter` (`CommandsRoot`, `Command`) em Claude Code, Gemini CLI, Cursor e OpenCode. O arquivo gerado é do pack e segue o lock, como os subagents.
- Gemini: `$ARGUMENTS` vira `{{args}}`; a string TOML usa os escapes do JSON, que são válidos em TOML.
- Cursor: Markdown puro, com aviso quando o corpo usa placeholders.
- `$1`…`$9` geram aviso no `validate`, porque só Claude Code e OpenCode os expandem.
- Codex, Crush e Grok não instalam commands; o Codex ganha uma nota sugerindo skill.
- `atlana build` gera commands no plugin do Claude e na extensão do Gemini.

## Consequências

Um command escrito uma vez roda nos quatro agentes. O Cursor perde a posição do argumento no prompt, porque o texto vai sempre para o fim.

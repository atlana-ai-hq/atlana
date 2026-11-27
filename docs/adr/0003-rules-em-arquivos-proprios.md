# ADR 0003: Rules em arquivos próprios, sem tocar nos arquivos de entrada

- Status: aceita
- Data: 2026-11-27

## Contexto

O PRD (F5) previa gerar `CLAUDE.md` (com `@AGENTS.md`), `GEMINI.md` e `.mdc` a partir do `AGENTS.md` do pack. Só que a maioria dos projetos e HOMEs já tem esses arquivos. Como o Atlana nunca sobrescreve o que não está no lock, gerar o arquivo de entrada viraria conflito na maioria das máquinas, e dois packs com rules não poderiam dividir o mesmo agente.

## Decisão

Cada adaptador grava só arquivos próprios, dentro de uma raiz de rules (interface `adapter.RulesAdapter`):

- **Claude Code:** `.claude/rules/atlana-<pack>/`, que o Claude Code carrega sozinho.
- **Cursor:** `.cursor/rules/atlana-<pack>-<slug>.mdc` com `alwaysApply: true`, só no escopo project (as rules de usuário do Cursor ficam no app).
- **Gemini CLI:** `.gemini/atlana/atlana-<pack>/`; a CLI mostra a linha `@...` para importar no `GEMINI.md`.
- **Codex:** `<CODEX_HOME>/atlana/atlana-<pack>/`; o `AGENTS.md` do Codex não importa arquivos, então a referência é manual.

O planner ganhou ações com conteúdo gerado (`Action.Content`), e a decisão criar/atualizar/igual/conflito/remover passou para um `filePlanner` comum a skills e rules.

## Alternativas

- **Gerar os arquivos de entrada:** é simples, mas dá conflito em quase toda máquina real.
- **Bloco gerenciado** (`<!-- atlana:<pack> -->`) dentro dos arquivos da pessoa: funciona em qualquer lugar, mas é um merge em arquivo de terceiros, e o lock teria de guardar o hash de um bloco. Fica para junto do merge de config do F6.

## Consequências

No Gemini e no Codex a ativação ainda pede um passo manual. Quando o merge do F6 existir, dá para ligar a importação no `GEMINI.md` e no `AGENTS.md` automaticamente, sem mudar onde os arquivos ficam.

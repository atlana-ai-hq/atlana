# ADR 0008: promover o formato para `atlana/v1`

- Status: aceita
- Data: 2026-11-27

## Contexto

A ADR 0001 definiu que `atlana/v1alpha1` viraria `atlana/v1` quando hooks e subagents estivessem implementados em pelo menos dois agentes. Desde a ADR 0005, os dois funcionam no Claude Code e no Gemini CLI, e o `atlana build` (ADR 0007) usa o mesmo formato.

## Decisão

- `apiVersion: atlana/v1` é a versão atual, em `atlana.yaml` e em `hub.yaml`.
- O formato não muda. `atlana/v1alpha1` continua aceita, com um aviso no `validate`, para não quebrar packs existentes.
- A partir daqui, mudança incompatível só numa `atlana/v2`. Campos novos e opcionais podem entrar na v1.

## Consequências

- Packs antigos seguem instalando; basta trocar a linha para silenciar o aviso.
- O suporte a `atlana/v1alpha1` pode sair numa versão futura da CLI, anunciado no changelog.

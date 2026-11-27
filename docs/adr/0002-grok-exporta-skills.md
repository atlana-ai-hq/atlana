# ADR 0002: Grok Bot recebe skills por exportação

- Status: aceita
- Data: 2026-11-27

## Contexto

O Grok Bot é um dos alvos principais (PRD, F9), mas não lê skills de uma pasta local como o Claude Code ou o Cursor: a pessoa envia as skills pela interface dele. O v1 não terá MCP remoto hospedado nem sync automático.

## Decisão

O adaptador `grok` trata a exportação como uma instalação com outra raiz:

- `--scope user`: `$XDG_CONFIG_HOME/atlana/exports/grok/skills` (padrão `~/.config/atlana/...`).
- `--scope project`: `<projeto>/.atlana/exports/grok/skills`.
- Cada skill sai sem mudanças, no formato Agent Skills (`<skill>/SKILL.md` e arquivos), em pastas (sem `.zip`).
- O adaptador implementa a interface opcional `adapter.Exporter`; o `doctor` mostra "só exporta" e o `install` imprime onde ficaram as pastas.

## Por quê

Planner, `--dry-run`, recusa de conflito, `atlana.lock` com sha256, `status` e `uninstall` passam a valer para a exportação sem nenhuma mudança no installer. As regras de segurança (só `Apply` escreve; nada fora do lock é sobrescrito) continuam iguais.

## Alternativas

- Comando `atlana export` separado: duplicaria planner e lock, e a exportação ficaria fora do `status`.
- Gerar `.zip` por skill: exigiria um novo tipo de ação no lock; fica para quando o upload pedir.
- `--out <dir>`: fica para depois; exigiria uma terceira raiz no lock.

## Consequências

Se o Grok ganhar uma pasta local ou uma API de sync, basta mudar `SkillsRoot` (ou somar uma ação nova) no adaptador. Na máquina onde o Grok for detectado, um `install` sem `--agent` também exporta para ele.

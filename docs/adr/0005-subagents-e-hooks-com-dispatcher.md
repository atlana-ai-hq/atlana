# ADR 0005: Subagents traduzidos e hooks por um dispatcher do atlana

- Status: aceita
- Data: 2026-11-27

## Contexto

O F7 (subagents) e o F8 (hooks) do PRD precisam de formatos próprios, porque não há padrão comum. Hoje o Claude Code e o Gemini CLI têm os dois recursos: subagents em `.claude/agents` e `.gemini/agents`, e hooks no `settings.json`, onde `exit 2` bloqueia. Os nomes de eventos, de ferramentas e o formato do payload são diferentes em cada um.

## Decisão

**Subagents.** O `agents/<nome>.md` canônico tem ferramentas abstratas (`read`, `search`, `edit`, `write`, `shell`, `shell:readonly`, `web`, `mcp`) e `model: inherit | fast | smart`. Cada adaptador gera o arquivo nativo (interface `AgentsAdapter`). Ferramenta sem equivalente seguro fica de fora, com aviso. Se a lista inteira some, o subagent não é instalado, porque nos agentes uma lista vazia dá acesso a todas as ferramentas.

**Hooks.**
- Os scripts são copiados para uma pasta do atlana por agente (`…/atlana/hooks/<agente>/<pack>/<hook>/`) e registrados no lock como arquivos.
- Na config do agente entra **um item por evento**, que chama `atlana hook dispatch --agent <id> --event <evento> --dir <pasta>`. O item é editado com o mesmo mecanismo de `keys` do ADR 0004, mas dentro de um array (`configmerge.ParseArrayItems`), e é reconhecido pelo comando. Packs que usam o mesmo evento dividem o item, e o uninstall só o tira quando ninguém mais usa.
- O dispatcher (`internal/hookrun`) normaliza o payload, aplica `match` (ferramenta e glob de caminho), roda cada script com timeout e repassa `exit 2` com o stderr. Outros erros só viram aviso, para um hook quebrado não travar o agente.
- **Nunca em silêncio:** hooks só entram com `--only hooks`, e o plano lista o que cada hook roda.
- **Fallback** para agentes sem hooks: `rule` instala `rule.md` como rule (ADR 0003), `skip` avisa, `fail` vira conflito.

**Plano encadeado.** O Gemini guarda MCP e hooks no mesmo `settings.json`. O planner abre a config a partir do conteúdo pendente das ações anteriores, e o `Apply` grava a versão mais completa, para um merge não apagar o outro.

## Alternativas

- **Um item de hook por hook do pack**, com o script direto na config. Não tem a normalização do payload, então cada script teria de entender o formato de cada agente, e o `match` canônico não funcionaria.
- **Shim em shell** em vez do binário. Precisaria de `jq` ou similar para ler o JSON, e duplicaria a lógica de match por agente.

## Consequências

- O caminho do binário fica gravado na config do agente. Se ele mudar, reinstalar atualiza o item. `go run` gera um binário temporário, e o plano avisa.
- `fast` e `smart` no Gemini ainda não têm alias estável, então usam o modelo da sessão (com aviso). No Claude viram `haiku` e `opus`.
- Com hooks e subagents implementados em dois agentes, a condição do ADR 0001 para `apiVersion: atlana/v1` foi atingida. A promoção é uma mudança de formato e fica para uma decisão à parte.
- Cursor e Codex ganham hooks e subagents nativos em fatias futuras. Até lá usam o fallback.

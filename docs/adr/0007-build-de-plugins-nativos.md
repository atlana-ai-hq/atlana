# ADR 0007: `atlana build` gera plugins nativos reaproveitando os adaptadores

- Status: aceita
- Data: 2026-11-27

## Contexto

O F12 do PRD pede plugins nativos para quem não quer instalar a CLI: `claude-plugin`, `cursor-plugin`, `gemini-extension` e o padrão aberto `agent-plugin`. Cada formato aceita módulos diferentes e tem uma variável própria para a raiz do plugin.

## Decisão

- **A mesma tradução do `install`.** `internal/build` chama os adaptadores (`Subagent`, `MCPServer`, `HookEntry`, `CursorRule`) com a variável de raiz de cada formato (`${CLAUDE_PLUGIN_ROOT}`, `${extensionPath}`). Assim um pack instalado e um pack empacotado dão o mesmo resultado.
- **Calcula em memória, grava no fim.** `Build` devolve a lista de arquivos e os avisos; só `Write` grava, numa pasta temporária renomeada no fim. Ele só substitui um diretório que tenha o marcador `.atlana-build.json`.
- **Cada formato só leva o que suporta.** O resto vira aviso, nunca uma aproximação silenciosa:
  - O plugin do Claude não leva rules.
  - O plugin do Cursor não leva subagents, e os hooks usam o `fallback`.
  - O `agent-plugin` segue a spec 1.0.0 à risca: campos fechados e `$schema` exatos. Servidores MCP que dependem de `${VAR}` ficam de fora, porque a spec não expande variáveis de ambiente e proíbe segredos.
- **Hooks continuam opt-in** (`--only hooks`) e chamam `atlana hook dispatch` do PATH.
- **README gerado** quando o pack não tem um, porque diretórios de plugins pedem README para listar e revisar.

## Alternativas

- **Script de hook direto, sem dispatcher:** o plugin não dependeria do atlana, mas o script receberia o payload nativo de cada agente, e o `match` canônico deixaria de valer.
- **Um gerador por formato, sem os adaptadores:** mais livre, mas duplicaria as tabelas de ferramentas, eventos e MCP, que já mudam rápido.

## Consequências

- `claude plugin validate` aprova o plugin do Claude gerado a partir de `templates/pack`, inclusive com `--profile directory`.
- O formato do `.cursor-plugin/plugin.json` e as variáveis de plugin do Cursor ainda não estão totalmente documentados. Por isso, servidores que usam `${PLUGIN_ROOT}` não vão para o plugin do Cursor.
- Publicar em marketplaces (o `marketplace.json` do Claude, a galeria de extensões do Gemini) fica para depois.

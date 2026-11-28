# ADR 0010: MCP e subagents no OpenCode, na forma que o binário normaliza

- Status: aceita
- Data: 2026-11-27
- Complementa a ADR 0009

## Contexto

A ADR 0009 deixou MCP e subagents do OpenCode de fora porque a documentação descreve dois formatos (V1 e V2). Testando o próprio binário (OpenCode 1.18.31, `opencode debug config`), vimos:

- **MCP:** o binário lê `mcp.<nome>` (V1) e `mcp.servers.<nome>` (V2), e normaliza os dois para `mcp.<nome>`. `{env:VAR}` é expandido.
- **Subagents:** lê `agents/` e `agent/`, e um `tools` antigo vira um mapa `permission` (`edit`, `bash`, `webfetch` com `allow | ask | deny`).

## Decisão

- **MCP:** merge na seção `mcp` de `opencode.json`, ou de `opencode.jsonc` se esse já existir. A propriedade de cada servidor fica no lock, como na ADR 0004.
  - stdio vira `type: local`, com `command` em array e `environment`.
  - HTTP e SSE viram `type: remote`, com `url` e `headers`.
  - `${VAR}` vira `{env:VAR}`.
  - `cwd` é recusado, porque não tem equivalente documentado.
- **Subagents:** `agents/<nome>.md` com `mode: subagent`, `description` e `permission`.
  - **edit / write:** `edit` é `allow` se `edit` ou `write` foi pedido.
  - **shell:** `bash` é `allow` com `shell`, `ask` com `shell:readonly` (com aviso), e `deny` sem nenhum dos dois.
  - **web:** `webfetch` é `allow` com `web`.
  - **sem tools:** sem `permission`, ou seja, tudo liberado, como nos outros agentes.
  - **model:** `fast` e `smart` usam o modelo da sessão, com aviso.

## Consequências

- **Limites que geram aviso:**
  - Leitura e busca ficam sempre liberadas.
  - Ferramentas MCP não são restringidas por subagent.
  - `edit` e `write` são uma permissão só no OpenCode.
- Um `opencode.jsonc` com comentários é recusado pelo merge (conflito), sem tocar no arquivo.
- Rules (o `instructions` do OpenCode) e hooks (só plugins JS) ficam para depois.

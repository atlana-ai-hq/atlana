# ADR 0004: MCP por merge, com a posse de cada servidor no lock

- Status: aceita
- Data: 2026-11-27

## Contexto

Servidores MCP ficam dentro de configs que são da pessoa e do próprio agente (`~/.claude.json`, `settings.json`, `config.toml`). Substituir o arquivo apagaria o resto. O PRD (F6) sugeria marcar os servidores com um campo `_atlana`. O formato muda por agente (JSON ou TOML, `${VAR}` ou `${env:VAR}`, `url` ou `httpUrl`), e o Claude Code reescreve o `~/.claude.json` o tempo todo.

## Decisão

- **Merge servidor a servidor** (pacote `internal/configmerge`). No JSON, as chaves de topo mantêm ordem e valor. No TOML, o atlana só edita tabelas `[mcp_servers.<nome>]` e as subtabelas delas.
- **Posse no lock, não no arquivo.** `atlana.lock` ganha `keys` (caminho, nome, `sha256` do valor normalizado). Nada de `_atlana` na config, porque agentes com schema estrito podem recusar campos desconhecidos. As regras de sempre continuam: só atualiza ou remove o servidor se ele é nosso e o hash bate; senão é conflito ou aviso.
- **Proteção contra plano velho.** Cada ação guarda o hash do arquivo no momento do plano, e `Apply` recusa gravar (`ErrStale`) se o arquivo mudou nesse intervalo.
- **Sem segredos.** `validate` só aceita `${NOME}` em `env` e `headers` (e `Bearer ${NOME}` em headers). Cada adaptador traduz a referência para a sintaxe do agente; o Codex, que não expande variáveis, usa `env_vars` e `bearer_token_env_var`.
- **Sem dependência nova.** O TOML é editado por blocos de texto, não por um parser completo. Formas que o editor não entende (tabela `[mcp_servers]`, inline, chave com ponto) viram conflito, nunca uma chave duplicada.

## Alternativas

- **Marcador `_atlana` no arquivo:** fica visível para quem lê a config, mas pode quebrar validação de schema dos agentes e polui o arquivo da pessoa.
- **Biblioteca TOML** (ex.: `pelletier/go-toml`): faria o parse completo, mas reescreveria o arquivo inteiro (perdendo comentários e formatação) e seria uma quarta dependência.

## Consequências

- O JSON editado é reindentado com 2 espaços; o conteúdo e a ordem das chaves de topo se mantêm.
- Desligar tools perigosas por padrão (PRD F6) fica para depois: não há um campo comum entre agentes. Entra por adaptador quando houver caso de uso.
- O mesmo mecanismo de `keys` serve para ligar `@import` no `GEMINI.md` (ADR 0003) e para hooks (F8).

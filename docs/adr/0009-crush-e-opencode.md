# ADR 0009: Crush e OpenCode começam só com skills

- Status: aceita
- Data: 2026-11-27

## Contexto

A CLI antiga (Bash) atendia o Crush e o OpenCode, e os packs pessoais migrados precisam instalar neles. Os dois leem Agent Skills, mas o resto da configuração está mudando:

- **Crush** (v0.98): a config passou a ser o `crushrc`, um script de shell, e o `crush.json` está obsoleto. A doc ainda chama os hooks de "preliminares".
- **OpenCode**: os formatos V1 e V2 diferem justamente no que o Atlana precisa gravar. Os servidores MCP vão em `mcp.<nome>` ou em `mcp.servers.<nome>`, e os agents usam um mapa `tools` ou uma lista `permissions`. Plugins em JS fazem o papel dos hooks.

## Decisão

- Os dois adaptadores instalam só **skills**, nos caminhos que as duas versões documentam:
  - **Crush:** `$CRUSH_SKILLS_DIR` ou `$XDG_CONFIG_HOME/crush/skills` (padrão `~/.config/...`), e `.crush/skills` no projeto.
  - **OpenCode:** `$XDG_CONFIG_HOME/opencode/skills` e `.opencode/skills`.
- **Detecção:** pelo binário ou pelos diretórios de config. No OpenCode, `~/.opencode` (do instalador oficial) também conta.
- **O que fica de fora:** rules, MCP, subagents e hooks são recusados com a mensagem de módulo não suportado. O Atlana não edita o script de shell da pessoa, nem uma config cujo formato depende da versão instalada.

## Consequências

- Lock, `status` e `uninstall` funcionam como nos outros agentes.
- Próximos passos:
  - **OpenCode:** detectar a versão do formato e gravar MCP e agents no formato certo, por merge com dono no lock, como na ADR 0004.
  - **Crush:** linhas de `crushrc` geradas para colar, ou um arquivo incluído, se o Crush passar a aceitar `source` de arquivo próprio.

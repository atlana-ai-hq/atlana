# ADR 0001: Decisões iniciais da CLI Atlana

- Status: aceita
- Data: 2026-11-24

## Contexto

A primeira versão do Atlana era um script Bash 3.2 de ~1.000 linhas, com os agentes fixos em blocos `case`, conteúdo pessoal e público no mesmo repositório, sem testes, sem forma confiável de desinstalar e com hooks que eram copiados mas nunca ligados. Cada agente (Cursor, Claude Code, Codex, Gemini CLI…) guarda skills, hooks e MCP em lugares e formatos diferentes, e esses formatos mudam rápido.

Este repositório recomeça o projeto com quatro decisões.

## Decisões

### 1. CLI em Go

**Escolha:** Go, com a biblioteca padrão para a CLI (`flag`) e só três dependências (`yaml.v3`, `jsonschema/v6`, `x/text`).

**Por quê:**
- Binário único, sem runtime: fácil distribuir por `go install`, releases e `brew tap`, inclusive para quem não usa Node.
- Cross-compile trivial para macOS, Linux e Windows (os agentes rodam nos três).
- Biblioteca padrão forte para o que a CLI faz: arquivos, hashes, JSON, embed do schema.
- `flag` em vez de cobra: a CLI tem poucos comandos; menos dependências, menos superfície.

**Alternativa considerada:** TypeScript/Node (merge de JSON e schemas mais fáceis, publicação no npm). Rejeitada por exigir Node instalado e pela distribuição mais pesada. O merge de JSON/TOML terá testes próprios quando o módulo MCP entrar.

### 2. Padrões abertos primeiro

**Escolha:** usar sem mudanças o que já tem padrão (Agent Skills `SKILL.md`, `AGENTS.md`, `mcp.json` de Agent Plugins) e só definir formato próprio para o que não tem (hooks com eventos genéricos, subagents, commands), num `atlana.yaml` pequeno por pack.

**Por quê:** um pack Atlana continua útil sem a CLI (as skills funcionam em qualquer ferramenta compatível), evita lock-in e reduz o que precisamos manter. O schema é JSON Schema 2020-12 e fica embutido no binário, então o publicado e o validado são o mesmo arquivo.

**Consequência:** `apiVersion: atlana/v1alpha1` sinaliza que o formato próprio ainda pode mudar. Vira `atlana/v1` quando hooks e subagents estiverem implementados em pelo menos dois agentes.

### 3. Lockfile

**Escolha:** `atlana.lock` em JSON (`~/.config/atlana/` no escopo user, raiz do projeto no escopo project) com pack, versão, origem, agente, módulo e `sha256` de cada arquivo gerado.

**Por quê:** é o que permite `status` (ok/editado/ausente) e `uninstall` exatos. Com o hash, o Atlana sabe o que é dele **e** se alguém editou depois: só atualiza ou remove arquivos próprios e intactos; o resto vira conflito ou aviso.

**Alternativas:** marcar arquivos com comentários (não funciona em todo formato e polui o conteúdo) ou varrer diretórios comparando com o pack (não distingue arquivo do usuário de arquivo nosso).

### 4. Plano antes de aplicar

**Escolha:** `install` e `uninstall` primeiro calculam um plano puro (criar, atualizar, igual, remover, conflitos, notas) sem tocar no disco, imprimem e só então aplicam: com confirmação no terminal, `--yes` em CI, ou nada com `--dry-run`. Conflito bloqueia o apply inteiro.

**Por quê:** a CLI escreve em diretórios de configuração de outras ferramentas; a pessoa precisa ver exatamente o que vai mudar. Separar plano de execução também deixa os adaptadores fáceis de testar (o plano é dado, não efeito colateral) e prepara o `diff`/`update`.

## Consequências

- Adicionar um agente = um arquivo em `internal/adapter/` + testes com HOME falso.
- Os próximos módulos (rules, MCP, hooks) entram como novos tipos de ação no mesmo planner (ex.: merge de JSON com marcador de propriedade), sem mudar o fluxo plano → apply → lock.
- Escrita atômica (arquivo temporário + rename) em arquivos e no lock.

# Como criar um pack Atlana

Um pack é uma pasta com um `atlana.yaml` e o conteúdo de cada módulo. Você escreve uma vez e o `atlana` instala em cada agente, no formato dele. O Atlana não traz skills prontas: o conteúdo é seu.

## 1. Comece pelo esqueleto

```bash
cp -R templates/pack ~/meus-packs/meu-pack
cd ~/meus-packs/meu-pack
atlana validate .
```

O esqueleto tem um exemplo de cada módulo. Apague o que não for usar, tirando a pasta **e** a entrada em `modules`.

## 2. `atlana.yaml`

| Campo | O que é |
|---|---|
| `name` | nome do pack: minúsculas, números e hífens |
| `version` | SemVer; suba a cada mudança que você distribuir |
| `visibility` | `public` ou `private`. `atlana scan` acusa pack `private` num repo público |
| `modules` | o que o pack oferece; só o que está listado é instalado |
| `requires.env` | **nomes** das variáveis que o `mcp.json` usa. Nunca os valores |
| `targets` | `include`/`exclude` de agentes, ex.: `exclude: [grok]` |

A spec completa está em [`spec/README.md`](../spec/README.md).

## 3. Módulos

**Skills** (`skills/<nome>/SKILL.md`): padrão [Agent Skills](https://agentskills.io), sem extensões. O `name` do frontmatter é igual ao nome da pasta. A `description` diz *quando* usar a skill, porque é ela que o agente lê para decidir. Arquivos de apoio (`references/`, scripts) ficam na mesma pasta.

**Rules** (`AGENTS.md` ou outros `.md`): instruções gerais. O Atlana grava em arquivos próprios e nunca mexe no seu `CLAUDE.md`, `GEMINI.md` ou `AGENTS.md`. No Gemini e no Codex, a CLI diz como ativar.

**Subagents** (`agents/<nome>.md`): use as ferramentas abstratas (`read`, `search`, `edit`, `write`, `shell`, `shell:readonly`, `web`, `mcp`) e `model: inherit | fast | smart`. Uma ferramenta sem equivalente seguro no agente fica de fora, com aviso.

**Hooks** (`hooks/<nome>/hook.yaml` + script): o script recebe um JSON normalizado no stdin, e `exit 2` nega a ação. Defina `fallback` para agentes sem hooks (`rule` usa o `rule.md` ao lado). Hooks só entram com `--only hooks`. Lembre a quem for usar o pack que eles rodam comandos na máquina.

**MCP** (`mcp.json`, formato Agent Plugins): em `env` e `headers` use só `${NOME}` (ou `Bearer ${NOME}`) e liste o nome em `requires.env`. Valor literal é erro no `validate`. `${PLUGIN_ROOT}` é o diretório do pack.

## 4. Teste antes de publicar

```bash
atlana validate .
atlana install . --agent claude --scope project --dry-run   # veja o plano
atlana install . --agent claude --scope project --yes
atlana status
atlana uninstall meu-pack --scope project --yes
```

## 5. Publique

Coloque o pack num repositório git (um repo pode ter vários packs em subpastas). Quem tiver acesso instala direto:

```bash
atlana install github:seu-usuario/seus-packs//meu-pack@v0.1.0
```

Repo privado? Funciona com o seu `gh auth login` ou SSH, sem token em arquivo. Para nomes curtos, use `atlana registry add`.

Antes de tornar um repo público, rode `atlana scan` (ver [release.md](release.md#nomes-privados)).

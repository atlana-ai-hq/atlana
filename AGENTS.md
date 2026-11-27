# AGENTS.md

Instruções para agentes de código (e pessoas) que editam este repositório.

## O que é

CLI em Go (`cmd/atlana`) que valida packs no formato Atlana e os instala em agentes de IA. A spec fica em `spec/`; o schema `spec/schemas/atlana.schema.json` é embutido na CLI via `spec/spec.go`.

## Mapa

- `cmd/atlana/`: `main`, só monta o `cli.App` com o ambiente real.
- `internal/cli/`: comandos (biblioteca padrão `flag`, sem framework).
- `internal/manifest/`: carrega `atlana.yaml`, valida contra o schema e valida `SKILL.md`.
- `internal/adapter/`: um adaptador por agente (`Detect`, `Supports`, `SkillsRoot`). Tudo que vem do sistema passa por `adapter.Env`.
- `internal/installer/`: planner (`PlanInstall`, `PlanUninstall`), `Apply` e `Status`.
- `internal/lockfile/`: formato e leitura/escrita do `atlana.lock`.
- `internal/configmerge/`: merge não destrutivo de configs de agentes (JSON, TOML e itens de array).
- `internal/hookrun/`: dispatcher que os agentes chamam nos hooks (`atlana hook dispatch`).
- `internal/source/`: origens `github:owner/repo//caminho@ref`, buscadas com o git da pessoa para um cache.
- `internal/registry/`: registry local do Hub (`hub.yaml`).
- `internal/scan/`: `atlana scan`, nomes privados (lista local, nunca no repo) e packs `private`.
- `templates/pack/`: esqueleto de pack com todos os módulos; precisa passar em `atlana validate`.
- `docs/`: guias de autoria, migração e release (`docs/autoria.md`, `docs/migracao.md`, `docs/release.md`).
- `packs/`: packs de exemplo; todos precisam passar em `atlana validate`.
- `docs/adr/`: decisões de arquitetura.

## Comandos

```bash
gofmt -l .                       # precisa sair vazio
go vet ./...
go test -race ./...
go run ./cmd/atlana validate packs
go run ./cmd/atlana validate templates
go run ./cmd/atlana scan .         # nomes privados de ~/.config/atlana/private-names.txt
```

## Regras

- Calcular o plano **nunca** escreve no disco. Só `installer.Apply` escreve. (Buscar uma origem remota escreve só no cache `~/.cache/atlana`, antes do plano.)
- Nunca sobrescrever ou apagar arquivo que não está no lock com o mesmo hash.
- Testes não podem tocar no HOME real: use `adapter.Env` com `t.TempDir()` e `LookPath` falso.
- Mudou o formato? Atualize juntos `spec/README.md`, o schema, os testes e, se for decisão relevante, um ADR novo.
- Novo agente: crie o adaptador em `internal/adapter/`, registre em `All()`, cubra `Detect` e caminhos nos testes e atualize a tabela de suporte do README.
- Dependências novas só com justificativa no PR. Hoje são três: `yaml.v3`, `jsonschema/v6` e `x/text`.
- Packs deste repo são públicos e MIT: só conteúdo autoral ou com licença compatível e origem registrada. Nada de segredos, nomes internos de empresas ou conteúdo pessoal.
- Mensagens de commit no padrão Conventional Commits.

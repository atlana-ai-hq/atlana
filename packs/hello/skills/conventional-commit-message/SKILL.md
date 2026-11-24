---
name: conventional-commit-message
description: Escreve mensagens de commit no padrão Conventional Commits a partir das mudanças em stage. Use quando a pessoa pedir para commitar, sugerir ou revisar uma mensagem de commit.
license: MIT
metadata:
  pack: hello
  version: "0.1.0"
---

# Conventional Commit Message

Escreva a mensagem de commit seguindo [Conventional Commits 1.0](https://www.conventionalcommits.org/pt-br/v1.0.0/).

## Passos

1. Leia o que vai entrar no commit (`git diff --staged`). Se não houver nada em stage, avise e pare.
2. Escolha **um** tipo:
   - `feat`: funcionalidade nova para quem usa
   - `fix`: correção de bug
   - `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`: o resto
3. Escolha um escopo curto e opcional, com o nome do módulo ou pasta principal (`feat(cli): ...`).
4. Escreva o resumo:
   - no imperativo ("adiciona", "corrige"), sem ponto final;
   - com no máximo 72 caracteres na primeira linha;
   - no idioma que o repositório já usa nos commits.
5. Se o motivo não for óbvio pelo diff, acrescente um corpo depois de uma linha em branco explicando **por quê**, não o quê.
6. Mudança que quebra compatibilidade: use `!` depois do tipo (`feat(api)!: ...`) e um rodapé `BREAKING CHANGE: <o que muda para quem usa>`.

## Exemplos

```text
feat(install): adiciona --dry-run ao comando install
```

```text
fix(lock): não remove arquivos editados pelo usuário no uninstall

O uninstall comparava só o caminho. Agora compara o sha256 registrado
no atlana.lock e mantém o arquivo se ele foi alterado.
```

## Não faça

- Não misture mudanças sem relação num commit só; sugira dividir.
- Não invente issue, ticket ou coautor que não esteja no contexto.

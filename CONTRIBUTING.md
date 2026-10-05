# Como contribuir com o Vitral

Obrigado por querer ajudar. Este guia é curto; o resto está em [`docs/`](docs/).

## Preparar

- Go (a versão do [`go.mod`](go.mod)) e `make`.
- O Chrome for Testing dos testes é baixado sozinho na primeira vez que você roda `make e2e` ou
  `make check` (cerca de 150 MB, em `~/Library/Caches/vitral-e2e` no Mac e `~/.cache/vitral-e2e` no Linux).
- Ative o gate rápido antes de cada push: `git config core.hooksPath .githooks`.

## O gate

```bash
make check-fast    # lint + unitários + cobertura (o mesmo do pre-push)
make check         # o gate do PR: tudo acima + integração + e2e + mutação + tempos e tokens
```

Abra o PR só com o `make check` verde. Os números do gate (cobertura, mutação, tokens, tempos) estão em
[`.quality/thresholds.yaml`](.quality/thresholds.yaml): eles só sobem, e qualquer mudança precisa de
justificativa no próprio arquivo. A estratégia completa está em [`docs/TESTING.md`](docs/TESTING.md).

## Regras do projeto

- **Mensagens para pessoas em português do Brasil** (CLI, selo, README), curtas e sem jargão. **Erros e
  descrições para a IA em inglês**, imperativos e acionáveis. Veja o tom em [`docs/BRAND.md`](docs/BRAND.md).
- **Nada de evasão:** o Vitral não disfarça automação, não resolve nem contorna CAPTCHA ou verificação, e
  não retoca sites para contornar paywall. Senhas e códigos são digitados pela pessoa.
- **Sem LLM dentro do Vitral.** A IA fica do lado de fora, no Copilot.
- **Privacidade:** o mapa de telas guarda estrutura, nunca dados; sessões salvas ficam com permissão 0600 e
  nunca vão para a IA nem para o repositório.
- **Decisões com medição:** quando houver dúvida entre dois caminhos, meça antes (veja os testes com tag
  `diag`, `sessiondiag` e `retouchdiag`) e registre a decisão no documento da feature.
- **Lógica pura fora do navegador:** o que dá para testar sem Chrome vai para um pacote puro, com
  unitários, propriedades e, se fizer sentido, fuzz.

## Onde está cada coisa

O mapa dos pacotes e o diagrama estão em [`docs/DESENVOLVIMENTO.md`](docs/DESENVOLVIMENTO.md). As
especificações de cada parte estão em [`docs/`](docs/), e o roteiro de validação manual (o que só dá
para conferir com a janela aberta) em [`examples/vscode/CHECKLIST.md`](examples/vscode/CHECKLIST.md).

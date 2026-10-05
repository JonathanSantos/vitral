# Segurança

## Como avisar sobre uma falha

Não abra uma issue pública. Use o [aviso privado de vulnerabilidade do GitHub](https://github.com/JonathanSantos/vitral/security/advisories/new)
deste repositório, com o que você fez, o que esperava e o que aconteceu.

## O que o Vitral guarda e onde

| O quê | Onde | Proteção |
|---|---|---|
| Logins do navegador (cookies, senhas que o Chrome salvar) | `~/.vitral/profiles/<perfil>` | cofre do sistema (Keychain, DPAPI, libsecret) |
| Sessões salvas (`session_save`) | `~/.vitral/sessions/<nome>.json` | permissão 0600; o conteúdo nunca vai para a IA |
| Mapa de telas | `~/.vitral/maps/<site>.json` | 0600; só estrutura (papéis, nomes de botões, títulos), nunca dados |
| Retoques | `~/.vitral/retoques/<nome>/` | 0600; só CSS; o save recusa CSS que busca em outro host |
| Fluxos | `.vitral/flows/` no projeto | YAML legível; valores que mudam entram como parâmetros |

## O que está dentro do escopo

- Qualquer forma de a página controlada alcançar o Vitral (o selo, as refs, a ligação com o Go).
- Algo que faça a IA digitar uma senha ou um código, ou passar por um passo sensível sem confirmação.
- Dados da página indo parar no mapa de telas, nos logs ou na resposta das ferramentas sem a IA pedir.
- Retoques que consigam mandar dados para fora.

O Vitral é feito para ser identificável como automação: o navegador da IA se apresenta como
automatizado de propósito, e isso não é falha.

# h2flood

**Categoria:** Volumétrico / HTTP/2 multiplexing

## O que faz

Abre um número pequeno de conexões HTTP/2 e dispara muitos streams concorrentes dentro de cada conexão. HTTP/2 permite múltiplas requisições simultâneas sobre uma única conexão TCP — o ataque explora isso para gerar alta pressão com poucos sockets.

É a base do ataque conhecido como **HTTP/2 Rapid Reset** (CVE-2023-44487): abrir streams e cancelá-los rapidamente para saturar o servidor antes que o rate limiting por conexão consiga agir.

## O que testa

- Se o servidor tem limites em `MaxConcurrentStreams` por conexão HTTP/2
- Se o rate limiting opera por conexão TCP (bypassável via streams) ou por IP/requisição
- Comportamento do servidor sob pressão de streams paralelos sobre conexões persistentes
- TLS e negociação ALPN (HTTP/2 requer HTTPS em produção)

## Quando usar

Quando o alvo usa HTTP/2 (HTTPS com ALPN). Servidores que limitam por conexão TCP podem ser bypassados via multiplexing. Útil para testar a configuração de `MaxConcurrentStreams` do servidor.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `h2flood` no menu, preencha os campos e execute — sem decorar parâmetros.

## Uso

```bash
# Básico (requer HTTPS para h2 via ALPN)
./limithit h2flood https://localhost:8443/api/ping --total 200

# Mais streams por conexão
./limithit h2flood https://localhost:8443/api/ping --connections 2 --streams 500 --total 1000

# Pular verificação TLS (certificado autoassinado)
./limithit h2flood https://localhost:8443/api/ping --insecure --total 200
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--connections` | `1` | Número de conexões TCP/HTTP/2 |
| `--streams` | `100` | Streams concorrentes por conexão |
| `--method` | `GET` | Método HTTP |
| `--insecure` | `false` | Pular verificação de certificado TLS |
| `--total` | `100` | Total de requisições |
| `--concurrency` | `10` | Workers (sobrescrito por `streams × connections`) |

## Concorrência efetiva

O ataque usa `streams × connections` como concorrência real, ignorando `--concurrency` quando o produto for maior. Com 2 conexões e 500 streams: 1000 requisições paralelas sobre 2 sockets TCP.

## Lendo o resultado

- **Rate limiting funciona**: `429` aparece mesmo com streams paralelos
- **Sem limitação**: servidor processa todos os streams sem 429 — rate limiting é por conexão, não por requisição
- **Erros de handshake**: servidor pode não suportar h2 no endpoint testado

## Nota sobre HTTP/2 sem TLS (h2c)

HTTP/2 cleartext (h2c) não é suportado diretamente. Para testar h2c, use um proxy ou certifique-se de que o servidor negocia via upgrade HTTP/1.1.

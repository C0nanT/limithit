# ratelash

CLI em Go para testes de **rate-limit HTTP**. Dispara N requisições em paralelo via worker pool e imprime apenas o resumo final.

Sem dependências externas — stdlib Go apenas.

## Requisitos

- Go 1.22+

## Instalação

```bash
git clone https://github.com/conantorreswf/ratelash.git
cd ratelash
go build -o ratelash .
```

Ou rodar sem compilar:

```bash
go run . --url http://localhost:8080/api/ping --total 100
```

## Flags

| Flag | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `--url` | string | — | URL alvo (**obrigatório**) |
| `--total` | int | 100 | Total de requisições |
| `--concurrency` | int | 10 | Workers simultâneos |
| `--method` | string | `GET` | GET, POST, PUT, PATCH, DELETE, HEAD |
| `--timeout` | int | 10 | Timeout por request (segundos) |
| `--body` | string | "" | Corpo da requisição |
| `--header` | string | — | Header `"Chave: Valor"` (pode repetir) |

`Ctrl+C` cancela e imprime o resumo com o total real enviado.

## testserver

Servidor alvo local com **dashboard visual em tempo real**. Ideal para testar o ratelash sem depender de serviços externos.

**Terminal 1 — subir o servidor:**

```bash
cd testserver

go run .                        # sem rate limit
go run . --rate 50 --burst 50   # limita a 50 req/s
go run . --port 9090            # porta diferente (default: 8080)
```

**Abrir o dashboard:** `http://localhost:8080`

O dashboard mostra em tempo real: total de GETs/POSTs, 429s recebidos, latência média, gráfico de req/s (últimos 60s), histograma de latência e log das últimas 20 requisições.

**Endpoints disponíveis:**

| Método | URL | Descrição |
|--------|-----|-----------|
| GET | `http://localhost:8080/api/ping` | Retorna `{"pong":true}` |
| POST | `http://localhost:8080/api/echo` | Ecoa o body JSON enviado |
| GET | `http://localhost:8080/` | Dashboard web |

## Exemplos com testserver

**Terminal 2 — rodar o ratelash:**

```bash
# GET simples — 1000 requisições, 50 workers
./ratelash \
  --url http://localhost:8080/api/ping \
  --total 1000 \
  --concurrency 50

# POST com body JSON
./ratelash \
  --url http://localhost:8080/api/echo \
  --method POST \
  --body '{"user":"alice","test":true}' \
  --header 'Content-Type: application/json' \
  --total 500 \
  --concurrency 20

# Testar rate limit (subir testserver com --rate 50 primeiro)
./ratelash \
  --url http://localhost:8080/api/ping \
  --total 2000 \
  --concurrency 100
```

No terceiro cenário, a saída do ratelash vai mostrar 429s — e o dashboard vai refletir os mesmos números em tempo real.

## Saída

```
=== ratelash summary ===
Sent:         2000
Success(2xx): 312
Client(4xx):  1688   (429: 1688)
Server(5xx):  0
Errors:       0   (timeouts: 0)
Duration:     3.841s
RPS:          520.68

Status distribution:
  200: 312
  429: 1688
```

## Estrutura

```
ratelash/
├── go.mod
├── main.go
├── internal/
│   ├── client/    # cliente HTTP + execução de request
│   ├── worker/    # worker pool com buffered channel
│   └── metrics/   # collector thread-safe + relatório final
└── testserver/    # servidor alvo + dashboard
```

---

> Use apenas contra APIs que você possui ou tem autorização explícita para testar.

# spoof

**Categoria:** Bypass de rate limiting / IP spoofing via headers

## O que faz

Rotaciona IPs falsos no header `X-Forwarded-For` (e `X-Real-IP`) a cada requisição, ciclando por um pool de endereços definido. Se o servidor usa esses headers para identificar o cliente ao fazer rate limiting, cada requisição parece vir de um IP diferente — bypassando limites por IP.

O pool de IPs pode ser um CIDR, um arquivo de texto ou uma lista de endereços.

## O que testa

- Se o rate limiting usa `X-Forwarded-For` como fonte de IP confiável sem validar a origem
- Se o servidor confia cegamente em headers de proxy mesmo sem estar atrás de um proxy confiável
- Efetividade do mecanismo `--trust-xff-cidr` do servidor: só deve confiar em XFF de IPs internos conhecidos

## Condição de bypass

O spoof só bypassa o rate limit quando o servidor está configurado para confiar em XFF do IP real do atacante. No `testserver`, isso é controlado por `--trust-xff-cidr`. Sem isso, todo o tráfego é identificado pelo `RemoteAddr` real e cai no mesmo bucket de rate limit.

## Quando usar

Para testar se o servidor valida corretamente a origem do `X-Forwarded-For` antes de usá-lo como IP do cliente. Em produção, servidores atrás de load balancers precisam confiar em XFF — mas só do IP do próprio load balancer, não de qualquer header enviado pelo cliente.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `spoof` no menu, preencha os campos (incluindo o pool de IPs) e execute — sem decorar parâmetros.

## Uso

```bash
# CIDR como pool
./limithit spoof http://localhost:8080/api/ping --ip-pool 10.0.0.0/28 --total 200

# Lista de IPs
./limithit spoof http://localhost:8080/api/ping --ip-pool "1.2.3.4,5.6.7.8,9.10.11.12" --total 100

# Arquivo de IPs
./limithit spoof http://localhost:8080/api/ping --ip-pool file:ips.txt --total 500

# Com pacing para simular tráfego distribuído
./limithit spoof http://localhost:8080/api/ping --ip-pool 10.0.0.0/24 --pacing poisson --rps 20 --total 1000

# Header alternativo (alguns servidores usam X-Real-IP diretamente)
./limithit spoof http://localhost:8080/api/ping --ip-pool 10.0.0.0/24 --xff-header X-Real-IP --total 200
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--ip-pool` | _(obrigatório)_ | `CIDR`, `file:path.txt` ou `ip1,ip2,...` |
| `--xff-header` | `X-Forwarded-For` | Header para injetar o IP spoofado |
| `--pacing` | `none` | `none`, `uniform`, `poisson`, `zipf` |
| `--rps` | `50` | Target requests/segundo (pacing `poisson`) |
| `--min-delay-ms` | `0` | Delay mínimo entre requisições (ms) |
| `--max-delay-ms` | `50` | Delay máximo entre requisições (ms) |
| `--method` | `GET` | Método HTTP |
| `--total` | `1000` | Total de requisições |
| `--concurrency` | `20` | Workers paralelos |

## Pacing disponível

| Modo | Comportamento |
|------|---------------|
| `none` | Sem delay, máxima velocidade |
| `uniform` | Delay aleatório entre min e max ms |
| `poisson` | Chegadas a taxa média de `--rps` (mais realista) |
| `zipf` | Bursts: alguns IPs fazem muito mais requests |

## Lendo o resultado

- **Com `--trust-xff-cidr` no testserver incluindo seu IP**: cada IP do pool tem bucket próprio → 429 só depois de N reqs por IP
- **Sem `--trust-xff-cidr`**: XFF ignorado → todos os reqs caem no mesmo bucket pelo IP real → 429 normal

## Formatos do arquivo de IPs

```
# Um IP por linha
192.168.1.1
10.0.0.100
172.16.5.23
```

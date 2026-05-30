# fuzz

**Categoria:** Enumeração / Descoberta de caminhos

## O que faz

Itera sobre uma wordlist de caminhos HTTP e faz GET em cada um contra o host alvo. Opcionalmente adiciona um parâmetro aleatório `_cb=<hex>` em cada URL para contornar caches — útil quando o servidor ou um CDN retorna respostas cacheadas.

O fuzz vem com uma wordlist embutida de caminhos comuns. Você pode substituí-la por qualquer arquivo de texto com um caminho por linha.

## O que testa

- Rotas não documentadas ou esquecidas que ainda respondem
- Endpoints que escapam do rate limiting por estarem em caminhos diferentes
- Diferenças de comportamento entre caminhos (200, 403, 404, 429)
- Efetividade de caches: com `--cache-bust`, cada requisição é única

## Quando usar

Após confirmar que o endpoint principal tem rate limiting, use `fuzz` para descobrir se outros caminhos no mesmo servidor também estão protegidos — ou se têm buracos.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `fuzz` no menu, preencha os campos e execute — sem decorar parâmetros.

## Uso

```bash
# Wordlist embutida
./limithit fuzz http://localhost:8080 --total 200

# Cache bust ativo (contorna CDN/reverse proxy cache)
./limithit fuzz http://localhost:8080 --cache-bust --total 200

# Wordlist personalizada
./limithit fuzz http://localhost:8080 --wordlist /path/to/paths.txt --total 1000 --concurrency 30
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--wordlist` | _(embutida)_ | Arquivo com caminhos (um por linha) |
| `--cache-bust` | `false` | Adiciona `?_cb=<hex>` único em cada request |
| `--total` | `1000` | Total de requisições |
| `--concurrency` | `20` | Workers paralelos |
| `--timeout` | `10` | Timeout por requisição (segundos) |

## Lendo o resultado

- Verifique distribuição de status codes por caminho
- Caminhos com `200` inesperado = rota não protegida exposta
- `403` em caminhos sensíveis = existe mas está bloqueado (confirma presença)
- Com `--cache-bust`: ausência de `429` indica que o cache está mascarando o rate limit real

## Formato da wordlist personalizada

```
/api/v1/users
/api/v1/admin
/health
/metrics
/debug/pprof
```

Linhas em branco e comentários com `#` são ignorados.

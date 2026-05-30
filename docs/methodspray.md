# methodspray

**Categoria:** Enumeração / Bypass de rate limiting por verbo

## O que faz

Monta o produto cartesiano de métodos HTTP × caminhos da wordlist e dispara todas as combinações contra o alvo. Em vez de fazer apenas `GET /api/users`, também testa `POST /api/users`, `PUT /api/users`, `DELETE /api/users`, etc.

Objetivo: encontrar rotas que o rate limiting não cobre para determinados verbos. Alguns servidores limitam `GET` mas esquecem `POST`, ou vice-versa.

## O que testa

- Diferença de rate limiting entre verbos HTTP no mesmo caminho
- Rotas que existem para um verbo mas são esquecidas em outros
- Configurações de rate limit que são aplicadas por rota mas não por método
- Endpoints que retornam `405 Method Not Allowed` (confirma existência da rota)

## Quando usar

Após `fuzz` identificar caminhos interessantes, use `methodspray` para verificar se todos os verbos nesses caminhos também estão protegidos.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `methodspray` no menu, preencha os campos e execute — sem decorar parâmetros.

## Uso

```bash
# Todos os verbos padrão × wordlist embutida
./limithit methodspray http://localhost:8080 --total 200

# Verbos específicos
./limithit methodspray http://localhost:8080 --methods GET,POST,DELETE --total 500

# Wordlist personalizada
./limithit methodspray http://localhost:8080 --wordlist /path/to/paths.txt --total 1000 --concurrency 30
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--methods` | `GET,POST,PUT,PATCH,DELETE,OPTIONS,HEAD` | Verbos a testar (comma-separated) |
| `--wordlist` | _(embutida)_ | Arquivo de caminhos (um por linha) |
| `--total` | `1000` | Total de requisições |
| `--concurrency` | `20` | Workers paralelos |
| `--timeout` | `10` | Timeout (segundos) |

## Volume de requisições

Com 7 métodos e uma wordlist de 200 caminhos = 1400 combinações únicas. Configure `--total` de acordo com o tamanho da matriz.

## Lendo o resultado

- **429 apenas em GET**: rate limit configurado só para `GET` — `POST` bypassa
- **405 em caminhos**: rota existe mas não aceita aquele verbo
- **200 em verbos inesperados**: endpoint não deveria aceitar aquele método (ex: `DELETE` em rota pública)
- **Diferença entre verbos**: assimetria de proteção = vetor de bypass

## Exemplo de bypass encontrado

```
GET  /api/data → 429 (rate limited)
POST /api/data → 200 (sem limite)
```

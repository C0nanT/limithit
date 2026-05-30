# flood

**Categoria:** Volumétrico / Teste de rate limiting

## O que faz

Dispara um volume alto de requisições HTTP contra um endpoint usando um pool de workers concorrentes. É o ataque mais direto: bate no alvo com o máximo de requisições possível, no método e concorrência configurados.

## O que testa

- Se o servidor aplica rate limiting (espera-se ver respostas `429 Too Many Requests`)
- Capacidade real de throughput sob carga
- Comportamento do servidor quando a fila de conexões satura
- Tempos de resposta degradados sob pressão

## Quando usar

Ponto de partida para qualquer teste de resistência. Se o servidor não responde com 429 depois de centenas de requisições por segundo, o rate limiting provavelmente não está configurado.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `flood` no menu, preencha os campos e execute — sem decorar parâmetros.

## Uso

```bash
# Básico
./limithit flood http://localhost:8080/api/ping --total 200 --concurrency 20

# Com método POST e corpo
./limithit flood http://localhost:8080/api/data --method POST --body '{"key":"val"}' --total 500 --concurrency 50

# Com URL como flag
./limithit flood --url http://localhost:8080/api/ping --total 1000 --concurrency 100
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--method` | `GET` | Método HTTP (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`) |
| `--body` | _(vazio)_ | Corpo da requisição |
| `--total` | `100` | Total de requisições a enviar |
| `--concurrency` | `10` | Workers paralelos |
| `--timeout` | `10` | Timeout por requisição (segundos) |

## Lendo o resultado

- **2xx alto** com poucos `429`: servidor não está limitando
- **429** aparecendo: rate limit ativo — note a partir de quantas requisições
- **Timeouts/erros**: servidor saturou antes de conseguir responder

## Interpretação de vulnerabilidade

| Sinal | Significado |
|-------|-------------|
| Sem `429` mesmo com milhares de reqs | Rate limit ausente ou muito alto |
| `429` após N reqs | Rate limit ativo, N = threshold |
| Timeouts crescentes | Servidor degradando (possível DoS) |

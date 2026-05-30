# headerbomb

**Categoria:** Exaustão de recursos / Headers oversized

## O que faz

Envia requisições com um volume grande de headers `X-Junk-N` de tamanho configurável, combinado com um corpo que cresce progressivamente a cada requisição (de `--body-start` até `--body-max`, dobrando por padrão ou crescendo pelo step configurado).

Dois vetores simultâneos:
1. **Header bomb**: N headers × tamanho por header = overhead de parsing
2. **Body escalation**: corpo cresce a cada request, testando limites de leitura do servidor

## O que testa

- `MaxHeaderBytes` do servidor (limite padrão do Go: 1MB)
- Se o servidor tem `http.MaxBytesReader` no corpo
- Comportamento sob parse de headers muito grandes
- Limites de memória e buffers do servidor sob corpos crescentes
- Diferença entre rejeição early (header) vs rejeição tardia (corpo)

## Quando usar

Para testar se o servidor rejeita requisições malformadas/oversized rapidamente. Um servidor bem configurado deve retornar `413 Request Entity Too Large` ou `431 Request Header Fields Too Large` antes de processar o conteúdo.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `headerbomb` no menu, preencha os campos e execute — sem decorar parâmetros.

## Uso

```bash
# Padrão (500 headers × 1KB cada, corpo de 1KB a 16MB)
./limithit headerbomb http://localhost:8080/api/echo

# Apenas header bomb, sem corpo crescente
./limithit headerbomb http://localhost:8080/api/echo --body-start 0 --body-max 0 --header-count 1000

# Apenas corpo crescente, sem headers extras
./limithit headerbomb http://localhost:8080/api/echo --header-count 0 --body-start 1024 --body-max 52428800

# Step linear em vez de dobrar
./limithit headerbomb http://localhost:8080/api/echo --body-step 1048576
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--header-count` | `500` | Número de headers `X-Junk-N` por requisição |
| `--header-size` | `1024` | Bytes por valor de header junk |
| `--body-start` | `1024` | Tamanho inicial do corpo (bytes) |
| `--body-max` | `16777216` | Tamanho máximo do corpo (bytes, 16MB) |
| `--body-step` | `0` | Crescimento por requisição (0 = dobra) |
| `--method` | _(auto)_ | POST se corpo > 0, GET caso contrário |
| `--total` | `50` | Total de requisições |
| `--concurrency` | `5` | Workers paralelos |
| `--timeout` | `15` | Timeout (segundos, maior por conta dos corpos grandes) |

## Carga por requisição

Com padrão: 500 headers × 1KB = 500KB só em headers, mais o corpo crescente. Total por requisição pode superar 16MB no pico.

## Lendo o resultado

| Resposta | Significado |
|----------|-------------|
| `431` | Servidor rejeita headers grandes — protegido |
| `413` | Servidor rejeita corpo grande — protegido |
| `200` / `2xx` consistente | Servidor aceita tudo sem limite — vulnerável |
| Timeout no cliente | Servidor travou processando — possível DoS |
| Conexão recusada | Servidor derrubou a conexão (proteção de emergência) |

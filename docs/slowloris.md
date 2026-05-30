# slowloris

**Categoria:** Exaustão de conexões / Slow HTTP

## O que faz

Abre muitas conexões TCP com o servidor e as mantém abertas enviando headers HTTP parciais a conta-gotas (drip), nunca completando a requisição. O servidor aguarda o header final que nunca chega, mantendo a conexão ocupada.

O objetivo é esgotar o pool de conexões do servidor para que clientes legítimos não consigam conectar — sem gerar volume de tráfego alto.

Cada conexão envia o início de uma requisição GET, depois dripa um header `X-Keep-Alive-N` aleatório a cada `--header-interval` segundos para evitar que o servidor detecte inatividade e feche a conexão.

## O que testa

- Limite de conexões simultâneas do servidor
- Presença de `ReadHeaderTimeout` no servidor (proteção principal)
- Presença de `IdleTimeout` para conexões lentas
- Comportamento quando o pool de conexões está esgotado

## Como interpretar se o servidor está protegido

- `DroppedByServer > 0`: servidor fechou ativamente as conexões — protegido
- `DroppedByServer = 0` com `AvgHold ≈ --hold`: servidor manteve todas abertas — vulnerável
- `AvgHold` muito menor que `--hold`: servidor tem timeout curto — protegido

O testserver local tem `ReadHeaderTimeout: 5s`, então após 5 segundos fecha conexões lentas.

## Quando usar

Para verificar se o servidor HTTP tem timeouts de leitura configurados corretamente. Servidores sem `ReadHeaderTimeout` são diretamente vulneráveis ao Slowloris clássico.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `slowloris` no menu, preencha os campos e execute — sem decorar parâmetros.

## Uso

```bash
# Básico: 50 conexões por 30 segundos
./limithit slowloris http://localhost:8080 --connections 50 --hold 30

# Mais agressivo
./limithit slowloris http://localhost:8080 --connections 200 --hold 120

# Header drip mais frequente (evita timeout rápido)
./limithit slowloris http://localhost:8080 --connections 100 --header-interval 5 --hold 60

# HTTPS
./limithit slowloris https://localhost:8443 --connections 50 --insecure
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--connections` | `200` | Conexões simultâneas a manter abertas |
| `--header-interval` | `10` | Segundos entre drips de header |
| `--hold` | `120` | Duração máxima de cada conexão (segundos) |
| `--dial-timeout` | `5` | Timeout de conexão TCP (segundos) |
| `--insecure` | `false` | Pular verificação de certificado TLS |

## Lendo o relatório

| Campo | Significado |
|-------|-------------|
| `Attempted` | Total de conexões que tentamos abrir |
| `Established` | Conexões que chegaram a se conectar |
| `DroppedByServer` | Servidor fechou antes de --hold |
| `DroppedByClient` | Fechamos nós mesmos (--hold expirou) |
| `AvgHold` | Tempo médio de vida das conexões |
| `Errors` | Classificação de erros (refused, timeout, reset) |

## Proteção esperada

```
ReadHeaderTimeout: 5s   → fecha slowloris em 5s
IdleTimeout: 30s        → fecha conexões ociosas
MaxHeaderBytes: 16KB    → rejeita headers gigantes
```

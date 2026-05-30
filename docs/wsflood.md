# wsflood

**Categoria:** Exaustão de conexões / WebSocket

## O que faz

Abre um grande número de conexões WebSocket e as mantém abertas pelo tempo configurado. Opcionalmente envia pings periódicos para manter as conexões ativas e simular clientes reais.

O handshake WebSocket é feito na mão (sem biblioteca): envia `Upgrade: websocket` e verifica `101 Switching Protocols`. Após estabelecer, a conexão fica aberta sem enviar dados (modo silencioso) ou enviando ping frames WebSocket se `--message-rate > 0`.

## O que testa

- Limite de conexões WebSocket simultâneas do servidor
- Se o servidor tem rate limiting na fase de upgrade (antes de estabelecer WS)
- Capacidade do servidor de manter muitas conexões WS abertas sem degradar
- Presença de `429` ou rejeição durante handshake para clientes excessivos

## Quando usar

Para testar servidores que expõem endpoints WebSocket — chats, feeds em tempo real, dashboards de monitoramento. O esgotamento de conexões WS pode bloquear usuários legítimos da funcionalidade em tempo real.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `wsflood` no menu, preencha os campos e execute — sem decorar parâmetros.

## Uso

```bash
# Básico: 100 conexões por 60 segundos
./limithit wsflood ws://localhost:8080/ws --total 100

# Usando http:// (aceito também)
./limithit wsflood http://localhost:8080/ws --connections 100 --hold 60

# Com pings a 2/segundo por conexão
./limithit wsflood ws://localhost:8080/ws --connections 50 --hold 30 --message-rate 2

# WebSocket TLS (wss://)
./limithit wsflood wss://localhost:8443/ws --connections 50 --insecure
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--connections` | `200` | Conexões WebSocket a abrir e manter |
| `--hold` | `60` | Segundos para manter cada conexão aberta |
| `--message-rate` | `0` | Pings por segundo por conexão (0 = silencioso) |
| `--dial-timeout` | `5` | Timeout de conexão TCP (segundos) |
| `--insecure` | `false` | Pular verificação de certificado TLS |

## Esquemas aceitos

| Entrada | Interpretado como |
|---------|-------------------|
| `ws://` | WebSocket plain |
| `wss://` | WebSocket TLS |
| `http://` | Convertido para `ws://` |
| `https://` | Convertido para `wss://` |

## Lendo o relatório

| Campo | Significado |
|-------|-------------|
| `Attempted` | Tentativas de conexão |
| `Established` | Handshakes bem-sucedidos (101) |
| `DroppedByServer` | Servidor fechou antes de --hold |
| `DroppedByClient` | --hold expirou normalmente |
| `Errors` | `rejected-429`, `handshake-*`, `timeout`, etc. |

## Sinais de proteção

- `rejected-429` nos erros: servidor aplica rate limit no upgrade — protegido
- `DroppedByServer` alto: servidor está fechando conexões excessivas — protegido
- `Established` = `Attempted` com longa `AvgHold`: servidor aceita tudo — sem proteção de conexão WS

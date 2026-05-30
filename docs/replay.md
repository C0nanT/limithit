# replay

**Categoria:** Reprodução de tráfego / Teste realista

## O que faz

Carrega um arquivo de requisições capturadas (HAR exportado do browser ou arquivo de texto simples) e as reproduz contra o alvo com concorrência configurável. Permite testar o servidor com tráfego real de produção em vez de requests sintéticos.

Suporta dois formatos de entrada:
- **HAR** (HTTP Archive): exportado pelo DevTools do browser ou proxies como Burp/mitmproxy
- **Texto delimitado por linha**: `METHOD URL` por linha, ou só `URL` (GET implícito)

## O que testa

- Rate limiting sob padrão de tráfego real (não sintético)
- Comportamento do servidor com a variedade de rotas e métodos do tráfego de produção
- Se sessões ou tokens capturados ainda são válidos
- Reprodução de sequências específicas de requisições que causaram bugs

## Quando usar

Quando você tem um trace de tráfego real e quer reproduzi-lo sob carga, ou quando quer testar se o rate limiting funciona não apenas com flood em um endpoint, mas com a distribuição natural de chamadas do cliente.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `replay` no menu, preencha o caminho do arquivo e execute — sem decorar parâmetros.

## Uso

```bash
# Arquivo de texto simples
./limithit replay http://localhost:8080 --file requests.txt --total 100

# HAR exportado do Chrome DevTools
./limithit replay http://localhost:8080 --file session.har

# Loop para atingir --total maior que o arquivo
./limithit replay http://localhost:8080 --file requests.txt --loop --total 1000

# Alta concorrência
./limithit replay http://localhost:8080 --file session.har --loop --total 500 --concurrency 50
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--file` | _(obrigatório)_ | Caminho para arquivo HAR ou texto |
| `--loop` | `false` | Reinicia do início quando acabar o arquivo |
| `--total` | `0` | Total de requisições (0 = tamanho do arquivo) |
| `--concurrency` | `10` | Workers paralelos |

## Formato do arquivo de texto

```
# Comentários são ignorados
GET http://localhost:8080/api/users
POST http://localhost:8080/api/login
http://localhost:8080/api/ping
DELETE http://localhost:8080/api/session/123
```

Linha com só URL: assume `GET`. Linhas vazias e `#` são ignoradas.

## Exportando HAR no Chrome

1. Abrir DevTools → aba Network
2. Reproduzir a sessão que quer capturar
3. Botão direito em qualquer requisição → "Save all as HAR with content"
4. Usar o `.har` com `--file`

## Lendo o resultado

- **Sem `--loop`**: processa cada request do arquivo exatamente uma vez
- **Com `--loop`**: cicla pelo arquivo até atingir `--total`
- O campo URL no relatório mostra a URL de cada request individualmente
- Compare distribuição de status codes com o tráfego real esperado

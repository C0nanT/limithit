# gzipbomb

**Categoria:** Amplificação / Exaustão de recursos

> **AVISO:** Este ataque requer `--i-understand` explícito. Servidores vulneráveis podem crashar ou esgotar memória. Use apenas em sistemas que você possui ou tem autorização escrita para testar.

## O que faz

Envia requisições HTTP com corpo comprimido via gzip (`Content-Encoding: gzip`) que expande para um tamanho muito maior no servidor. O payload transferido pela rede é pequeno; o servidor precisa descomprimir e alocar toda a memória expandida.

Exemplo com `--expanded-mb 10`: envia alguns KB pela rede, força o servidor a alocar 10 MB por requisição.

## O que testa

- Se o servidor descomprime corpos gzip sem limite de tamanho
- Limites de memória e proteção contra decompression bomb
- Presença de `MaxBytesReader` ou equivalente no handler HTTP
- Comportamento sob amplificação: N requisições × M MB de expansão = stress de memória real

## Quando usar

Após confirmar que o endpoint aceita `Content-Encoding: gzip`. Servidores com frameworks que descomprimem automaticamente (como middlewares gzip em Node.js, Go, Python) podem ser vulneráveis se não houver limite configurado.

## UI Interativa

Sem flags, o `limithit` abre uma interface interativa no terminal para configurar o ataque:

```bash
./limithit
```

Selecione `gzipbomb` no menu. A UI exibe o aviso de amplificação e requer confirmação antes de prosseguir.

## Uso

```bash
# Exige --i-understand
./limithit gzipbomb http://localhost:8080/api/echo --i-understand

# Aumentar tamanho de expansão
./limithit gzipbomb http://localhost:8080/api/echo --i-understand --expanded-mb 50

# Menor concorrência para alvos frágeis
./limithit gzipbomb http://localhost:8080/api/echo --i-understand --expanded-mb 20 --total 5 --concurrency 2
```

## Flags

| Flag | Padrão | Descrição |
|------|--------|-----------|
| `--i-understand` | _(obrigatório)_ | Confirmação de uso responsável |
| `--expanded-mb` | `10` | Tamanho descomprimido por requisição (MB) |
| `--method` | `POST` | Método HTTP |
| `--total` | `10` | Total de requisições |
| `--concurrency` | `5` | Workers paralelos |

## Lendo o resultado

- **Servidor responde normalmente (2xx/4xx)**: tem proteção — `MaxBytesReader` ou similar está ativo
- **Timeouts crescentes ou erros de conexão**: servidor provavelmente saturando
- **OOM/crash no servidor**: vulnerável — não há limite de descompressão

## Proteção esperada no servidor

Servidor hardened rejeita o payload ou lê apenas até o limite configurado. O `testserver` local tem `MaxHeaderBytes: 16KB` mas o handler `/api/echo` pode precisar de `http.MaxBytesReader` adicional para o corpo.

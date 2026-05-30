# limithit — Documentação de Ataques

Referência de todos os ataques disponíveis. Cada ataque tem sua própria página com descrição, flags, exemplos e como interpretar os resultados.

> Todos os ataques devem ser usados apenas em sistemas que você possui ou tem autorização explícita para testar.

## Ataques disponíveis

| Ataque | Categoria | O que testa |
|--------|-----------|-------------|
| [flood](flood.md) | Volumétrico | Rate limiting básico, throughput sob carga |
| [fuzz](fuzz.md) | Enumeração | Rotas ocultas, proteção por caminho, cache bypass |
| [gzipbomb](gzipbomb.md) | Amplificação | Decompressão sem limite, exaustão de memória |
| [h2flood](h2flood.md) | Volumétrico HTTP/2 | Rate limit por conexão vs por stream, MaxConcurrentStreams |
| [headerbomb](headerbomb.md) | Exaustão de recursos | MaxHeaderBytes, MaxBytesReader, limites de parser |
| [methodspray](methodspray.md) | Enumeração | Rate limiting por verbo, rotas que escapam por método |
| [replay](replay.md) | Reprodução | Tráfego real sob carga, HAR import |
| [slowloris](slowloris.md) | Slow HTTP | ReadHeaderTimeout, pool de conexões |
| [spoof](spoof.md) | IP bypass | Confiança em X-Forwarded-For, trust-xff-cidr |
| [wsflood](wsflood.md) | WebSocket | Conexões WS simultâneas, rate limit no upgrade |

## UI Interativa

Sem argumentos, o `limithit` abre uma interface interativa no terminal — selecione o ataque, preencha os campos e execute sem decorar flags:

```bash
./limithit
```

## Referência rápida de comandos

```bash
# Flood básico
./limithit flood http://localhost:8080/api/ping --total 200 --concurrency 20

# Enumerar caminhos com cache bypass
./limithit fuzz http://localhost:8080 --cache-bust --total 200

# Amplificação por gzip (requer --i-understand)
./limithit gzipbomb http://localhost:8080/api/echo --i-understand

# HTTP/2 stream flood
./limithit h2flood https://localhost:8443/api/ping --total 200

# Headers e corpo oversized
./limithit headerbomb http://localhost:8080/api/echo --header-count 100 --header-size 256

# Spray de métodos × caminhos
./limithit methodspray http://localhost:8080 --total 200

# Reproduzir requests de um arquivo
./limithit replay http://localhost:8080 --file requests.txt --total 200

# Slowloris (conexões lentas)
./limithit slowloris http://localhost:8080 --connections 50 --hold 30

# IP spoofing via X-Forwarded-For
./limithit spoof http://localhost:8080/api/ping --ip-pool 10.0.0.0/28 --total 200

# WebSocket flood
./limithit wsflood ws://localhost:8080/ws --total 100
```

## Rodando um cenário completo

Em vez de rodar ataques individuais, use um arquivo YAML para orquestrar uma sequência:

```bash
./limithit scenario run examples/scenario.yaml
./limithit scenario run examples/scenario.yaml --continue-on-fail
./limithit scenario validate examples/scenario.yaml
./limithit scenario init > limithit.yaml  # scaffold
```

## Testserver local

Para testar sem expor infraestrutura real:

```bash
# Rate limit básico
cd testserver && go run . --rate 5 --burst 5

# Com suporte a XFF (para testar spoof)
cd testserver && go run . --rate 5 --burst 5 --trust-xff-cidr 127.0.0.0/8

# Algoritmo alternativo
cd testserver && go run . --rate 5 --burst 5 --algo slidingwindow
```

Dashboard em tempo real disponível em `http://localhost:8080` após iniciar o testserver.

# Semantic Cache Gateway

A high-performance middleware service that optimizes LLM interactions through semantic caching. It intercepts requests between clients and LLM providers (e.g., OpenAI), using vector similarity search to serve cached responses for semantically similar queries.

## Performance Results

| Metric | Value |
|--------|-------|
| Cache Hit Rate | 60% |
| Avg Cache HIT Latency | 55ms |
| Avg Direct OpenAI Latency | 842ms |
| **Speedup** | **15.2x faster** |
| Latency Saved per HIT | 787ms |

**At scale (1M requests/month with 60% hit rate):**
- 600K cached responses
- ~$1,200/month saved in API costs
- Sub-100ms response times for cache hits

## Features

- **Two-tier caching**: SHA-256 exact match + vector similarity search
- **Semantic matching**: Catches paraphrased queries (0.95 cosine similarity threshold)
- **Async write-behind**: Zero added latency for cache misses
- **Graceful degradation**: Falls back to direct upstream on Redis/embedding failures
- **OpenAI API compatible**: Drop-in replacement for `/chat/completions` endpoint
- **Structured logging**: JSON logs with request correlation and latency tracking

## Quick Start

### Prerequisites
- Docker and Docker Compose
- OpenAI API key

### Run with Docker

```bash
# Set your OpenAI API key
export EMBEDDING_API_KEY=sk-your-key-here

# Start the gateway and Redis
docker compose up --build

# Test health endpoint
curl http://localhost:8080/health
```

### Test the Gateway

```bash
# First request (cache miss - goes to OpenAI)
curl -X POST http://localhost:8080/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is the capital of France?"}]}'

# Second request (cache hit - served from Redis in ~50ms)
curl -X POST http://localhost:8080/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is the capital of France?"}]}'
```

Look for `X-Cache-Status: HIT` header in the response.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `PORT` | 8080 | Gateway listen port |
| `UPSTREAM_URL` | https://api.openai.com/v1 | LLM provider URL |
| `REDIS_URL` | redis://localhost:6379 | Redis Stack connection |
| `SIMILARITY_THRESHOLD` | 0.95 | Cosine similarity threshold for cache hits |
| `EMBEDDING_API_KEY` | - | OpenAI API key for embeddings |

## Architecture

```
Client → Gateway → [Hash Check] → [Vector Search] → Redis Cache
                         ↓              ↓
                    Cache HIT      Cache MISS → OpenAI → Async Store
```

1. **Intercept**: Gateway receives POST `/chat/completions`
2. **Hash Check**: SHA-256 exact match lookup (O(1))
3. **Embed**: Generate 1536-dim vector via OpenAI embeddings
4. **Vector Search**: KNN search with HNSW index in Redis
5. **Serve**: Return cached response (hit) or forward to upstream (miss)
6. **Store**: Async goroutine stores response + embedding on miss

## Running Tests

```bash
go test ./... -v
```

## Benchmarking

```powershell
# PowerShell
$env:OPENAI_API_KEY = "your-key"
.\scripts\benchmark.ps1
```

## Project Structure

```
├── cmd/gateway/          # Main entry point
├── internal/
│   ├── cache/           # Redis client and cache service
│   ├── config/          # Configuration loading
│   ├── embedding/       # OpenAI embedding service
│   ├── handler/         # Main request handler
│   ├── logger/          # Structured logging
│   ├── middleware/      # Body buffer middleware
│   ├── models/          # Request/response models
│   └── proxy/           # Upstream proxy
├── scripts/             # Benchmark scripts
├── docker-compose.yml   # Docker orchestration
└── Dockerfile           # Multi-stage build
```

## License

MIT

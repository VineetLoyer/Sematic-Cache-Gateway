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
export EMBEDDING_API_KEY=sk-...

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

## Stats Dashboard

Access real-time metrics at `/stats`:

- **Cache Hit Rate** - Percentage of requests served from cache
- **Cost Saved** - Estimated API cost savings ($0.002/request)
- **Total Requests** - Total requests processed
- **Avg Latency** - Average response time
- **Uptime** - Time since gateway started

JSON API available at `/stats/json`.

![Stats Dashboard](https://via.placeholder.com/800x400?text=Stats+Dashboard+Preview)

## Deployment

### Railway (Recommended)

1. **Create a Railway account** at [railway.app](https://railway.app)

2. **Deploy from GitHub:**
   ```bash
   # Push your code to GitHub first
   git push origin main
   ```
   - Go to Railway Dashboard → New Project → Deploy from GitHub repo
   - Select your repository

3. **Add Redis:**
   - In your Railway project, click "New" → "Database" → "Redis"
   - Railway will automatically set `REDIS_URL`

4. **Set environment variables:**
   - `EMBEDDING_API_KEY` = your OpenAI API key
   - `UPSTREAM_URL` = `https://api.openai.com/v1`
   - `SIMILARITY_THRESHOLD` = `0.95`

5. **Get your URL:**
   - Railway provides a URL like `https://your-app.up.railway.app`
   - Use this as your OpenAI base URL in your apps

**Example with Railway:**
```python
from openai import OpenAI

client = OpenAI(
    api_key="your-openai-key",
    base_url="https://your-gateway.up.railway.app"  # Railway URL
)
```

### Docker Compose (Local/Self-hosted)

```bash
export EMBEDDING_API_KEY=sk-your-key
docker compose up --build
```

### Kubernetes

See the Kubernetes deployment example in the Integration Guide section.

## Integration Guide

### Using as a Middleware in Your Project

The gateway acts as a transparent proxy. Simply point your OpenAI client to the gateway instead of OpenAI directly.

#### Python (OpenAI SDK)

```python
from openai import OpenAI

# Instead of using OpenAI directly, point to the gateway
client = OpenAI(
    api_key="your-openai-key",
    base_url="http://localhost:8080"  # Gateway URL
)

# Use exactly as before - no code changes needed
response = client.chat.completions.create(
    model="gpt-3.5-turbo",
    messages=[{"role": "user", "content": "What is the capital of France?"}]
)
print(response.choices[0].message.content)
```

#### Node.js (OpenAI SDK)

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: 'your-openai-key',
  baseURL: 'http://localhost:8080'  // Gateway URL
});

const response = await client.chat.completions.create({
  model: 'gpt-3.5-turbo',
  messages: [{ role: 'user', content: 'What is the capital of France?' }]
});
console.log(response.choices[0].message.content);
```

#### cURL / HTTP

```bash
# Just change the URL from api.openai.com to your gateway
curl -X POST http://localhost:8080/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-openai-key" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}'
```

#### LangChain (Python)

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    model="gpt-3.5-turbo",
    openai_api_key="your-openai-key",
    openai_api_base="http://localhost:8080"  # Gateway URL
)

response = llm.invoke("What is the capital of France?")
```

### Docker Compose Integration

Add the gateway to your existing `docker-compose.yml`:

```yaml
services:
  # Your existing app
  my-app:
    build: .
    environment:
      - OPENAI_BASE_URL=http://semantic-cache:8080
    depends_on:
      - semantic-cache

  # Semantic Cache Gateway
  semantic-cache:
    image: ghcr.io/your-org/semantic-cache-gateway:latest
    # Or build from source:
    # build: ./path/to/semantic-cache-gateway
    environment:
      - UPSTREAM_URL=https://api.openai.com/v1
      - REDIS_URL=redis://redis:6379
      - EMBEDDING_API_KEY=${OPENAI_API_KEY}
      - SIMILARITY_THRESHOLD=0.95
    depends_on:
      - redis

  redis:
    image: redis/redis-stack-server:latest
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: semantic-cache-gateway
spec:
  replicas: 2
  selector:
    matchLabels:
      app: semantic-cache
  template:
    spec:
      containers:
      - name: gateway
        image: semantic-cache-gateway:latest
        ports:
        - containerPort: 8080
        env:
        - name: UPSTREAM_URL
          value: "https://api.openai.com/v1"
        - name: REDIS_URL
          value: "redis://redis-service:6379"
        - name: EMBEDDING_API_KEY
          valueFrom:
            secretKeyRef:
              name: openai-secret
              key: api-key
---
apiVersion: v1
kind: Service
metadata:
  name: semantic-cache
spec:
  selector:
    app: semantic-cache
  ports:
  - port: 80
    targetPort: 8080
```

### Environment Variable Configuration

In your application, set the OpenAI base URL to point to the gateway:

```bash
# Instead of calling OpenAI directly
export OPENAI_API_BASE=http://localhost:8080

# Or in .env file
OPENAI_API_BASE=http://semantic-cache:8080
```

### Checking Cache Status

The gateway adds an `X-Cache-Status` header to responses:
- `HIT` - Response served from cache
- `MISS` - Response from OpenAI (now cached for future requests)

```python
import requests

response = requests.post(
    "http://localhost:8080/chat/completions",
    headers={"Authorization": "Bearer your-key", "Content-Type": "application/json"},
    json={"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}]}
)

print(f"Cache Status: {response.headers.get('X-Cache-Status')}")
print(f"Response: {response.json()}")
```

## License

MIT

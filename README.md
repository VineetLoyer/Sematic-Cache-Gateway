# Semantic Cache Gateway

A high-performance middleware that reduces LLM API costs and latency by caching responses and using vector similarity search to serve cached answers for semantically similar queries.

## Live Demo

Try the gateway without any setup:

**Base URL:** `https://sematic-cache-gateway-production.up.railway.app`

![Screenshot of stat dashboard](/image.png)
| Endpoint | Description |
|----------|-------------|
| `/stats` | Real-time metrics dashboard |
| `/stats/json` | JSON API for metrics |
| `/health` | Health check |
| `/v1/chat/completions` | OpenAI-compatible chat endpoint |

> **Note:** The demo uses shared API keys. For production use, deploy your own instance with your own keys.

## Performance Results

| Metric | Value |
|--------|-------|
| Cache Hit Rate | **80%** |
| Avg Cache Miss (OpenAI) | 1833ms |
| Avg Cache Hit | 360ms |
| **Speedup** | **5.1x faster** |

**At scale (1M requests/month with 80% hit rate):**
- 800K cached responses
- ~$1,600/month saved in API costs
- Sub-400ms response times for cache hits

## Features

- **Two-tier caching**: SHA-256 exact match + HNSW vector similarity search
- **Semantic matching**: Catches paraphrased queries (configurable similarity threshold)
- **24-hour TTL**: Cache entries auto-expire to manage memory
- **Async write-behind**: Zero added latency for cache misses
- **Graceful degradation**: Falls back to direct upstream on failures
- **OpenAI API compatible**: Drop-in replacement for `/chat/completions`
- **Real-time dashboard**: Monitor hit rates, latency, and cost savings

## How It Works

```
Client Request
      ↓
┌─────────────────┐
│  Gateway        │
│  1. Hash Check  │ ──→ Exact Match? → Return Cached Response
│  2. Embed Query │
│  3. Vector Search│ ──→ Similar Match (>90%)? → Return Cached Response
│  4. Forward to  │
│     OpenAI      │ ──→ Cache Miss → Get Response → Store Async
└─────────────────┘
      ↓
   Response
```


## Quick Start: Deploy Your Own Instance

### Prerequisites

- OpenAI API key
- Railway account (free tier works) OR Docker installed locally

### Option 1: Deploy to Railway (Recommended)

1. **Fork this repository** to your GitHub account

2. **Create a Railway account** at [railway.app](https://railway.app)

3. **Create a new project:**
   - Go to Railway Dashboard → "New Project" → "Deploy from GitHub repo"
   - Select your forked repository

4. **Add Redis Stack:**
   - In your Railway project, click "New" → "Database" → "Redis"
   - Railway automatically sets `REDIS_URL`

5. **Configure environment variables** in Railway:
   ```
   EMBEDDING_API_KEY=sk-your-openai-api-key
   UPSTREAM_API_KEY=sk-your-openai-api-key
   UPSTREAM_URL=https://api.openai.com/v1
   SIMILARITY_THRESHOLD=0.90
   PORT=8080
   ```

6. **Deploy** - Railway will build and deploy automatically

7. **Get your URL** - Railway provides a URL like `https://your-app.up.railway.app`

### Option 2: Run Locally with Docker

```bash
# Clone the repository
git clone https://github.com/your-username/semantic-cache-gateway.git
cd semantic-cache-gateway

# Create .env file
cat > .env << EOF
EMBEDDING_API_KEY=sk-your-openai-api-key
UPSTREAM_API_KEY=sk-your-openai-api-key
UPSTREAM_URL=https://api.openai.com/v1
SIMILARITY_THRESHOLD=0.90
REDIS_URL=redis://redis:6379
PORT=8080
EOF

# Start the gateway and Redis
docker compose up --build

# Gateway is now running at http://localhost:8080
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `PORT` | 8080 | Gateway listen port |
| `UPSTREAM_URL` | https://api.openai.com/v1 | LLM provider URL |
| `UPSTREAM_API_KEY` | - | API key for upstream LLM (server-side) |
| `EMBEDDING_API_KEY` | - | OpenAI API key for generating embeddings |
| `REDIS_URL` | redis://localhost:6379 | Redis Stack connection URL |
| `SIMILARITY_THRESHOLD` | 0.95 | Cosine similarity threshold (0.0-1.0) |

### Understanding the API Keys

- **`EMBEDDING_API_KEY`**: Used to generate vector embeddings for semantic search. This calls OpenAI's embedding API.
- **`UPSTREAM_API_KEY`**: Used to forward requests to OpenAI's chat completion API. If set, clients don't need to provide their own API key.

## Using the Gateway

### Replace OpenAI URL in Your Code

The gateway is a drop-in replacement. Just change the base URL:

#### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    api_key="not-needed-if-server-has-UPSTREAM_API_KEY",
    base_url="https://your-gateway.up.railway.app"  # Your gateway URL
)

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
  apiKey: 'not-needed-if-server-has-UPSTREAM_API_KEY',
  baseURL: 'https://your-gateway.up.railway.app'
});

const response = await client.chat.completions.create({
  model: 'gpt-3.5-turbo',
  messages: [{ role: 'user', content: 'What is the capital of France?' }]
});
```

#### cURL

```bash
curl -X POST https://your-gateway.up.railway.app/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is the capital of France?"}]}'
```

#### LangChain

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    model="gpt-3.5-turbo",
    openai_api_key="not-needed",
    openai_api_base="https://your-gateway.up.railway.app"
)

response = llm.invoke("What is the capital of France?")
```

### Checking Cache Status

The gateway adds headers to responses:

| Header | Values | Description |
|--------|--------|-------------|
| `X-Cache-Status` | `HIT` / `MISS` | Whether response was served from cache |
| `X-Request-ID` | UUID | Unique request identifier for debugging |

```python
import requests

response = requests.post(
    "https://your-gateway.up.railway.app/v1/chat/completions",
    headers={"Content-Type": "application/json"},
    json={"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}]}
)

print(f"Cache Status: {response.headers.get('X-Cache-Status')}")
# First request: MISS
# Subsequent similar requests: HIT
```

## Load Testing

Test the gateway performance with the included PowerShell script:

```powershell
# Clone the repo and run the load test
.\scripts\load_test.ps1

# Or specify custom parameters
.\scripts\load_test.ps1 -BaseUrl "https://your-gateway.up.railway.app" -UniqueQueries 10 -RepeatPerQuery 5
```

### Test Against the Live Demo

```powershell
# Test the live demo deployment
.\scripts\load_test.ps1 -BaseUrl "https://sematic-cache-gateway-production.up.railway.app"
```

Expected output:
```
========================================
  LOAD TEST RESULTS
========================================

REQUEST BREAKDOWN:
  Total Requests:    50
  Cache Misses:      10
  Exact Hits:        40
  Semantic Hits:     0
  Errors:            0

PERFORMANCE METRICS:
  Avg Miss Latency:     1833ms
  Avg Hit Latency:      360ms
  Speedup:              5.1x faster

========================================
  KEY METRICS (LinkedIn Ready)
========================================
  Cache Hit Rate:    80%
  Latency Reduction: 5.1x
  Cost Saved:        0.08 USD
========================================
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/chat/completions` | POST | OpenAI-compatible chat endpoint |
| `/chat/completions` | POST | Alias for above |
| `/health` | GET | Health check (returns Redis status) |
| `/stats` | GET | HTML metrics dashboard |
| `/stats/json` | GET | JSON metrics API |
| `/cache/clear` | POST | Clear all cached entries |

### Clear Cache

```bash
# Clear all cached entries and reset stats
curl -X POST https://your-gateway.up.railway.app/cache/clear
```

## Monitoring

### Stats Dashboard

Access real-time metrics at `/stats`:

- **Cache Hit Rate** - Percentage of requests served from cache
- **Cost Saved** - Estimated API cost savings ($0.002/request)
- **Total Requests** - Total requests processed
- **Avg Latency** - Average response time
- **Uptime** - Time since gateway started

### JSON API

```bash
curl https://your-gateway.up.railway.app/stats/json
```

Response:
```json
{
  "total_requests": 50,
  "cache_hits": 40,
  "cache_misses": 10,
  "errors": 0,
  "total_latency_ms": 25000,
  "start_time": "2024-01-15T10:00:00Z",
  "cost_per_request": 0.002
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Semantic Cache Gateway                    │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Handler   │→ │   Cache     │→ │   Redis Stack       │  │
│  │             │  │   Service   │  │   - JSON Storage    │  │
│  │  - Parse    │  │             │  │   - HNSW Index      │  │
│  │  - Route    │  │  - Hash     │  │   - Vector Search   │  │
│  │  - Respond  │  │  - Vector   │  │                     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│         │                                                    │
│         ↓                                                    │
│  ┌─────────────┐  ┌─────────────┐                           │
│  │  Embedding  │→ │   Proxy     │→ OpenAI API               │
│  │  Service    │  │             │                           │
│  └─────────────┘  └─────────────┘                           │
└─────────────────────────────────────────────────────────────┘
```

## Project Structure

```
├── cmd/gateway/          # Main entry point
├── internal/
│   ├── cache/           # Redis client and cache service
│   ├── config/          # Configuration loading
│   ├── embedding/       # OpenAI embedding service
│   ├── handler/         # HTTP handlers and stats
│   ├── logger/          # Structured logging
│   ├── middleware/      # Request body buffering
│   ├── models/          # Request/response models
│   └── proxy/           # Upstream proxy
├── scripts/             # Load testing scripts
├── docker-compose.yml   # Local development
├── Dockerfile           # Production build
└── railway.json         # Railway deployment config
```

## Tuning the Similarity Threshold

The `SIMILARITY_THRESHOLD` controls how similar queries must be to get a cache hit:

| Threshold | Behavior |
|-----------|----------|
| 0.99 | Very strict - only nearly identical queries match |
| 0.95 | Strict - minor variations match |
| 0.90 | Balanced - paraphrased queries match (recommended) |
| 0.85 | Loose - broadly similar queries match |
| 0.80 | Very loose - may return irrelevant cached responses |

**Example at 0.90 threshold:**
- "What is the capital of France?" ✓ matches
- "What's the capital city of France?" ✓ matches
- "Tell me France's capital" ✓ matches
- "What is the capital of Germany?" ✗ different query

## Limitations

- **Single-tenant**: Current design shares cache across all users
- **Model-agnostic caching**: Returns cached response regardless of requested model
- **No streaming support**: Streaming responses are not cached
- **Embedding model locked**: Uses OpenAI's text-embedding-ada-002 (1536 dimensions)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test ./...`
5. Submit a pull request

## License

MIT

---

**Built by [Vineet Loyer](https://github.com/vineetloyer)**

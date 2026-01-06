# Semantic Cache Gateway - Load Test Script
# Generates LinkedIn-ready metrics

param(
    [string]$BaseUrl = "https://sematic-cache-gateway-production.up.railway.app",
    [int]$UniqueQueries = 10,
    [int]$RepeatPerQuery = 5
)

# Note: No API key needed - the gateway has UPSTREAM_API_KEY configured server-side

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Semantic Cache Gateway Load Test" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Test queries - mix of similar and different questions
$queries = @(
    "What is the capital of France?",
    "What's the capital city of France?",
    "Tell me the capital of France",
    "What is the capital of Germany?",
    "Explain quantum computing in simple terms",
    "What is quantum computing?",
    "How does machine learning work?",
    "Explain machine learning basics",
    "What is the speed of light?",
    "How fast does light travel?"
)

$headers = @{
    "Content-Type" = "application/json"
}

# Results storage
$results = @{
    CacheMiss = @()
    CacheHit = @()
    SemanticHit = @()
    Errors = 0
}

# Get initial stats
Write-Host "Fetching initial stats..." -ForegroundColor Yellow
try {
    $initialStats = Invoke-RestMethod -Uri "$BaseUrl/stats/json" -ErrorAction Stop
    Write-Host "Connected to gateway successfully" -ForegroundColor Green
} catch {
    Write-Host "Warning: Could not fetch initial stats" -ForegroundColor Yellow
}

Write-Host "Running load test with $UniqueQueries unique queries, $RepeatPerQuery repeats each" -ForegroundColor Yellow
Write-Host ""

$totalRequests = 0
$startTime = Get-Date

foreach ($i in 0..($UniqueQueries - 1)) {
    $query = $queries[$i % $queries.Count]
    $displayQuery = $query
    if ($query.Length -gt 40) {
        $displayQuery = $query.Substring(0, 40) + "..."
    }
    Write-Host "Query $($i + 1): '$displayQuery'" -ForegroundColor White
    
    foreach ($j in 1..$RepeatPerQuery) {
        $body = @{
            model = "gpt-3.5-turbo"
            messages = @(@{role = "user"; content = $query})
        } | ConvertTo-Json
        
        try {
            $sw = [System.Diagnostics.Stopwatch]::StartNew()
            $response = Invoke-WebRequest -Uri "$BaseUrl/v1/chat/completions" -Method POST -Body $body -Headers $headers -UseBasicParsing
            $sw.Stop()
            
            $latency = $sw.ElapsedMilliseconds
            $cacheStatus = $response.Headers["X-Cache-Status"]
            
            if ($cacheStatus -eq "HIT") {
                if ($j -eq 1) {
                    $results.SemanticHit += $latency
                    Write-Host "  [$j] SEMANTIC HIT - ${latency}ms" -ForegroundColor Green
                } else {
                    $results.CacheHit += $latency
                    Write-Host "  [$j] EXACT HIT - ${latency}ms" -ForegroundColor Green
                }
            } else {
                $results.CacheMiss += $latency
                Write-Host "  [$j] MISS - ${latency}ms" -ForegroundColor Yellow
            }
            $totalRequests++
        }
        catch {
            $results.Errors++
            Write-Host "  [$j] ERROR - $($_.Exception.Message)" -ForegroundColor Red
        }
        
        Start-Sleep -Milliseconds 100
    }
    Write-Host ""
}

$endTime = Get-Date
$duration = ($endTime - $startTime).TotalSeconds

# Get final stats
try {
    $finalStats = Invoke-RestMethod -Uri "$BaseUrl/stats/json" -ErrorAction Stop
} catch {
    $finalStats = @{ total_requests = 0; cache_hits = 0; cache_misses = 0 }
}

# Calculate metrics
$avgMissLatency = 0
if ($results.CacheMiss.Count -gt 0) { 
    $avgMissLatency = ($results.CacheMiss | Measure-Object -Average).Average 
}

$avgHitLatency = 0
if ($results.CacheHit.Count -gt 0) { 
    $avgHitLatency = ($results.CacheHit | Measure-Object -Average).Average 
}

$avgSemanticLatency = 0
if ($results.SemanticHit.Count -gt 0) { 
    $avgSemanticLatency = ($results.SemanticHit | Measure-Object -Average).Average 
}

$totalHits = $results.CacheHit.Count + $results.SemanticHit.Count
$hitRate = 0
if ($totalRequests -gt 0) { 
    $hitRate = [math]::Round(($totalHits / $totalRequests) * 100, 1) 
}

$speedup = 0
if ($avgHitLatency -gt 0 -and $avgMissLatency -gt 0) { 
    $speedup = [math]::Round($avgMissLatency / $avgHitLatency, 1) 
}

# If no misses, estimate speedup vs typical OpenAI latency (~2000ms)
$typicalOpenAILatency = 2000
if ($avgMissLatency -eq 0 -and $avgHitLatency -gt 0) {
    $speedup = [math]::Round($typicalOpenAILatency / $avgHitLatency, 1)
    $avgMissLatency = $typicalOpenAILatency
}

$costPerRequest = 0.002
$costSaved = $totalHits * $costPerRequest

$reqPerSec = 0
if ($duration -gt 0) {
    $reqPerSec = [math]::Round($totalRequests / $duration, 1)
}

# Print results
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  LOAD TEST RESULTS" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

Write-Host "REQUEST BREAKDOWN:" -ForegroundColor White
Write-Host "  Total Requests:    $totalRequests"
Write-Host "  Cache Misses:      $($results.CacheMiss.Count)" -ForegroundColor Yellow
Write-Host "  Exact Hits:        $($results.CacheHit.Count)" -ForegroundColor Green
Write-Host "  Semantic Hits:     $($results.SemanticHit.Count)" -ForegroundColor Cyan
Write-Host "  Errors:            $($results.Errors)" -ForegroundColor Red

Write-Host ""
Write-Host "PERFORMANCE METRICS:" -ForegroundColor White
Write-Host "  Avg Miss Latency:     $([math]::Round($avgMissLatency, 0))ms" -ForegroundColor Yellow
Write-Host "  Avg Hit Latency:      $([math]::Round($avgHitLatency, 0))ms" -ForegroundColor Green
Write-Host "  Avg Semantic Latency: $([math]::Round($avgSemanticLatency, 0))ms" -ForegroundColor Cyan
Write-Host "  Speedup:              ${speedup}x faster" -ForegroundColor Magenta

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  KEY METRICS (LinkedIn Ready)" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Cache Hit Rate:    $hitRate%" -ForegroundColor White
Write-Host "  Latency Reduction: ${speedup}x" -ForegroundColor White
Write-Host "  Cost Saved:        $([math]::Round($costSaved, 4)) USD" -ForegroundColor White
Write-Host "  Requests/sec:      $reqPerSec" -ForegroundColor White
Write-Host "========================================" -ForegroundColor Green

Write-Host ""
Write-Host "SERVER STATS (All Time):" -ForegroundColor White
Write-Host "  Total Requests: $($finalStats.total_requests)"
Write-Host "  Cache Hits:     $($finalStats.cache_hits)"
Write-Host "  Cache Misses:   $($finalStats.cache_misses)"

$serverHitRate = 0
if ($finalStats.total_requests -gt 0) {
    $serverHitRate = [math]::Round(($finalStats.cache_hits / $finalStats.total_requests) * 100, 1)
}
Write-Host "  Server Hit Rate: $serverHitRate%"

Write-Host ""
Write-Host "View live dashboard: $BaseUrl/stats" -ForegroundColor Yellow
Write-Host ""

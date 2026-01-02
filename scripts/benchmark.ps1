# Semantic Cache Gateway Benchmark Script
# Measures latency improvements and cache hit rates

param(
    [string]$ApiKey = $env:OPENAI_API_KEY,
    [string]$GatewayUrl = "http://localhost:8080",
    [string]$DirectUrl = "https://api.openai.com/v1",
    [int]$Iterations = 10
)

if (-not $ApiKey) {
    Write-Error "Please set OPENAI_API_KEY environment variable or pass -ApiKey parameter"
    exit 1
}

$headers = @{
    "Authorization" = "Bearer $ApiKey"
    "Content-Type" = "application/json"
}

# Test queries - includes exact duplicates and semantic variations
$queries = @(
    # Group 1: Capital questions (semantic similarity)
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is the capital of France?"}]}',
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is the capital of France?"}]}',  # Exact duplicate
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Tell me the capital city of France"}]}',  # Semantic match
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"France capital?"}]}',  # Semantic match
    
    # Group 2: Weather questions
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"How does weather forecasting work?"}]}',
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"How does weather forecasting work?"}]}',  # Exact duplicate
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Explain weather prediction methods"}]}',  # Semantic match
    
    # Group 3: Programming questions
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is a REST API?"}]}',
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is a REST API?"}]}',  # Exact duplicate
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Explain REST APIs"}]}'  # Semantic match
)

Write-Host "============================================" -ForegroundColor Cyan
Write-Host "Semantic Cache Gateway Benchmark" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""

# Results storage
$gatewayResults = @()
$directResults = @()

# Function to make request and measure time
function Measure-Request {
    param(
        [string]$Url,
        [string]$Body,
        [hashtable]$Headers
    )
    
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $response = Invoke-WebRequest -Uri $Url -Method POST -Body $Body -Headers $Headers -UseBasicParsing -TimeoutSec 60
        $stopwatch.Stop()
        
        $cacheStatus = "MISS"
        if ($response.Headers["X-Cache-Status"]) {
            $cacheStatus = $response.Headers["X-Cache-Status"]
        }
        
        return @{
            Success = $true
            LatencyMs = $stopwatch.ElapsedMilliseconds
            CacheStatus = $cacheStatus
            StatusCode = $response.StatusCode
        }
    }
    catch {
        $stopwatch.Stop()
        return @{
            Success = $false
            LatencyMs = $stopwatch.ElapsedMilliseconds
            CacheStatus = "ERROR"
            Error = $_.Exception.Message
        }
    }
}

Write-Host "Phase 1: Testing Gateway (with caching)" -ForegroundColor Yellow
Write-Host "----------------------------------------"

$cacheHits = 0
$cacheMisses = 0

for ($i = 0; $i -lt $queries.Count; $i++) {
    $query = $queries[$i]
    $queryPreview = ($query | ConvertFrom-Json).messages[0].content
    if ($queryPreview.Length -gt 40) { $queryPreview = $queryPreview.Substring(0, 40) + "..." }
    
    Write-Host "  [$($i+1)/$($queries.Count)] '$queryPreview'" -NoNewline
    
    $result = Measure-Request -Url "$GatewayUrl/chat/completions" -Body $query -Headers $headers
    $gatewayResults += $result
    
    if ($result.Success) {
        $color = if ($result.CacheStatus -eq "HIT") { "Green" } else { "Yellow" }
        Write-Host " - $($result.LatencyMs)ms [$($result.CacheStatus)]" -ForegroundColor $color
        
        if ($result.CacheStatus -eq "HIT") { $cacheHits++ } else { $cacheMisses++ }
    }
    else {
        Write-Host " - ERROR: $($result.Error)" -ForegroundColor Red
    }
    
    Start-Sleep -Milliseconds 2000  # Rate limiting - 2 seconds between requests
}

Write-Host ""
Write-Host "Phase 2: Testing Direct OpenAI (no caching)" -ForegroundColor Yellow
Write-Host "--------------------------------------------"

# Only test unique queries for direct comparison
$uniqueQueries = @(
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is the capital of Germany?"}]}',
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"How does photosynthesis work?"}]}',
    '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is machine learning?"}]}'
)

foreach ($query in $uniqueQueries) {
    $queryPreview = ($query | ConvertFrom-Json).messages[0].content
    if ($queryPreview.Length -gt 40) { $queryPreview = $queryPreview.Substring(0, 40) + "..." }
    
    Write-Host "  '$queryPreview'" -NoNewline
    
    $result = Measure-Request -Url "$DirectUrl/chat/completions" -Body $query -Headers $headers
    $directResults += $result
    
    if ($result.Success) {
        Write-Host " - $($result.LatencyMs)ms" -ForegroundColor Cyan
    }
    else {
        Write-Host " - ERROR: $($result.Error)" -ForegroundColor Red
    }
    
    Start-Sleep -Milliseconds 500
}

# Calculate statistics
Write-Host ""
Write-Host "============================================" -ForegroundColor Cyan
Write-Host "RESULTS SUMMARY" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan

$successfulGateway = $gatewayResults | Where-Object { $_.Success }
$successfulDirect = $directResults | Where-Object { $_.Success }

$gatewayHits = @($successfulGateway | Where-Object { $_.CacheStatus -eq "HIT" })
$gatewayMisses = @($successfulGateway | Where-Object { $_.CacheStatus -ne "HIT" })

if ($gatewayHits.Count -gt 0) {
    $avgHitLatency = ($gatewayHits.LatencyMs | Measure-Object -Average).Average
}
else {
    $avgHitLatency = 0
}

if ($gatewayMisses.Count -gt 0) {
    $avgMissLatency = ($gatewayMisses.LatencyMs | Measure-Object -Average).Average
}
else {
    $avgMissLatency = 0
}

if ($successfulDirect.Count -gt 0) {
    $avgDirectLatency = ($successfulDirect.LatencyMs | Measure-Object -Average).Average
}
else {
    $avgDirectLatency = 0
}

$hitRate = if ($successfulGateway.Count -gt 0) { [math]::Round(($cacheHits / $successfulGateway.Count) * 100, 1) } else { 0 }

Write-Host ""
Write-Host "Cache Performance:" -ForegroundColor White
Write-Host "  Total Requests:     $($successfulGateway.Count)"
Write-Host "  Cache Hits:         $cacheHits ($hitRate%)" -ForegroundColor Green
Write-Host "  Cache Misses:       $cacheMisses" -ForegroundColor Yellow
Write-Host ""
Write-Host "Latency Comparison:" -ForegroundColor White
Write-Host "  Avg Cache HIT:      $([math]::Round($avgHitLatency, 0))ms" -ForegroundColor Green
Write-Host "  Avg Cache MISS:     $([math]::Round($avgMissLatency, 0))ms" -ForegroundColor Yellow
Write-Host "  Avg Direct OpenAI:  $([math]::Round($avgDirectLatency, 0))ms" -ForegroundColor Cyan
Write-Host ""

if ($avgDirectLatency -gt 0 -and $avgHitLatency -gt 0) {
    $speedup = [math]::Round($avgDirectLatency / $avgHitLatency, 1)
    $latencySaved = [math]::Round($avgDirectLatency - $avgHitLatency, 0)
    Write-Host "Performance Gains:" -ForegroundColor White
    Write-Host "  Speedup (HIT vs Direct): ${speedup}x faster" -ForegroundColor Green
    Write-Host "  Latency Saved per HIT:   ${latencySaved}ms" -ForegroundColor Green
}

# Cost estimation (rough)
$costPerRequest = 0.002  # Approximate cost per GPT-3.5-turbo request
$savedRequests = $cacheHits
$estimatedSavings = [math]::Round($savedRequests * $costPerRequest, 4)

Write-Host ""
Write-Host "Cost Estimation:" -ForegroundColor White
Write-Host "  API Calls Saved:    $savedRequests"
Write-Host "  Est. Cost Saved:    `$$estimatedSavings (at ~`$0.002/request)"
Write-Host ""
Write-Host "============================================" -ForegroundColor Cyan

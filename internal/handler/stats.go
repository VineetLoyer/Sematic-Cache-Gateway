// Package handler provides HTTP handlers for the gateway.
package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sync/atomic"
	"time"
)

// Stats tracks gateway metrics.
type Stats struct {
	TotalRequests    int64     `json:"total_requests"`
	CacheHits        int64     `json:"cache_hits"`
	CacheMisses      int64     `json:"cache_misses"`
	Errors           int64     `json:"errors"`
	TotalLatencyMs   int64     `json:"total_latency_ms"`
	StartTime        time.Time `json:"start_time"`
	CostPerRequest   float64   `json:"cost_per_request"`
}

// Global stats instance
var globalStats = &Stats{
	StartTime:      time.Now(),
	CostPerRequest: 0.002, // ~$0.002 per GPT-3.5-turbo request
}

// RecordHit records a cache hit.
func RecordHit(latencyMs int64) {
	atomic.AddInt64(&globalStats.TotalRequests, 1)
	atomic.AddInt64(&globalStats.CacheHits, 1)
	atomic.AddInt64(&globalStats.TotalLatencyMs, latencyMs)
}

// RecordMiss records a cache miss.
func RecordMiss(latencyMs int64) {
	atomic.AddInt64(&globalStats.TotalRequests, 1)
	atomic.AddInt64(&globalStats.CacheMisses, 1)
	atomic.AddInt64(&globalStats.TotalLatencyMs, latencyMs)
}

// RecordError records an error.
func RecordError() {
	atomic.AddInt64(&globalStats.TotalRequests, 1)
	atomic.AddInt64(&globalStats.Errors, 1)
}

// ResetStats resets all stats to zero.
func ResetStats() {
	atomic.StoreInt64(&globalStats.TotalRequests, 0)
	atomic.StoreInt64(&globalStats.CacheHits, 0)
	atomic.StoreInt64(&globalStats.CacheMisses, 0)
	atomic.StoreInt64(&globalStats.Errors, 0)
	atomic.StoreInt64(&globalStats.TotalLatencyMs, 0)
	globalStats.StartTime = time.Now()
}

// GetStats returns current stats.
func GetStats() Stats {
	return Stats{
		TotalRequests:  atomic.LoadInt64(&globalStats.TotalRequests),
		CacheHits:      atomic.LoadInt64(&globalStats.CacheHits),
		CacheMisses:    atomic.LoadInt64(&globalStats.CacheMisses),
		Errors:         atomic.LoadInt64(&globalStats.Errors),
		TotalLatencyMs: atomic.LoadInt64(&globalStats.TotalLatencyMs),
		StartTime:      globalStats.StartTime,
		CostPerRequest: globalStats.CostPerRequest,
	}
}

// StatsJSON returns stats as JSON.
func StatsJSON(w http.ResponseWriter, r *http.Request) {
	stats := GetStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// StatsDashboard returns an HTML dashboard.
func StatsDashboard(w http.ResponseWriter, r *http.Request) {
	stats := GetStats()
	
	// Calculate derived metrics
	hitRate := float64(0)
	if stats.TotalRequests > 0 {
		hitRate = float64(stats.CacheHits) / float64(stats.TotalRequests) * 100
	}
	
	avgLatency := float64(0)
	if stats.TotalRequests > 0 {
		avgLatency = float64(stats.TotalLatencyMs) / float64(stats.TotalRequests)
	}
	
	costSaved := float64(stats.CacheHits) * stats.CostPerRequest
	uptime := time.Since(stats.StartTime).Round(time.Second)
	
	data := struct {
		Stats
		HitRate    float64
		AvgLatency float64
		CostSaved  float64
		Uptime     string
	}{
		Stats:      stats,
		HitRate:    hitRate,
		AvgLatency: avgLatency,
		CostSaved:  costSaved,
		Uptime:     uptime.String(),
	}
	
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
}

var tmpl = template.Must(template.New("dashboard").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Semantic Cache Gateway - Stats</title>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="5">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            color: #fff;
            min-height: 100vh;
            padding: 40px 20px;
        }
        .container { max-width: 900px; margin: 0 auto; }
        h1 { 
            text-align: center; 
            margin-bottom: 40px;
            font-size: 2.5em;
            background: linear-gradient(90deg, #00d9ff, #00ff88);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .grid { 
            display: grid; 
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .card {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 24px;
            text-align: center;
            border: 1px solid rgba(255,255,255,0.1);
            backdrop-filter: blur(10px);
        }
        .card-value {
            font-size: 2.5em;
            font-weight: bold;
            margin-bottom: 8px;
        }
        .card-label {
            color: #888;
            font-size: 0.9em;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        .hit-rate { color: #00ff88; }
        .cost-saved { color: #00d9ff; }
        .requests { color: #ff6b6b; }
        .latency { color: #ffd93d; }
        .footer {
            text-align: center;
            color: #666;
            margin-top: 40px;
            font-size: 0.85em;
        }
        .bar-container {
            background: rgba(255,255,255,0.1);
            border-radius: 10px;
            height: 20px;
            margin-top: 20px;
            overflow: hidden;
        }
        .bar-fill {
            height: 100%;
            background: linear-gradient(90deg, #00ff88, #00d9ff);
            border-radius: 10px;
            transition: width 0.5s ease;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 Semantic Cache Gateway</h1>
        
        <div class="grid">
            <div class="card">
                <div class="card-value hit-rate">{{printf "%.1f" .HitRate}}%</div>
                <div class="card-label">Cache Hit Rate</div>
                <div class="bar-container">
                    <div class="bar-fill" style="width: {{printf "%.0f" .HitRate}}%"></div>
                </div>
            </div>
            
            <div class="card">
                <div class="card-value cost-saved">${{printf "%.4f" .CostSaved}}</div>
                <div class="card-label">Cost Saved</div>
            </div>
            
            <div class="card">
                <div class="card-value requests">{{.TotalRequests}}</div>
                <div class="card-label">Total Requests</div>
            </div>
            
            <div class="card">
                <div class="card-value latency">{{printf "%.0f" .AvgLatency}}ms</div>
                <div class="card-label">Avg Latency</div>
            </div>
        </div>
        
        <div class="grid">
            <div class="card">
                <div class="card-value" style="color: #00ff88;">{{.CacheHits}}</div>
                <div class="card-label">Cache Hits</div>
            </div>
            
            <div class="card">
                <div class="card-value" style="color: #ffd93d;">{{.CacheMisses}}</div>
                <div class="card-label">Cache Misses</div>
            </div>
            
            <div class="card">
                <div class="card-value" style="color: #ff6b6b;">{{.Errors}}</div>
                <div class="card-label">Errors</div>
            </div>
            
            <div class="card">
                <div class="card-value" style="color: #888; font-size: 1.2em;">{{.Uptime}}</div>
                <div class="card-label">Uptime</div>
            </div>
        </div>
        
        <div class="footer">
            Auto-refreshes every 5 seconds • 
            <a href="/stats/json" style="color: #00d9ff;">JSON API</a>
            <div style="margin-top: 15px; color: #555;">
                Made with ❤️ by <span style="color: #00d9ff;">Vineet Loyer</span>
            </div>
        </div>
    </div>
</body>
</html>
`))

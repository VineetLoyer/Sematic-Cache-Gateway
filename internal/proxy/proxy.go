// Package proxy provides upstream LLM forwarding functionality.
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// UpstreamProxy forwards requests to the upstream LLM provider.
type UpstreamProxy interface {
	// Forward sends request to upstream LLM and returns response
	Forward(ctx context.Context, req *http.Request) (*http.Response, error)
}

// ProxyConfig holds configuration for the upstream proxy.
type ProxyConfig struct {
	// UpstreamURL is the base URL of the upstream LLM API
	UpstreamURL string
	// Timeout is the maximum duration for upstream requests
	Timeout time.Duration
	// APIKey is the optional server-side API key for upstream requests
	APIKey string
}

// DefaultTimeout is the default timeout for upstream requests.
const DefaultTimeout = 60 * time.Second

// Proxy implements UpstreamProxy for forwarding requests to the upstream LLM.
type Proxy struct {
	config     ProxyConfig
	client     *http.Client
	upstreamURL *url.URL
}

// New creates a new Proxy with the given configuration.
func New(config ProxyConfig) (*Proxy, error) {
	if config.UpstreamURL == "" {
		return nil, fmt.Errorf("upstream URL is required")
	}

	parsedURL, err := url.Parse(config.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL: %w", err)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &Proxy{
		config:     config,
		upstreamURL: parsedURL,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// Forward sends the request to the upstream LLM and returns the response.
// It preserves the original request headers and body.
func (p *Proxy) Forward(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Build the upstream URL by combining base URL with request path
	upstreamURL := p.buildUpstreamURL(req.URL.Path, req.URL.RawQuery)

	// Read the request body if present
	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body.Close()
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create the upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, req.Method, upstreamURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream request: %w", err)
	}

	// Copy headers from original request, preserving authentication and content type
	copyHeaders(req.Header, upstreamReq.Header)

	// If server-side API key is configured, use it instead of client's auth header
	if p.config.APIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	// Set Host header to upstream host
	upstreamReq.Host = p.upstreamURL.Host

	// Forward the request to upstream
	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}

	return resp, nil
}

// buildUpstreamURL constructs the full upstream URL from the request path.
func (p *Proxy) buildUpstreamURL(path, rawQuery string) string {
	u := *p.upstreamURL
	// Append the request path to the upstream base path
	// e.g., upstream "https://api.openai.com/v1" + path "/chat/completions"
	// becomes "https://api.openai.com/v1/chat/completions"
	if u.Path != "" && u.Path != "/" {
		u.Path = u.Path + path
	} else {
		u.Path = path
	}
	u.RawQuery = rawQuery
	return u.String()
}

// copyHeaders copies headers from src to dst, excluding hop-by-hop headers.
func copyHeaders(src, dst http.Header) {
	// Hop-by-hop headers that should not be forwarded
	hopByHopHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}

	for key, values := range src {
		if hopByHopHeaders[key] {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

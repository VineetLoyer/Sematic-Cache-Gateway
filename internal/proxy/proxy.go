package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type UpstreamProxy interface {
	Forward(ctx context.Context, req *http.Request) (*http.Response, error)
}

type ProxyConfig struct {
	UpstreamURL string
	Timeout     time.Duration
	APIKey      string
}

const DefaultTimeout = 60 * time.Second

type Proxy struct {
	config      ProxyConfig
	client      *http.Client
	upstreamURL *url.URL
}

// New creates a new upstream proxy with the given configuration.
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
		config:      config,
		upstreamURL: parsedURL,
		client:      &http.Client{Timeout: timeout},
	}, nil
}

// Forward sends the request to the upstream LLM and returns the response.
func (p *Proxy) Forward(ctx context.Context, req *http.Request) (*http.Response, error) {
	upstreamURL := p.buildUpstreamURL(req.URL.Path, req.URL.RawQuery)

	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body.Close()
		bodyReader = bytes.NewReader(bodyBytes)
	}

	upstreamReq, err := http.NewRequestWithContext(ctx, req.Method, upstreamURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream request: %w", err)
	}

	copyHeaders(req.Header, upstreamReq.Header)
	if p.config.APIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	upstreamReq.Host = p.upstreamURL.Host

	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	return resp, nil
}

func (p *Proxy) buildUpstreamURL(path, rawQuery string) string {
	u := *p.upstreamURL
	basePath := u.Path
	if basePath == "" || basePath == "/" {
		u.Path = path
	} else if strings.HasPrefix(path, basePath) {
		u.Path = path
	} else {
		u.Path = basePath + path
	}
	u.RawQuery = rawQuery
	return u.String()
}

func copyHeaders(src, dst http.Header) {
	hopByHopHeaders := map[string]bool{
		"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
		"Proxy-Authorization": true, "Te": true, "Trailers": true,
		"Transfer-Encoding": true, "Upgrade": true,
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

// Package npmjs is the library behind the npmjs command: the HTTP client,
// request shaping, and the typed data models for the npm registry.
//
// The npm Registry at registry.npmjs.org is open; no key required.
package npmjs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Host is the canonical npm registry hostname.
const Host = "registry.npmjs.org"

const (
	registryBase     = "https://" + Host
	defaultUserAgent = "npmjs-cli/dev (+https://github.com/tamnd/npmjs-cli)"
	defaultRate      = 200 * time.Millisecond
	defaultTimeout   = 30 * time.Second
	defaultRetries   = 3
)

// ErrNotFound is returned when the registry returns 404 or null for a package.
var ErrNotFound = errors.New("not found")

// Client talks to the npm Registry API.
type Client struct {
	http      *http.Client
	userAgent string
	rate      time.Duration
	retries   int
	mu        sync.Mutex
	last      time.Time
	UserAgent string
	Rate      time.Duration
	Retries   int
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	return &Client{
		http:      &http.Client{Timeout: defaultTimeout},
		userAgent: defaultUserAgent,
		rate:      defaultRate,
		retries:   defaultRetries,
		UserAgent: defaultUserAgent,
		Rate:      defaultRate,
		Retries:   defaultRetries,
	}
}

// SearchPackages queries /-/v1/search?text={query}&size={limit}.
func (c *Client) SearchPackages(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 250 {
		limit = 250
	}
	params := url.Values{}
	params.Set("text", query)
	params.Set("size", fmt.Sprintf("%d", limit))
	rawURL := registryBase + "/-/v1/search?" + params.Encode()

	var resp wireSearchResult
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(resp.Objects))
	for _, obj := range resp.Objects {
		out = append(out, SearchResult{
			Name:        obj.Package.Name,
			Version:     obj.Package.Version,
			Description: obj.Package.Description,
			Score:       obj.Score.Final,
			Date:        obj.Package.Date,
		})
	}
	return out, nil
}

// GetPackage fetches full package metadata from /{name}.
func (c *Client) GetPackage(ctx context.Context, name string) (*Package, error) {
	rawURL := registryBase + "/" + url.PathEscape(name)
	var doc wirePackage
	if err := c.getJSON(ctx, rawURL, &doc); err != nil {
		return nil, err
	}
	if doc.DistTags.Latest == "" {
		return nil, fmt.Errorf("package %q: no latest version: %w", name, ErrNotFound)
	}
	return &Package{
		Name:        doc.Name,
		Version:     doc.DistTags.Latest,
		Description: doc.Description,
		License:     doc.License,
		Homepage:    doc.Homepage,
		Repository:  extractRepo(doc.Repository),
		Keywords:    doc.Keywords,
		Author:      extractAuthor(doc.Author),
	}, nil
}

// GetVersion fetches metadata for a specific version from /{name}/{version}.
func (c *Client) GetVersion(ctx context.Context, name, version string) (*PackageVersion, error) {
	rawURL := registryBase + "/" + url.PathEscape(name) + "/" + url.PathEscape(version)
	var doc wireVersion
	if err := c.getJSON(ctx, rawURL, &doc); err != nil {
		return nil, err
	}
	return &PackageVersion{
		Name:         doc.Name,
		Version:      doc.Version,
		License:      doc.License,
		Main:         doc.Main,
		UnpackedSize: doc.Dist.UnpackedSize,
	}, nil
}

// ─── wire types ──────────────────────────────────────────────────────────────

// wirePackage is the full package document from /{name}.
type wirePackage struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	License     string          `json:"license"`
	Homepage    string          `json:"homepage"`
	Repository  json.RawMessage `json:"repository"` // can be string or {"type":"git","url":"..."}
	Keywords    []string        `json:"keywords"`
	Author      json.RawMessage `json:"author"` // can be string or {"name":"...","email":"..."}
	DistTags    struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
	Versions map[string]struct{} `json:"versions"` // just keys, don't decode values
}

// wireVersion is the response from /{name}/{version}.
type wireVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license"`
	Main    string `json:"main"`
	Dist    struct {
		UnpackedSize int64 `json:"unpackedSize"`
	} `json:"dist"`
}

// wireSearchResult is the top-level response from /-/v1/search.
type wireSearchResult struct {
	Objects []struct {
		Package struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
			Date        string `json:"date"`
		} `json:"package"`
		Score struct {
			Final float64 `json:"final"`
		} `json:"score"`
	} `json:"objects"`
	Total int `json:"total"`
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// extractRepo extracts repository URL from the raw JSON field (string or object).
func extractRepo(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.URL
	}
	return ""
}

// extractAuthor extracts an author name from the raw JSON field (string or object).
func extractAuthor(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Name
	}
	return ""
}

// ─── HTTP internals ──────────────────────────────────────────────────────────

func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) == "null" {
		return ErrNotFound
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	retries := c.retries
	if retries <= 0 {
		retries = defaultRetries
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = c.userAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, fmt.Errorf("http 404: %w", ErrNotFound)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	rate := c.Rate
	if rate <= 0 {
		rate = c.rate
	}
	if rate <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

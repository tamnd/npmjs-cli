// Package npmjs is the library behind the npmjs command: the HTTP client,
// request shaping, and the typed data models for the npm registry.
//
// Two APIs: the npm Registry at registry.npmjs.org for package metadata, and
// the Downloads API at api.npmjs.org for install counts. Both are open, no
// key required.
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

const (
	registryBase  = "https://registry.npmjs.org"
	downloadsBase = "https://api.npmjs.org"
)

// DefaultUserAgent identifies the client to the npm registry.
const DefaultUserAgent = "npmjs/dev (+https://github.com/tamnd/npmjs-cli)"

// ErrNotFound is returned when the registry returns 404 or null for a package.
var ErrNotFound = errors.New("not found")

// Config holds constructor parameters.
type Config struct {
	UserAgent string
	Rate      time.Duration
	Retries   int
	Workers   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults for the npm registry.
func DefaultConfig() Config {
	return Config{
		UserAgent: DefaultUserAgent,
		Rate:      100 * time.Millisecond,
		Retries:   3,
		Workers:   8,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the npm Registry and Downloads APIs.
type Client struct {
	httpClient *http.Client
	userAgent  string
	rate       time.Duration
	retries    int
	workers    int
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		userAgent:  cfg.UserAgent,
		rate:       cfg.Rate,
		retries:    cfg.Retries,
		workers:    cfg.Workers,
	}
}

// get fetches a URL with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
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
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate <= 0 {
		return
	}
	if wait := c.rate - time.Since(c.last); wait > 0 {
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

// getJSON fetches and JSON-decodes into v. Returns ErrNotFound when the body is null.
func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "null" {
		return ErrNotFound
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

// Search queries the npm registry search endpoint.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Package, error) {
	size := limit
	if size <= 0 {
		size = 20
	}
	if size > 250 {
		size = 250
	}

	params := url.Values{}
	params.Set("text", query)
	params.Set("size", fmt.Sprintf("%d", size))

	rawURL := registryBase + "/-/v1/search?" + params.Encode()
	var resp searchResp
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return nil, err
	}
	return wireSearchToPackages(resp, limit), nil
}

// Package fetches full package metadata and returns one Package record.
func (c *Client) Package(ctx context.Context, name string) (Package, error) {
	rawURL := registryBase + "/" + url.PathEscape(name)
	var doc pkgDoc
	if err := c.getJSON(ctx, rawURL, &doc); err != nil {
		return Package{}, err
	}
	if doc.DistTags["latest"] == "" {
		return Package{}, fmt.Errorf("package %q: no latest version: %w", name, ErrNotFound)
	}
	return wireDocToPackage(doc), nil
}

// Versions fetches the version history for a package and returns VersionInfo records.
func (c *Client) Versions(ctx context.Context, name string, limit int) ([]VersionInfo, error) {
	rawURL := registryBase + "/" + url.PathEscape(name)
	var doc pkgDoc
	if err := c.getJSON(ctx, rawURL, &doc); err != nil {
		return nil, err
	}
	return wireDocToVersions(doc, limit), nil
}

// Deps fetches the dependencies of the latest version of a package.
func (c *Client) Deps(ctx context.Context, name string) ([]Dep, error) {
	rawURL := registryBase + "/" + url.PathEscape(name)
	var doc pkgDoc
	if err := c.getJSON(ctx, rawURL, &doc); err != nil {
		return nil, err
	}
	latest := doc.DistTags["latest"]
	if latest == "" {
		return nil, fmt.Errorf("package %q: no latest version: %w", name, ErrNotFound)
	}
	manifest, ok := doc.Versions[latest]
	if !ok {
		return nil, fmt.Errorf("package %q version %q not found: %w", name, latest, ErrNotFound)
	}
	return wireManifestToDeps(manifest), nil
}

// Downloads fetches download statistics for a package and period.
// period is one of: last-day, last-week, last-month, last-year.
func (c *Client) Downloads(ctx context.Context, name string, period string) (DownloadStat, error) {
	rawURL := downloadsBase + "/downloads/point/" + period + "/" + url.PathEscape(name)
	var resp dlResp
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return DownloadStat{}, err
	}
	return wireDownloadToStat(resp, period), nil
}

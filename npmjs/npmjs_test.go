package npmjs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.Rate = 0
	c := NewClient(cfg)
	// point both base URLs at the test server by overriding getJSON via a
	// wrapper: tests pass a full URL so base URLs do not matter here.
	_ = srv
	return c
}

func TestGetSendsUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(`"hello"`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `"hello"` {
		t.Errorf("body = %q", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`"recovered"`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClient(cfg)

	start := time.Now()
	body, err := c.get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `"recovered"` {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetNullReturnsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var v any
	err := c.getJSON(context.Background(), srv.URL, &v)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestGet404ReturnsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.get(context.Background(), srv.URL)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

const searchJSON = `{
  "objects": [
    {
      "package": {
        "name": "express",
        "version": "4.18.2",
        "description": "Fast, unopinionated web framework",
        "date": "2022-10-08T10:28:48.638Z",
        "links": {"npm": "https://www.npmjs.com/package/express"},
        "publisher": {"username": "wesleytodd"}
      },
      "score": {"final": 0.97, "detail": {"quality": 0.96, "popularity": 0.98, "maintenance": 0.99}}
    }
  ],
  "total": 1
}`

func TestSearchDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)

	var resp searchResp
	if err := c.getJSON(context.Background(), srv.URL, &resp); err != nil {
		t.Fatal(err)
	}
	pkgs := wireSearchToPackages(resp, 10)
	if len(pkgs) != 1 {
		t.Fatalf("got %d packages, want 1", len(pkgs))
	}
	if pkgs[0].Name != "express" {
		t.Errorf("name = %q, want express", pkgs[0].Name)
	}
	if pkgs[0].Version != "4.18.2" {
		t.Errorf("version = %q, want 4.18.2", pkgs[0].Version)
	}
	if pkgs[0].Rank != 1 {
		t.Errorf("rank = %d, want 1", pkgs[0].Rank)
	}
	if pkgs[0].URL != "https://www.npmjs.com/package/express" {
		t.Errorf("url = %q", pkgs[0].URL)
	}
}

const pkgDocJSON = `{
  "name": "lodash",
  "description": "Lodash modular utilities.",
  "dist-tags": {"latest": "4.17.21"},
  "versions": {
    "4.17.21": {
      "name": "lodash",
      "version": "4.17.21",
      "description": "Lodash modular utilities.",
      "dependencies": {"some-dep": "^1.0.0"},
      "devDependencies": {"some-dev": "^2.0.0"}
    },
    "4.17.20": {
      "name": "lodash",
      "version": "4.17.20",
      "description": "Lodash modular utilities."
    }
  },
  "time": {
    "created": "2012-04-05T00:00:00.000Z",
    "modified": "2021-02-20T15:42:16.891Z",
    "4.17.21": "2021-02-20T15:42:16.891Z",
    "4.17.20": "2020-10-14T08:00:00.000Z"
  },
  "author": {"name": "John-David Dalton"},
  "license": "MIT"
}`

func TestPackageDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pkgDocJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var doc pkgDoc
	if err := c.getJSON(context.Background(), srv.URL, &doc); err != nil {
		t.Fatal(err)
	}
	pkg := wireDocToPackage(doc)
	if pkg.Name != "lodash" {
		t.Errorf("name = %q, want lodash", pkg.Name)
	}
	if pkg.Version != "4.17.21" {
		t.Errorf("version = %q, want 4.17.21", pkg.Version)
	}
	if pkg.Author != "John-David Dalton" {
		t.Errorf("author = %q, want John-David Dalton", pkg.Author)
	}
	if pkg.License != "MIT" {
		t.Errorf("license = %q, want MIT", pkg.License)
	}
	if pkg.URL != "https://www.npmjs.com/package/lodash" {
		t.Errorf("url = %q", pkg.URL)
	}
}

func TestVersionsDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pkgDocJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var doc pkgDoc
	if err := c.getJSON(context.Background(), srv.URL, &doc); err != nil {
		t.Fatal(err)
	}
	versions := wireDocToVersions(doc, 0)
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	// newest first
	if versions[0].Version != "4.17.21" {
		t.Errorf("versions[0] = %q, want 4.17.21", versions[0].Version)
	}
}

func TestDepsDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pkgDocJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var doc pkgDoc
	if err := c.getJSON(context.Background(), srv.URL, &doc); err != nil {
		t.Fatal(err)
	}
	manifest := doc.Versions["4.17.21"]
	deps := wireManifestToDeps(manifest)
	if len(deps) != 2 {
		t.Fatalf("got %d deps, want 2", len(deps))
	}
	// runtime should come first
	if deps[0].Kind != "runtime" {
		t.Errorf("deps[0].Kind = %q, want runtime", deps[0].Kind)
	}
	if deps[1].Kind != "dev" {
		t.Errorf("deps[1].Kind = %q, want dev", deps[1].Kind)
	}
}

const dlJSON = `{"downloads":328844422,"start":"2024-01-01","end":"2024-01-31","package":"express"}`

func TestDownloadsDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dlJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var resp dlResp
	if err := c.getJSON(context.Background(), srv.URL, &resp); err != nil {
		t.Fatal(err)
	}
	stat := wireDownloadToStat(resp, "last-month")
	if stat.Package != "express" {
		t.Errorf("package = %q, want express", stat.Package)
	}
	if stat.Downloads != 328844422 {
		t.Errorf("downloads = %d, want 328844422", stat.Downloads)
	}
	if stat.Period != "last-month" {
		t.Errorf("period = %q, want last-month", stat.Period)
	}
}

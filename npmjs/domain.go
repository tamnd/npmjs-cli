package npmjs

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the npm registry as a kit Domain: a driver that a
// multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/npmjs-cli/npmjs"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then
// dereferences npmjs:// URIs by routing to the operations Register installs.
func init() { kit.Register(Domain{}) }

// Domain is the npm registry driver. It carries no state; the per-run client
// is built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "npmjs",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "npmjs",
			Short:  "Browse the npm JavaScript package registry",
			Long: `Browse the npm JavaScript package registry.

npmjs reads public npm registry data over plain HTTPS, shapes it into clean
records, and prints output that pipes into the rest of your tools. No API key,
nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/npmjs-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read",
		Summary: "Search npm packages",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}}}, opSearch)

	kit.Handle(app, kit.OpMeta{Name: "package", Group: "read", Single: true,
		Summary: "Get package info", URIType: "package", Resolver: true,
		Args: []kit.Arg{{Name: "name", Help: "package name (e.g. react, express, @types/node)"}}}, opPackage)

	kit.Handle(app, kit.OpMeta{Name: "version", Group: "read", Single: true,
		Summary: "Get specific version info", URIType: "package",
		Args: []kit.Arg{
			{Name: "name", Help: "package name"},
			{Name: "version", Help: "version (e.g. 18.3.1, latest)"},
		}}, opVersion)
}

// newClient builds the npm registry client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	return c, nil
}

// ─── inputs ──────────────────────────────────────────────────────────────────

type searchInput struct {
	Query  string  `kit:"arg" help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"10"`
	Client *Client `kit:"inject"`
}

type packageInput struct {
	Name   string  `kit:"arg" help:"package name (e.g. react, express, @types/node)"`
	Client *Client `kit:"inject"`
}

type versionInput struct {
	Name    string  `kit:"arg" help:"package name"`
	Version string  `kit:"arg" help:"version (e.g. 18.3.1, latest)"`
	Client  *Client `kit:"inject"`
}

// ─── handlers ────────────────────────────────────────────────────────────────

func opSearch(ctx context.Context, in searchInput, emit func(SearchResult) error) error {
	results, err := in.Client.SearchPackages(ctx, in.Query, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, r := range results {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func opPackage(ctx context.Context, in packageInput, emit func(*Package) error) error {
	pkg, err := in.Client.GetPackage(ctx, in.Name)
	if err != nil {
		return mapErr(err)
	}
	return emit(pkg)
}

func opVersion(ctx context.Context, in versionInput, emit func(*PackageVersion) error) error {
	v, err := in.Client.GetVersion(ctx, in.Name, in.Version)
	if err != nil {
		return mapErr(err)
	}
	return emit(v)
}

// ─── Resolver ────────────────────────────────────────────────────────────────

// Classify turns any accepted input into the canonical (type, id).
// Scoped packages (starting with @) → ("package", name).
// Valid npm names (lowercase, hyphens, digits, dots) → ("package", name).
// Otherwise → ("query", input).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty npm package reference")
	}
	if isNPMName(input) {
		return "package", input, nil
	}
	return "query", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "package":
		return "https://www.npmjs.com/package/" + id, nil
	case "query":
		return "https://www.npmjs.com/search?q=" + id, nil
	default:
		return "", errs.Usage("npmjs has no resource type %q", uriType)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// isNPMName returns true when input looks like a valid npm package name.
// Scoped packages start with @ (e.g. @types/node).
// Unscoped: lowercase letters, digits, hyphens, underscores, dots.
func isNPMName(s string) bool {
	if s == "" {
		return false
	}
	// scoped: @scope/name
	if strings.HasPrefix(s, "@") {
		rest := s[1:]
		slash := strings.IndexByte(rest, '/')
		if slash < 1 {
			return false
		}
		scope := rest[:slash]
		name := rest[slash+1:]
		return isNPMPart(scope) && isNPMPart(name)
	}
	return isNPMPart(s)
}

// isNPMPart checks that all runes are valid npm name characters.
func isNPMPart(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return errs.NotFound("%s", err.Error())
	}
	return err
}

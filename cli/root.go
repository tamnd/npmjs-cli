// Package cli builds the npmjs command tree on top of the npmjs library.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tamnd/npmjs-cli/npmjs"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// exit codes.
const (
	exitError  = 1
	exitUsage  = 2
	exitNoData = 3
)

// ExitError carries a process exit code up to main.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

func codeError(code int, err error) error { return &ExitError{Code: code, Err: err} }

// App holds shared state threaded through every command.
type App struct {
	client *npmjs.Client

	// client config flags
	rate      time.Duration
	timeout   time.Duration
	retries   int
	userAgent string

	output   string
	fields   []string
	noHeader bool
	template string
	limit    int
	quiet    bool
}

// Root builds the root command and its subtree.
func Root() *cobra.Command {
	app := &App{
		rate:      200 * time.Millisecond,
		timeout:   30 * time.Second,
		retries:   3,
		userAgent: "",
	}

	root := &cobra.Command{
		Use:   "npmjs",
		Short: "Browse the npm JavaScript package registry",
		Long: `npmjs reads the npm JavaScript package registry through its public APIs.
No API key or authentication is required. It returns records as table, JSON,
JSONL, CSV, TSV, or URLs.

npmjs is an independent tool and is not affiliated with npm, Inc. or GitHub.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return app.setup()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&app.output, "output", "o", "auto", "output: table|json|jsonl|csv|tsv|url|raw (auto=table on TTY, jsonl piped)")
	pf.StringSliceVar(&app.fields, "fields", nil, "comma-separated columns to include")
	pf.BoolVar(&app.noHeader, "no-header", false, "omit the header row in table/csv/tsv")
	pf.StringVar(&app.template, "template", "", "Go text/template applied per record")
	pf.IntVarP(&app.limit, "limit", "n", 0, "limit number of records (0 = command default)")
	pf.BoolVarP(&app.quiet, "quiet", "q", false, "suppress progress on stderr")

	pf.DurationVar(&app.rate, "delay", app.rate, "minimum spacing between requests")
	pf.DurationVar(&app.timeout, "timeout", app.timeout, "per-request timeout")
	pf.IntVar(&app.retries, "retries", app.retries, "retry attempts on 429/5xx")
	pf.StringVar(&app.userAgent, "user-agent", app.userAgent, "User-Agent sent with each request")

	root.AddCommand(
		app.searchCmd(),
		app.packageCmd(),
		app.versionCmd(),
		newVersionCmd(),
	)
	return root
}

func (a *App) setup() error {
	if a.output == "" || a.output == "auto" {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			a.output = string(FormatTable)
		} else {
			a.output = string(FormatJSONL)
		}
	}
	if !Format(a.output).Valid() {
		return codeError(exitUsage, fmt.Errorf("unknown output format %q", a.output))
	}
	c := npmjs.NewClient()
	if a.userAgent != "" {
		c.UserAgent = a.userAgent
	}
	if a.rate > 0 {
		c.Rate = a.rate
	}
	if a.retries > 0 {
		c.Retries = a.retries
	}
	a.client = c
	return nil
}

func (a *App) render(records any) error {
	r := NewRenderer(os.Stdout, Format(a.output), a.fields, a.noHeader, a.template)
	return r.Render(records)
}

func (a *App) renderOrEmpty(records any, n int) error {
	if err := a.render(records); err != nil {
		return err
	}
	if n == 0 {
		return codeError(exitNoData, nil)
	}
	return nil
}

func (a *App) progressf(format string, args ...any) {
	if a.quiet {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func mapFetchErr(err error) error {
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return codeError(exitNoData, err)
	}
	return codeError(exitError, err)
}

func (a *App) effectiveLimit(def int) int {
	if a.limit > 0 {
		return a.limit
	}
	return def
}

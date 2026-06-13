package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) versionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "versions <name>",
		Short: "List published versions of a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			n := a.effectiveLimit(0)
			a.progressf("fetching versions for %q...", name)
			versions, err := a.client.Versions(cmd.Context(), name, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(versions, len(versions))
		},
	}
}

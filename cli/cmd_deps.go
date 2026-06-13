package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) depsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deps <name>",
		Short: "List dependencies of the latest version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			a.progressf("fetching dependencies for %q...", name)
			deps, err := a.client.Deps(cmd.Context(), name)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(deps, len(deps))
		},
	}
}

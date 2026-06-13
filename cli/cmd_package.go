package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) packageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "package <name>",
		Short: "Show package metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			a.progressf("fetching package %q...", name)
			pkg, err := a.client.Package(cmd.Context(), name)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.render(pkg)
		},
	}
}

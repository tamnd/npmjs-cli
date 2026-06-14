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
			pkg, err := a.client.GetPackage(cmd.Context(), name)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.render(pkg)
		},
	}
}

func (a *App) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version <name> <version>",
		Short: "Show a specific version's metadata",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, ver := args[0], args[1]
			a.progressf("fetching %s@%s...", name, ver)
			v, err := a.client.GetVersion(cmd.Context(), name, ver)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.render(v)
		},
	}
}

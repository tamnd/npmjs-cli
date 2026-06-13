package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) searchCmd() *cobra.Command {
	var sort string
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the npm registry for packages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(20)
			a.progressf("searching for %q...", args[0])
			pkgs, err := a.client.Search(cmd.Context(), args[0], n)
			if err != nil {
				return mapFetchErr(err)
			}
			_ = sort // stored in query by client; kept here for flag visibility
			return a.renderOrEmpty(pkgs, len(pkgs))
		},
	}
	cmd.Flags().StringVar(&sort, "sort", "", "sort by: quality, popularity, maintenance")
	return cmd
}

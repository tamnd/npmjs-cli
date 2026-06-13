package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/npmjs-cli/npmjs"
)

func (a *App) downloadsCmd() *cobra.Command {
	var period string
	cmd := &cobra.Command{
		Use:   "downloads <name>",
		Short: "Show download statistics for a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if period != "" {
				a.progressf("fetching %s downloads for %q...", period, name)
				stat, err := a.client.Downloads(cmd.Context(), name, period)
				if err != nil {
					return mapFetchErr(err)
				}
				return a.render([]npmjs.DownloadStat{stat})
			}
			// default: fetch last-week, last-month, last-year
			periods := []string{"last-week", "last-month", "last-year"}
			a.progressf("fetching download stats for %q...", name)
			var stats []npmjs.DownloadStat
			for _, p := range periods {
				stat, err := a.client.Downloads(cmd.Context(), name, p)
				if err != nil {
					return mapFetchErr(err)
				}
				stats = append(stats, stat)
			}
			return a.renderOrEmpty(stats, len(stats))
		},
	}
	cmd.Flags().StringVar(&period, "period", "", "time window: last-week, last-month, last-year")
	return cmd
}

package cli

import "github.com/spf13/cobra"

func newShopsCmd(o *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "shops",
		Short: "List configured shops",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := o.load()
			if err != nil {
				return err
			}
			if *o.jsonOut {
				type shopView struct {
					Name    string `json:"name"`
					URL     string `json:"url"`
					Default bool   `json:"default"`
				}
				views := make([]shopView, 0, len(cfg.Shops))
				for _, s := range cfg.Shops {
					views = append(views, shopView{Name: s.Name, URL: s.URL, Default: s.Name == cfg.DefaultName()})
				}
				return RenderJSON(cmd.OutOrStdout(), views)
			}
			return RenderShops(cmd.OutOrStdout(), cfg)
		},
	}
}

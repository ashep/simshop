package cli

import "github.com/spf13/cobra"

// globalOpts holds pointers to the persistent flags shared by all commands.
type globalOpts struct {
	shop    *string
	cfgPath *string
	jsonOut *bool
}

// load reads the config from the resolved path.
func (o *globalOpts) load() (*Config, error) {
	path := *o.cfgPath
	if path == "" {
		p, err := DefaultConfigPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return LoadConfig(path)
}

// selectShop resolves the shop chosen by --shop (or the default one).
func (o *globalOpts) selectShop() (*Shop, error) {
	cfg, err := o.load()
	if err != nil {
		return nil, err
	}
	return cfg.Select(*o.shop)
}

// client resolves the selected shop and builds an API client for it.
func (o *globalOpts) client() (*Client, error) {
	s, err := o.selectShop()
	if err != nil {
		return nil, err
	}
	return NewClient(s.URL, s.APIKey), nil
}

// NewRootCmd builds the root simshop command tree.
func NewRootCmd() *cobra.Command {
	opts := &globalOpts{shop: new(string), cfgPath: new(string), jsonOut: new(bool)}

	root := &cobra.Command{
		Use:           "simshop",
		Short:         "CLI for the simshop backend (runs the interactive UI when no command is given)",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd.Context(), opts)
		},
	}
	root.PersistentFlags().StringVar(opts.shop, "shop", "", "shop name from ~/.simshop.yaml (default: the default shop)")
	root.PersistentFlags().StringVar(opts.cfgPath, "config", "", "config file path (default: ~/.simshop.yaml)")
	root.PersistentFlags().BoolVar(opts.jsonOut, "json", false, "output raw JSON")

	root.AddCommand(newOrderCmd(opts))
	root.AddCommand(newShopsCmd(opts))
	root.AddCommand(newTUICmd(opts))
	return root
}

package app

import "cli-navidrome-helper/internal/config"

// Options captures user-supplied CLI parameters before config/env enrichment.
type Options struct {
	Artist   string
	URL      string
	TmpDir   string
	KeepTemp bool
	DryRun   bool
}

// Run is the entry point for the import workflow.
func Run(opts Options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	return newRunner(cfg, opts).Execute()
}

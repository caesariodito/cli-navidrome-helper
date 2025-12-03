package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"cli-navidrome-helper/internal/app"
)

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := app.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags(args []string) (app.Options, error) {
	fs := flag.NewFlagSet("nd-import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	artist := fs.String("artist", "", "Artist folder name to group tracks (required)")
	url := fs.String("url", "", "Pixeldrain download URL or ID (required)")
	tmpDir := fs.String("tmp-dir", "", "Temporary directory override")
	keepTemp := fs.Bool("keep-temp", false, "Keep downloaded and extracted files instead of cleanup")
	dryRun := fs.Bool("dry-run", false, "Validate and plan actions without writing files")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "  %s --artist <name> --url <pixeldrain-url> [options]\n", os.Args[0])
		fmt.Fprintf(fs.Output(), "  %s \"<artist>\" \"<pixeldrain-url>\" [options]\n\n", os.Args[0])
		fmt.Fprintln(fs.Output(), "Environment: NAVIDROME_MUSIC_PATH is required; UNNEEDED_FILES and PIXELDRAIN_TOKEN are optional.")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return app.Options{}, err
	}

	// Support positional args: <artist> <url>
	positional := fs.Args()
	if strings.TrimSpace(*artist) == "" && len(positional) >= 1 {
		*artist = positional[0]
	}
	if strings.TrimSpace(*url) == "" && len(positional) >= 2 {
		*url = positional[1]
	}

	var missing []string
	if strings.TrimSpace(*artist) == "" {
		missing = append(missing, "--artist")
	}
	if strings.TrimSpace(*url) == "" {
		missing = append(missing, "--url")
	}
	if len(missing) > 0 {
		fs.Usage()
		return app.Options{}, fmt.Errorf("missing required flag(s): %s", strings.Join(missing, ", "))
	}

	return app.Options{
		Artist:   strings.TrimSpace(*artist),
		URL:      strings.TrimSpace(*url),
		TmpDir:   strings.TrimSpace(*tmpDir),
		KeepTemp: *keepTemp,
		DryRun:   *dryRun,
	}, nil
}

// Package list implements `klanky project list`. It prints the projects
// registered in .klankyrc.json. Honors --output / default_output.
package list

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/config"
)

// Options holds the parsed flag set.
type Options struct {
	Output     string
	ConfigPath string
}

// NewCmdList returns the cobra command.
func NewCmdList(cfgPath string) *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List klanky-registered projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.ConfigPath = cfgPath
			return RunProjectList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "",
		"output mode: text|json (default: from default_output, or text)")
	return cmd
}

// row is one line in the listing.
type row struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// RunProjectList is the testable entry point.
func RunProjectList(opts Options, out io.Writer) error {
	cfg, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	mode, err := config.ResolveOutput(cfg, opts.Output)
	if err != nil {
		return err
	}

	rows := make([]row, 0, len(cfg.Projects))
	for slug, p := range cfg.Projects {
		rows = append(rows, row{Slug: slug, Title: p.Title, URL: p.URL})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Slug < rows[j].Slug })

	switch mode {
	case config.OutputJSON:
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	default:
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SLUG\tTITLE\tURL")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Slug, r.Title, r.URL)
		}
		return tw.Flush()
	}
}

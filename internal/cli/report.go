package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/report"
	"github.com/spf13/cobra"
)

func newReport() *cobra.Command {
	var (
		from   string
		to     string
		format string
		output string
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Export packets in a date range as an auditor-readable CSV or PDF",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}

			fmtLower := strings.ToLower(format)
			if fmtLower != "csv" && fmtLower != "pdf" {
				return fmt.Errorf(`--format must be "pdf" or "csv".`)
			}

			fromT, err := report.ParseDate(from, "from")
			if err != nil {
				return err
			}
			toT, err := report.ParseDate(to, "to")
			if err != nil {
				return err
			}
			if fromT.After(toT) {
				return fmt.Errorf("--from must be on or before --to.")
			}

			all, err := packet.ListReachable(cwd)
			if err != nil {
				return err
			}
			inRange := report.FilterByDate(all, report.DateRange{From: fromT, To: toT})
			rows := report.ToRows(inRange)
			title := fmt.Sprintf("Entrypoint report %s → %s", from, to)

			if fmtLower == "csv" {
				csv := report.ToCSV(rows)
				if output != "" {
					if err := os.WriteFile(output, []byte(csv), 0o644); err != nil {
						return err
					}
					fmt.Printf("Wrote %d packet%s to %s\n", len(rows), plural(len(rows)), output)
				} else {
					fmt.Print(csv)
				}
				return nil
			}

			pdf := report.ToPDF(rows, title)
			outPath := output
			if outPath == "" {
				outPath = "entrypoint-report.pdf"
			}
			if err := os.WriteFile(outPath, pdf, 0o644); err != nil {
				return err
			}
			fmt.Printf("Wrote %d packet%s to %s\n", len(rows), plural(len(rows)), outPath)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&from, "from", "", "start date (YYYY-MM-DD), inclusive")
	f.StringVar(&to, "to", "", "end date (YYYY-MM-DD), inclusive")
	f.StringVar(&format, "format", "csv", "pdf or csv")
	f.StringVar(&output, "output", "", "write to file (default: stdout for csv, entrypoint-report.pdf for pdf)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

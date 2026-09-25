package sample

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available sample applications",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	names := make([]string, 0, len(catalog))
	for n := range catalog {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := catalog[name]
		langs := make([]string, 0, len(entry.Repos))
		for l := range entry.Repos {
			langs = append(langs, l)
		}
		sort.Strings(langs)

		fmt.Printf("%-20s %s\n", name, entry.Description)
		fmt.Printf("%-20s languages: %v\n\n", "", langs)
	}
	return nil
}

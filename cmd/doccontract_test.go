package cmd

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// knownDrift lists "command path::flag" entries intentionally not yet reconciled.
// REMOVE entries here as the underlying drift is fixed.
//   - "location create::subaccount": tracked by the --site→--subaccount rename
//     spec (docs/superpowers/specs/2026-05-07-subaccount-rename.md). Remove when
//     that PR lands.
var knownDrift = map[string]bool{
	"location create::subaccount": true,
}

// knownDriftCommands lists command paths documented in tables that are known
// rename-related drift and intentionally not yet reconciled.
// REMOVE entries here as the underlying drift is fixed.
var knownDriftCommands = map[string]bool{}

// bandUsageRe captures everything after "band " to end of line (GREEDY — a
// non-greedy capture would stop at the first space and truncate multi-word
// command paths like "message send" down to "message"). Tokenization below
// then trims the capture down to the command path.
var bandUsageRe = regexp.MustCompile(`\bband ([a-z].*)$`)
var flagRe = regexp.MustCompile(`--([a-z][\w-]*)`)

// backtickBandRe matches a backtick-quoted span whose content starts with "band ":
// e.g. `band app list` or `band call get <id>`. Used to extract the command
// in column 1 of markdown table rows.
var backtickBandRe = regexp.MustCompile("`(band [^`]+)`")

// commandTokenRe matches a token that can be part of a command path: a
// lowercase command/subcommand name. Flags (--x), placeholders (<x>/[x]),
// IDs (c-abc123), phone numbers (+1...), and shell operators all fail to match,
// so the first non-matching token ends the command path.
var commandTokenRe = regexp.MustCompile(`^[a-z][a-z-]*$`)

// resolveCommand walks rootCmd by the command-path tokens, descending into
// subcommands. It returns the deepest command matched and how many leading
// tokens were matched as ACTUAL subcommands. matched==0 means the first token
// is not a real subcommand of `band`.
func resolveCommand(path []string) (cmd *cobra.Command, matched int) {
	cur := rootCmd
	for _, tok := range path {
		next, _, err := cur.Find([]string{tok})
		if err != nil || next == cur {
			break // positional arg, or (at depth 0) a bogus command name
		}
		cur = next
		matched++
	}
	return cur, matched
}

func flagExists(c *cobra.Command, name string) bool {
	if c.Flags().Lookup(name) != nil {
		return true
	}
	if c.InheritedFlags().Lookup(name) != nil {
		return true
	}
	return rootCmd.PersistentFlags().Lookup(name) != nil
}

func TestDocumentedCommandsAndFlagsExist(t *testing.T) {
	for _, doc := range []string{"../README.md", "../AGENTS.md"} {
		raw, err := os.ReadFile(doc)
		if err != nil {
			t.Fatalf("reading %s: %v", doc, err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			// Table rows: only the command in column 1 is validated for existence;
			// description-column flags are intentionally NOT checked (they're prose
			// mentions, not usage). This avoids false positives from flags named in
			// the "What it does" column being attributed to the column-1 command.
			if strings.HasPrefix(strings.TrimSpace(line), "|") {
				// Split on "|" and take the first non-empty cell (column 1).
				var col1 string
				for _, cell := range strings.Split(line, "|") {
					if strings.TrimSpace(cell) != "" {
						col1 = cell
						break
					}
				}
				// Look for a backtick-quoted `band ...` span in column 1 only.
				bm := backtickBandRe.FindStringSubmatch(col1)
				if bm == nil {
					continue // not a command row (e.g. header or non-command cell)
				}
				// bm[1] is the content inside the backticks, e.g. "band app list"
				// Strip the leading "band " and tokenize the command path.
				rest := strings.TrimPrefix(bm[1], "band ")
				fields := strings.Fields(rest)
				var path []string
				for _, f := range fields {
					if !commandTokenRe.MatchString(f) {
						break
					}
					path = append(path, f)
				}
				if len(path) == 0 {
					continue
				}
				cmdName := strings.Join(path, " ")
				if knownDriftCommands[cmdName] {
					continue
				}
				_, matched := resolveCommand(path)
				if matched == 0 {
					t.Errorf("%s documents `band %s …` but %q is not a command under `band`", doc, cmdName, path[0])
				}
				continue
			}

			m := bandUsageRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			// Restrict flag extraction to the text captured after "band " and
			// trim at the first backtick — commands in inline code spans like
			// `MEDIA_URL=$(band ... image.png)` then pass to `--flag` would
			// otherwise attribute the trailing flag to the wrong command.
			capture := m[1]
			if idx := strings.IndexByte(capture, '`'); idx >= 0 {
				capture = capture[:idx]
			}

			// Command path = leading command-name tokens before the first
			// flag/placeholder/arg.
			fields := strings.Fields(capture)
			var path []string
			for _, f := range fields {
				if !commandTokenRe.MatchString(f) {
					break
				}
				path = append(path, f)
			}
			if len(path) == 0 {
				continue
			}
			cmd, matched := resolveCommand(path)
			cmdName := strings.Join(path, " ")
			if matched == 0 {
				t.Errorf("%s documents `band %s …` but %q is not a command under `band`", doc, cmdName, path[0])
				continue
			}
			for _, fm := range flagRe.FindAllStringSubmatch(capture, -1) {
				flag := fm[1]
				// Cobra auto-injects --help on every command; skip it.
				if flag == "help" {
					continue
				}
				if knownDrift[cmdName+"::"+flag] {
					continue
				}
				if !flagExists(cmd, flag) {
					t.Errorf("%s documents `band %s --%s` but that flag does not exist on command %q",
						doc, cmdName, flag, cmd.CommandPath())
				}
			}
		}
	}
}

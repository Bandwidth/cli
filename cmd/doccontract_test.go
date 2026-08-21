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
//   - "tendlc numbers::campaign-id" and "tendlc numbers::status": the legacy
//     `band tendlc numbers` command (and its --campaign-id/--status flags) was
//     removed in the 10DLC PR5 cutover (task 3 of
//     .superpowers/sdd/2026-08-21-tendlc-pr5-cutover); AGENTS.md's doc sweep is
//     task 6 of that same plan. Remove when that PR lands.
var knownDrift = map[string]bool{
	"location create::subaccount": true,
	"tendlc numbers::campaign-id": true,
	"tendlc numbers::status":      true,
}

// knownDriftCommands lists command paths that are known drift and
// intentionally not yet reconciled — either a documented command path that
// no longer resolves, or (see the last entry) a prose false positive the
// parser can't tell apart from a real one.
// REMOVE entries here as the underlying drift is fixed.
//   - "tendlc campaigns", "tendlc numbers", "tendlc campaigns numbers": the
//     legacy `band tendlc campaigns`/`numbers`/`campaigns numbers` commands
//     were removed in the 10DLC PR5 cutover (task 3 of
//     .superpowers/sdd/2026-08-21-tendlc-pr5-cutover). Fixing the parser to
//     catch this exact class of drift is task 4 of that plan; the doc sweep
//     that removes these references (and these three entries) is task 6.
//   - "number list is not": not command drift at all — a false positive from
//     AGENTS.md's "# On Bandwidth Build accounts, band number list is not
//     available." comment. The line has no backtick/code-span markers, so
//     bandUsageRe (which matches "band " anywhere in a line) tokenizes the
//     following prose words ("list", "is", "not") as if they were further
//     command-path tokens, same as it would a real subcommand name. Task 6
//     rewords this line too; remove this entry alongside it.
var knownDriftCommands = map[string]bool{
	"tendlc campaigns":         true,
	"tendlc numbers":           true,
	"tendlc campaigns numbers": true,
	"number list is not":       true,
}

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

// usePlaceholderRe matches a "<...>" or "[...]" placeholder in a cobra
// Use string, e.g. "get <brand-id>" or "release [number]". This codebase
// consistently declares positional args this way (verified against every
// `Args: cobra.ExactArgs/MinimumNArgs/MaximumNArgs/RangeArgs` site in cmd/
// before relying on it here) — see resolvedCommandAbsorbsRemainder below.
var usePlaceholderRe = regexp.MustCompile(`[<\[]`)

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

// resolvedCommandAbsorbsRemainder decides whether path[matched:] — the
// command-shaped tokens (they already passed commandTokenRe) left over after
// resolveCommand stopped — are legitimate positional arguments of the
// resolved command, or evidence that the documented path doesn't actually
// exist (a stale/renamed command reference).
//
// A tightened `matched == len(path)` check (the obvious fix) is wrong: it
// would also reject real docs like `band auth use admin` and
// `band sip realm delete vapi`, where "admin"/"vapi" are positional
// arguments that happen to be lowercase words and so parse as command
// tokens too. cobra.Command.Args is a func, not introspectable data, so we
// can't read an arity off it directly.
//
// Heuristic chosen: trust the resolved command's Use string. If it declares
// a "<...>"/"[...]" placeholder, any leftover tokens are treated as that
// command's positional args (pass). Otherwise leftover tokens are treated as
// an unresolved subcommand reference (fail). This was checked against every
// Args-taking command in cmd/ and the convention holds everywhere.
//
// The alternative considered was "resolved command is a leaf (no
// subcommands)". That also passes the required cases here, but has a wider
// blind spot: it would accept ANY trailing word after a leaf command, even
// one that takes zero args (e.g. a stale `band tendlc number list foo` would
// silently pass because "list" is a leaf, regardless of what "foo" is). The
// Use-string check catches that, at the cost of its own blind spot: if a
// command takes positional args but its Use string forgets to declare a
// placeholder, this will wrongly flag it as stale.
//
// Neither heuristic — nor this whole boundary-based approach — catches
// drift where the documented path fully resolves as a command but is
// invoked with an argument shape that command no longer accepts (e.g. a
// stale `band tendlc number <phone>` after that subtree was restructured to
// require an explicit `get`/`list`/`history` subcommand): matched==len(path)
// there, so there's no remainder for this function to judge at all. That
// class of drift needs a human doc sweep, not this parser.
func resolvedCommandAbsorbsRemainder(cmd *cobra.Command, path []string, matched int) bool {
	if matched == len(path) {
		return true // fully resolved; no remainder to judge
	}
	return usePlaceholderRe.MatchString(cmd.Use)
}

// commandPathTokens splits s into whitespace-separated fields and returns the
// leading run of fields that look like command tokens (per commandTokenRe).
// The first field that isn't command-shaped (a flag, placeholder, ID, phone
// number, etc.) ends the path.
func commandPathTokens(s string) []string {
	var path []string
	for _, f := range strings.Fields(s) {
		if !commandTokenRe.MatchString(f) {
			break
		}
		path = append(path, f)
	}
	return path
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

// TestParserDistinguishesSubcommandsFromPositionals exercises the boundary
// logic (resolveCommand + resolvedCommandAbsorbsRemainder) directly against
// the real command tree, independent of any doc file. It proves the fix
// actually catches the class of drift this PR creates (a stale reference to
// a deleted multi-word command) without also flagging legitimate positional
// arguments that happen to be lowercase words.
func TestParserDistinguishesSubcommandsFromPositionals(t *testing.T) {
	tests := []struct {
		name        string
		commandLine string // everything after "band ", as it appears in docs
		wantFlagged bool
	}{
		{
			name:        "deleted `tendlc campaigns` with a trailing subcommand-shaped word",
			commandLine: "tendlc campaigns list",
			wantFlagged: true,
		},
		{
			name:        "deleted `tendlc numbers`",
			commandLine: "tendlc numbers",
			wantFlagged: true,
		},
		{
			name:        "wholly bogus top-level command",
			commandLine: "notacommand",
			wantFlagged: true,
		},
		{
			name:        "auth use admin: admin is a positional profile name, not a command",
			commandLine: "auth use admin",
			wantFlagged: false,
		},
		{
			name:        "sip realm delete vapi: vapi is a positional realm name, not a command",
			commandLine: "sip realm delete vapi",
			wantFlagged: false,
		},
		{
			name:        "tendlc brand get BEXMPL1: valid, real subcommand path with a positional",
			commandLine: "tendlc brand get BEXMPL1",
			wantFlagged: false,
		},
		{
			name:        "tendlc number list: fully valid, real subcommand path, no remainder",
			commandLine: "tendlc number list",
			wantFlagged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := commandPathTokens(tt.commandLine)
			if len(path) == 0 {
				t.Fatalf("commandPathTokens(%q) produced no path tokens", tt.commandLine)
			}
			cmd, matched := resolveCommand(path)
			// Mirrors exactly what TestDocumentedCommandsAndFlagsExist does
			// with the result of resolveCommand.
			flagged := matched == 0 || !resolvedCommandAbsorbsRemainder(cmd, path, matched)
			if flagged != tt.wantFlagged {
				t.Errorf("band %s: flagged = %v, want %v (matched=%d, path=%v, resolved=%q)",
					tt.commandLine, flagged, tt.wantFlagged, matched, path, cmd.CommandPath())
			}
		})
	}
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
				path := commandPathTokens(rest)
				if len(path) == 0 {
					continue
				}
				cmdName := strings.Join(path, " ")
				if knownDriftCommands[cmdName] {
					continue
				}
				cmd, matched := resolveCommand(path)
				if matched == 0 {
					t.Errorf("%s documents `band %s …` but %q is not a command under `band`", doc, cmdName, path[0])
					continue
				}
				if !resolvedCommandAbsorbsRemainder(cmd, path, matched) {
					t.Errorf("%s documents `band %s …` but only `band %s` resolves; %q has no declared positional args to absorb %q",
						doc, cmdName, strings.Join(path[:matched], " "), cmd.CommandPath(), strings.Join(path[matched:], " "))
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
			path := commandPathTokens(capture)
			if len(path) == 0 {
				continue
			}
			cmdName := strings.Join(path, " ")
			if knownDriftCommands[cmdName] {
				continue
			}
			cmd, matched := resolveCommand(path)
			if matched == 0 {
				t.Errorf("%s documents `band %s …` but %q is not a command under `band`", doc, cmdName, path[0])
				continue
			}
			if !resolvedCommandAbsorbsRemainder(cmd, path, matched) {
				t.Errorf("%s documents `band %s …` but only `band %s` resolves; %q has no declared positional args to absorb %q",
					doc, cmdName, strings.Join(path[:matched], " "), cmd.CommandPath(), strings.Join(path[matched:], " "))
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

package cmd

import (
	"fmt"
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

// knownDriftCommands lists command paths that are known drift and
// intentionally not yet reconciled. It is intentionally EMPTY: the 10DLC
// PR5 cutover's doc sweep reconciled every entry this map used to carry, and
// it must stay empty — do not re-add a command path here as a shortcut past
// a doc-test failure; fix the doc (or the command) instead.
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

// shellFields splits s into whitespace-separated fields, but treats content
// inside a matching quote pair ("...", '...') or placeholder pair (<...>,
// [...]) as belonging to a SINGLE field, the way a shell (for quotes) or a
// human reading a placeholder (for brackets) would. This matters because
// Args validators in this codebase only count arguments (see argsGateRejects),
// so `band bxml speak "Thanks for calling. How can we help?"` must count as
// one argument, not seven.
//
// ok is false if a quote or bracket is left unclosed by end of line. That
// happens legitimately and often: a command mentioned inside a single- or
// double-quoted phrase whose OPENING delimiter is prose that occurs before
// "band" — e.g. `Confirm with 'band tendlc brand get WEXAMPLE02' — a 404
// means it is gone.` — leaves an odd, unmatched quote count from this
// string's point of view (the capture starts at "band ", after the real
// opening quote). Callers must treat ok==false as "can't parse this line
// with confidence" and skip it, not as evidence of drift.
func shellFields(s string) (fields []string, ok bool) {
	var b strings.Builder
	inField := false
	var closing byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case closing != 0:
			b.WriteByte(c)
			if c == closing {
				closing = 0
			}
		case c == '"' || c == '\'':
			closing = c
			b.WriteByte(c)
			inField = true
		case c == '<':
			closing = '>'
			b.WriteByte(c)
			inField = true
		case c == '[':
			closing = ']'
			b.WriteByte(c)
			inField = true
		case c == ' ' || c == '\t':
			if inField {
				fields = append(fields, b.String())
				b.Reset()
				inField = false
			}
		default:
			b.WriteByte(c)
			inField = true
		}
	}
	if closing != 0 {
		return nil, false
	}
	if inField {
		fields = append(fields, b.String())
	}
	return fields, true
}

// splitPositionalArgs walks fields (everything after a resolved command) and
// separates flags — and, critically, their VALUES — from genuine positional
// arguments, consulting the resolved command's real flag definitions (the
// same source cobra itself uses) rather than guessing from a "--" prefix
// alone. That distinction matters because flags and positional args
// interleave in real examples: `band bxml speak --voice julie "Press 1 for
// sales."` has exactly one positional argument (the quoted string), but a
// naive scan that stops at the first "--x" token would see zero.
//
// A flag's pflag.Flag.NoOptDefVal is empty only when the flag REQUIRES an
// explicit value (e.g. a string flag): in that case the next field is
// consumed as its value and excluded from the result. Bool flags set
// NoOptDefVal ("true") so `--wait` alone doesn't eat the next token.
//
// ok is false when a flag can't be resolved against the command (unknown to
// Flags/InheritedFlags/the root persistent flags) — in that case we don't
// know whether it consumes the next token, so the caller should not trust
// the result rather than guess.
func splitPositionalArgs(cmd *cobra.Command, fields []string) (args []string, ok bool) {
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "#") {
			break // trailing shell comment
		}
		if !strings.HasPrefix(f, "-") || f == "-" {
			args = append(args, f)
			continue
		}
		if strings.Contains(f, "=") {
			continue // "--flag=value" is one self-contained token
		}
		name := strings.TrimLeft(f, "-")
		fl := cmd.Flags().Lookup(name)
		if fl == nil {
			fl = cmd.InheritedFlags().Lookup(name)
		}
		if fl == nil {
			fl = rootCmd.PersistentFlags().Lookup(name)
		}
		if fl == nil {
			return nil, false // unresolvable flag; don't guess
		}
		if fl.NoOptDefVal == "" && i+1 < len(fields) {
			i++ // this flag requires an explicit value; skip it too
		}
	}
	return args, true
}

// argsGateRejects is the second, authoritative gate on top of
// resolvedCommandAbsorbsRemainder. That heuristic only asks "does this
// remainder *look* positional" (via the resolved command's Use string) —
// which is blind to the case where `path` resolves fully (matched ==
// len(path), so there's no `path` remainder for the heuristic to examine at
// all) but the resolved command's actual runtime Args validator no longer
// accepts what follows it in the doc line. That's exactly the shape of the
// drift this PR creates: `band tendlc number <phone>` resolves completely to
// the `number` dispatcher (matched == len(path) == 2), yet `number`'s Args
// is cobra.NoArgs as of the 10DLC PR5 cutover (task 3 of
// .superpowers/sdd/2026-08-21-tendlc-pr5-cutover), which restructured it to
// require an explicit get/list/history subcommand instead of a bare
// phone-number argument.
//
// It re-resolves the command from s's raw fields rather than reusing the
// caller's `path`/`matched`: `path` was built from commandTokenRe, a regex
// that (being ignorant of the real command tree) can incorrectly exclude a
// real subcommand name that doesn't fit its lowercase-letters-and-hyphens
// shape — e.g. "resend-2fa" (has a digit) — which would otherwise make this
// gate blame the PARENT command for rejecting what is actually a perfectly
// valid subcommand name.
//
// This deliberately ABSTAINS (returns nil — "no drift found") rather than
// flag, whenever it can't parse the line with confidence:
//   - shellFields reports an unterminated quote/bracket (see its doc comment);
//   - splitPositionalArgs can't resolve a flag;
//   - there's no remainder at all — a bare mention like "`band portin get`",
//     naming a command without demonstrating a full invocation, is normal
//     technical writing and must not be required to show every argument;
//   - a positional token is a literal "..." or contains a "," — both strong
//     signals of deliberately elided/abbreviated example text (e.g. `band
//     tendlc campaign create <tier 1+2 fields, both booleans true> --plain`)
//     rather than a literal, runnable argument list;
//   - a positional token is a lone "\" — a shell line-continuation marker
//     from a multi-line example, meaning the real argument is on the next
//     line and this test only ever looks at one line at a time.
//
// Abstaining trades a known blind spot (a genuinely bogus example of one of
// these shapes would also slip through unflagged) for not flagging real,
// legitimate documentation — the right side to err on: a gate that cries
// wolf on correct docs gets ignored or its escape hatch gets widened, which
// is strictly worse than a gate with a named, narrow blind spot.
//
// Checked every `Args:` site in cmd/ as of this task (98 positional-taking
// commands): all are cobra.ExactArgs/MinimumNArgs/MaximumNArgs/RangeArgs/
// NoArgs, which only count arguments, never inspect their values — so this
// gate never needs to worry about a value-inspecting validator (e.g.
// cobra.OnlyValidArgs) rejecting a correct placeholder like "<brand-id>".
// Re-check this claim if such a validator is ever added.
func argsGateRejects(s string) error {
	fields, ok := shellFields(s)
	if !ok {
		return nil
	}
	cmd, matched := resolveCommand(fields)
	if matched == 0 || matched >= len(fields) {
		return nil
	}
	pos, ok := splitPositionalArgs(cmd, fields[matched:])
	if !ok || len(pos) == 0 {
		return nil
	}
	for _, a := range pos {
		if a == "..." || a == `\` || strings.Contains(a, ",") {
			return nil
		}
		// A "<...>"/"[...]" placeholder that itself spans multiple words
		// (e.g. "<all common fields + company/vertical/ein>", "<tier 1+2
		// fields, both booleans true>") isn't a single positional value —
		// every genuine one-argument placeholder in this codebase's Use
		// strings is one hyphenated word ("<brand-id>", "<phone-number>").
		// A multi-word one is prose shorthand for "insert several flags
		// here", not a literal argument; quoted multi-word strings (merged
		// above by shellFields) are unaffected since they don't start with
		// "<"/"[".
		if (strings.HasPrefix(a, "<") || strings.HasPrefix(a, "[")) && strings.ContainsAny(a, " \t") {
			return nil
		}
	}
	if cmd.Args == nil {
		return nil
	}
	if err := cmd.Args(cmd, pos); err != nil {
		return fmt.Errorf("resolved command %q rejects the documented arguments %v: %w", cmd.CommandPath(), pos, err)
	}
	return nil
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

// TestParserDistinguishesSubcommandsFromPositionals exercises the full
// three-gate boundary logic (resolveCommand + resolvedCommandAbsorbsRemainder
// + argsGateRejects) directly against the real command tree, independent of
// any doc file. It proves the fix actually catches the class of drift this
// PR creates — both the stale-multi-word-command shape (caught by the first
// two gates) and the fully-resolves-but-rejects-the-args shape (caught only
// by argsGateRejects) — without also flagging legitimate positional
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
			// The legacy bare `number <phone>` (a different command from the
			// current `number get <phone>`) — resolves fully to the `number`
			// dispatcher (matched == len(path)), so the Use-heuristic gate
			// alone can't see anything wrong; only argsGateRejects, which
			// calls numberCmd's real Args (cobra.NoArgs) against the
			// documented phone number, catches it. Reserved-range number
			// (555-01xx block), not a real one.
			name:        "deleted bare `tendlc number <phone>`: resolves fully but numberCmd's Args rejects it",
			commandLine: "tendlc number +15555550100",
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
			// with the result of resolveCommand: gate 1 (existence), gate 2
			// (Use-heuristic), then gate 3 (the real Args validator) only if
			// gates 1 and 2 both passed.
			flagged := matched == 0 || !resolvedCommandAbsorbsRemainder(cmd, path, matched)
			var argsErr error
			if !flagged {
				argsErr = argsGateRejects(tt.commandLine)
				flagged = argsErr != nil
			}
			if flagged != tt.wantFlagged {
				t.Errorf("band %s: flagged = %v, want %v (matched=%d, path=%v, resolved=%q, argsGateRejects=%v)",
					tt.commandLine, flagged, tt.wantFlagged, matched, path, cmd.CommandPath(), argsErr)
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
			// Shell comments inside fenced blocks are prose, not runnable
			// commands. They routinely mention a command mid-sentence — e.g.
			// "# On Build accounts, band number list is not available." — and
			// parsing them treats the following English words as a command
			// path. Skipping them removes a whole class of false positive at
			// the cost of not validating commands that appear only inside a
			// comment, which are by definition not invocations.
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
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
					continue
				}
				if err := argsGateRejects(rest); err != nil {
					t.Errorf("%s documents `band %s …` but %v", doc, cmdName, err)
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
			if err := argsGateRejects(capture); err != nil {
				t.Errorf("%s documents `band %s …` but %v", doc, cmdName, err)
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

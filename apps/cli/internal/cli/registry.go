package cli

import (
	"context"
	"sort"
	"strings"

	"ngfw/cli/internal/cpath"
	"ngfw/cli/internal/lineedit"
)

// Where a command is available.
const (
	inOp     = 1 << iota // operational mode (and one-shot `vrx <command>`)
	inConfig             // configuration mode
	inBoth   = inOp | inConfig
)

// Command is one CLI command. Ops lists the operationIds (OpenAPI) the command calls — the reference page and the
// acceptance check ("every CLI command maps to a documented REST call") are generated from it.
type Command struct {
	Words    []string
	Args     string
	Summary  string
	Where    int
	Ops      []string
	NoREST   string // why a command has no REST call (local command, or the API lacks the endpoint)
	Public   bool   // runs without credentials (login, help)
	Run      func(a *App, ctx context.Context, args []cpath.Token) error
	Complete func(a *App, ctx context.Context, args []string, partial string) []lineedit.Candidate
	Example  string
}

// Name is the command words joined.
func (c *Command) Name() string { return strings.Join(c.Words, " ") }

var registry []*Command

func register(c *Command) { registry = append(registry, c) }

// Commands returns every command, sorted by name then mode.
func Commands() []*Command {
	out := append([]*Command{}, registry...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name() != out[j].Name() {
			return out[i].Name() < out[j].Name()
		}
		return out[i].Where < out[j].Where
	})
	return out
}

func available(where int, mode Mode) bool {
	if mode == ModeConfig {
		return where&inConfig != 0
	}
	return where&inOp != 0
}

// match finds the command for the words of a line in mode: every command word may be abbreviated to a unique
// prefix among the commands still possible ("sh int" = "show interfaces"). It returns the command, the number of
// tokens consumed, and the candidates of the first word that did not match (for errors and completion).
func match(mode Mode, toks []cpath.Token) (*Command, int, []string) {
	cands := []*Command{}
	for _, c := range registry {
		if available(c.Where, mode) {
			cands = append(cands, c)
		}
	}
	var best *Command
	bestN := 0
	for depth := 0; ; depth++ {
		// commands complete at this depth win when no longer command matches
		for _, c := range cands {
			if len(c.Words) == depth && depth > bestN {
				best, bestN = c, depth
			}
		}
		if depth >= len(toks) || toks[depth].Quoted {
			break
		}
		w := toks[depth].Text
		next := map[string][]*Command{}
		for _, c := range cands {
			if len(c.Words) > depth {
				next[c.Words[depth]] = append(next[c.Words[depth]], c)
			}
		}
		var hit []string
		if _, ok := next[w]; ok {
			hit = []string{w}
		} else {
			for word := range next {
				if strings.HasPrefix(word, w) {
					hit = append(hit, word)
				}
			}
		}
		if len(hit) != 1 {
			if best == nil {
				words := make([]string, 0, len(next))
				for word := range next {
					words = append(words, word)
				}
				sort.Strings(words)
				return nil, depth, words
			}
			break
		}
		cands = next[hit[0]]
	}
	return best, bestN, nil
}

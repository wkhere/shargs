// Package shargs splits a shell command line into words. It is based on
// a port of Python's shlex.split(s, comments=True) (posix mode,
// whitespace_split), with two quirks fixed to follow the shell:
//
//   - '#' starts a comment only at the start of a word; in shlex it does
//     anywhere outside quotes, so a#b was cut to a.
//   - The operator characters ; & | ( ) < > split words even without
//     spaces around them, a run of them (&&, ||, >>) being one operator
//     word; in shlex, cmd;ansible-playbook was a single word.
//
// Expansions ($x, ${x}, $(cmd), `cmd`) are kept as written, and $(...)
// and `...` stay whole within their word, operators and spaces included,
// as in the shell (so a Makefile's $(var) reference stays whole too).
package shargs

import "errors"

// Word is a word of a command line.
type Word struct {
	// Text is the word with quotes and escapes removed;
	// expansions are kept as written.
	Text string
	// Op marks an operator word: a run of ; & | ( ) < > like && or >>.
	Op bool
}

// Split splits s into words. Errors are ErrUnclosedQuote,
// ErrTrailingEscape and ErrUnclosedSubst.
func Split(s string) ([]Word, error) {
	isWS := func(c rune) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
	isQuote := func(c rune) bool { return c == '\'' || c == '"' }

	rs := []rune(s)
	pos := 0
	next := func() (rune, bool) {
		if pos >= len(rs) {
			return 0, false
		}
		pos++
		return rs[pos-1], true
	}
	skipLine := func() {
		for pos < len(rs) {
			pos++
			if rs[pos-1] == '\n' {
				return
			}
		}
	}

	// state is ' ' (between words), 'a' (in a word), 'o' (in an operator),
	// a quote, '\\' (after an escape), or 0 past the end of input.
	state := ' '
	var words []Word
	for {
		var token []rune
		quoted := false
		escapedState := 'a'
		subst, backtick := 0, false // open $( ... ) and ` ... ` in the word
		substQuote, substEsc := rune(0), false
		// token length after a $ that can open $( ... ), and the state it was in
		dollar, dollarIn := -1, rune(0)
	read:
		for {
			c, ok := next()
			switch {
			case state == 0:
				token = nil
				break read
			case state == ' ':
				switch {
				case !ok:
					state = 0
					break read
				case isWS(c):
					if len(token) > 0 || quoted {
						break read
					}
				case c == '#':
					skipLine()
				case c == '\\':
					escapedState, state = 'a', '\\'
				case isQuote(c):
					state = c
				case isOp(c):
					token, state = []rune{c}, 'o'
				default:
					token, state = []rune{c}, 'a'
					backtick = c == '`'
					if c == '$' {
						dollar, dollarIn = 1, 'a'
					}
				}
			case state == 'o':
				if ok && isOp(c) {
					token = append(token, c)
					continue
				}
				if ok {
					pos-- // c starts the next word
				}
				state = ' '
				words = append(words, Word{string(token), true})
				token = nil
				if !ok {
					state = 0
				}
				continue
			case (state == 'a' || state == '"') && (subst > 0 || backtick):
				// kept as written; quotes and escapes only hide ( ) and `
				if !ok {
					return nil, ErrUnclosedSubst
				}
				token = append(token, c)
				switch {
				case substEsc:
					substEsc = false
				case c == '\\' && substQuote != '\'':
					substEsc = true
				case backtick:
					backtick = c != '`'
				case substQuote != 0:
					if c == substQuote {
						substQuote = 0
					}
				case isQuote(c):
					substQuote = c
				case c == '(':
					subst++
				case c == ')':
					subst--
				}
			case isQuote(state):
				quoted = true
				switch {
				case !ok:
					return nil, ErrUnclosedQuote
				case c == state:
					state = 'a'
				case c == '\\' && state == '"':
					escapedState, state = state, '\\'
				case state == '"' && c == '(' && dollar == len(token) && dollarIn == '"':
					token = append(token, c)
					subst = 1
				case state == '"' && c == '`':
					token = append(token, c)
					backtick = true
				default:
					token = append(token, c)
					if state == '"' && c == '$' {
						dollar, dollarIn = len(token), '"'
					}
				}
			case state == '\\':
				if !ok {
					return nil, ErrTrailingEscape
				}
				// in quotes, only the quote itself or '\' may be escaped
				if isQuote(escapedState) && c != state && c != escapedState {
					token = append(token, state)
				}
				token = append(token, c)
				state = escapedState
			case state == 'a':
				switch {
				case !ok:
					state = 0
					break read
				case isWS(c):
					state = ' '
					if len(token) > 0 || quoted {
						break read
					}
				case c == '(' && dollar == len(token) && dollarIn == 'a':
					token = append(token, c)
					subst = 1
				case c == '`':
					token = append(token, c)
					backtick = true
				case isOp(c):
					pos-- // the operator is a word of its own
					state = ' '
					if len(token) > 0 || quoted {
						break read
					}
				case isQuote(c):
					state = c
				case c == '\\':
					escapedState, state = 'a', '\\'
				default:
					token = append(token, c) // '#' too: not a comment mid-word
					if c == '$' {
						dollar, dollarIn = len(token), 'a'
					}
				}
			}
		}
		if !quoted && len(token) == 0 {
			if state == 0 {
				return words, nil
			}
			continue
		}
		words = append(words, Word{Text: string(token)})
	}
}

// Fields returns the words of s as the arguments of a single command.
// An operator word in s makes it fail with ErrUnexpectedOp, unless StopAtOp is given;
// > and < count too, so redirections fail as well.
// Other errors are those of Split.
func Fields(s string, opts ...Option) ([]string, error) {
	var c config
	for _, o := range opts {
		o(&c)
	}
	words, err := Split(s)
	if err != nil {
		return nil, err
	}
	var fs []string
	for i, w := range words {
		if w.Op {
			switch {
			case !c.stopAtOp:
				return nil, ErrUnexpectedOp
			case i == 0:
				return nil, ErrOpAtStart
			}
			break
		}
		fs = append(fs, w.Text)
	}
	return fs, nil
}

// Option configures Fields.
type Option func(*config)

type config struct {
	stopAtOp bool
}

// StopAtOp makes Fields return the words before the first operator
// instead of failing with ErrUnexpectedOp. An input starting with an operator
// fails with ErrOpAtStart.
func StopAtOp() Option {
	return func(c *config) { c.stopAtOp = true }
}

var (
	ErrUnclosedQuote  = errors.New("shargs: no closing quotation")
	ErrTrailingEscape = errors.New("shargs: no escaped character")
	ErrUnclosedSubst  = errors.New("shargs: no closing ) or ` of a substitution")

	ErrUnexpectedOp = errors.New("shargs: unexpected operator")
	ErrOpAtStart    = errors.New("shargs: operator at start")
)

func isOp(c rune) bool {
	switch c {
	case ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

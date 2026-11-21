# shargs

[![Go Reference](https://pkg.go.dev/badge/github.com/wkhere/shargs.svg)](https://pkg.go.dev/github.com/wkhere/shargs)

Package shargs splits a shell command line into words. It does what
Python's `shlex.split` does, plus it recognizes shell operators. It has no
dependencies.

It suits programs that read command lines written by people for a shell
(Makefile recipes, CI scripts, config files) and need their words and the
commands' boundaries, without running or expanding anything.

```
go get github.com/wkhere/shargs
```

## Usage

`Split` returns all the words, with operators marked:

```go
words, err := shargs.Split(`cd build && make -j4 > log`)
// cd  build  [&&]  make  -j4  [>]  log
// (operator words, Op set, in brackets)
```

`Fields` returns the arguments of a single command, ready for
`exec.Command`:

```go
args, err := shargs.Fields(`ls -l 'my dir'`)
// ["ls" "-l" "my dir"]

_, err = shargs.Fields(`ls -l | wc -l`)
// err == shargs.ErrUnexpectedOp

args, err = shargs.Fields(`ls -l | wc -l`, shargs.StopAtOp())
// ["ls" "-l"]
```

`Fields` rejects input with any operator in it, redirections included, so
the part after the operator can't be silently dropped or passed on as
arguments. With `StopAtOp` it returns the words before the first operator
instead, and fails with `ErrOpAtStart` if there are none, as in `(cd x)`.

## Splitting rules

Quoting and escapes follow `shlex.split(s, comments=True)` in posix mode:

| input | words |
|---|---|
| `a  b	c` | `a` `b` `c` |
| `'x y' "p 'q' r" foo\ bar` | `x y` `p 'q' r` `foo bar` |
| `'it'\''s' "a \"b\""` | `it's` `a "b"` |
| `a b # comment` | `a` `b` |

with two changes that follow the shell instead:

- `#` starts a comment only at the start of a word: `a#b` stays `a#b`.
- The operator characters `; & | ( ) < >` end a word even without spaces
  around them. A run of them is one operator word, so `a&&b;c` is `a`
  `[&&]` `b` `[;]` `c`.

Expansions are kept as written. `$(...)` and `` `...` `` stay whole
within their word, spaces and operators included, as they would in the
shell:

| input | words |
|---|---|
| `--tag=$(git describe) ${HOME}/bin` | `--tag=$(git describe)` `${HOME}/bin` |
| `$(echo x; y) z` | `$(echo x; y)` `z` |
| `$(echo ")" \)) z` | `$(echo ")" \))` `z` |

Inside them, quotes and escapes are kept too, and a `)` or `` ` `` they
hide doesn't end the substitution.

That also keeps a Makefile's `$(var)` references whole.

## Errors

| error | cause |
|---|---|
| `ErrUnclosedQuote` | a quote not closed by the end of input |
| `ErrTrailingEscape` | a backslash at the end of input |
| `ErrUnclosedSubst` | a `$(` or `` ` `` not closed by the end of input |
| `ErrUnexpectedOp` | `Fields`: an operator in the input |
| `ErrOpAtStart` | `Fields` with `StopAtOp`: the input starts with an operator |

## Caveats

shargs splits words. It isn't a shell parser, so:

- Nothing is expanded: `$x`, `*.go` and `{a,b}` come out as written. A
  word's `Text` doesn't tell whether a `$` in it was quoted: `"$HOME"`
  and `'$HOME'` both give `$HOME`.
- `$'...'` quoting isn't recognized: `$'a\tb'` gives `$a\tb`.
- Operators aren't checked: `|;` is one operator word. A redirection's
  file descriptor stays a separate word: `2>&1` gives `2` `[>&]` `1`.
- A backslash before a newline keeps the newline instead of joining the
  lines.
- No here-documents, and no keywords like `if` or `for`: they are plain
  words.

For full shell syntax, use [mvdan.cc/sh](https://pkg.go.dev/mvdan.cc/sh/v3/syntax).

## License

MIT, see [LICENSE](LICENSE).

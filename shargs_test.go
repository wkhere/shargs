package shargs

import (
	"errors"
	"slices"
	"testing"
)

func TestSplit(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string // operator words in [brackets]
	}{
		// as Python's shlex.split(s, comments=True)
		{"", nil},
		{"a  b\tc", []string{"a", "b", "c"}},
		{`a 'x y' "p 'q' r" foo\ bar`, []string{"a", "x y", "p 'q' r", "foo bar"}},
		{`x="q\"q" y="a\nb" z=a\"b w=""`, []string{`x=q"q`, `y=a\nb`, `z=a"b`, "w="}},
		{`q='it'\''s'`, []string{"q=it's"}},
		{"a b # c d", []string{"a", "b"}},
		{"$(dtag) ${HOST} `id`", []string{"$(dtag)", "${HOST}", "`id`"}},
		// fixed: '#' mid-word, operators without spaces
		{"a#b c", []string{"a#b", "c"}},
		{"cmd;ansible-playbook x", []string{"cmd", "[;]", "ansible-playbook", "x"}},
		{"a&&b||c", []string{"a", "[&&]", "b", "[||]", "c"}},
		{"(cd x)>log", []string{"[(]", "cd", "x", "[)>]", "log"}},
		{`a ";" \; 'b|c'`, []string{"a", ";", ";", "b|c"}},
		{"a && b", []string{"a", "[&&]", "b"}},
		{"a ;# c", []string{"a", "[;]"}},
		{"-t $(dtag) -i$(inv)x", []string{"-t", "$(dtag)", "-i$(inv)x"}},
		{"a $(echo x; f (y)) `b; c` d", []string{"a", "$(echo x; f (y))", "`b; c`", "d"}},
		{`x $(echo ")" ')') y`, []string{"x", `$(echo ")" ')')`, "y"}},
		{`x $(echo \) "\")") y`, []string{"x", `$(echo \) "\")")`, "y"}},
		{`x $(echo '\') y`, []string{"x", `$(echo '\')`, "y"}},
		{"x `echo \\` y` z", []string{"x", "`echo \\` y`", "z"}},
		{"$((1+2)) x", []string{"$((1+2))", "x"}},
		{"$$(git describe) x", []string{"$$(git describe)", "x"}},
		// substitutions in double quotes
		{`"$(echo ")")" x`, []string{`$(echo ")")`, "x"}},
		{`"a $(b c)d" e`, []string{"a $(b c)d", "e"}},
		{"\"a `echo \"x\"` b\" c", []string{"a `echo \"x\"` b", "c"}},
		// a $ that is quoted, escaped or in other quotes doesn't open one
		{`"\$(x" y`, []string{`\$(x`, "y"}}, // shlex keeps \ before $ in "..."
		{`'$'"(x" y`, []string{"$(x", "y"}},
		{`$"(x" y`, []string{"$(x", "y"}},
		{`'$(x' y`, []string{"$(x", "y"}},
		{`a \$(b`, []string{"a", "$", "[(]", "b"}},
	} {
		ws, err := Split(c.in)
		var got []string
		for _, w := range ws {
			if w.Op {
				w.Text = "[" + w.Text + "]"
			}
			got = append(got, w.Text)
		}
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("Split(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for in, want := range map[string]error{
		"a 'b": ErrUnclosedQuote, `a "b`: ErrUnclosedQuote, `a b\`: ErrTrailingEscape,
		"a $(b c": ErrUnclosedSubst, "a `b c": ErrUnclosedSubst, `a $(b ")"`: ErrUnclosedSubst,
		`a $(b \)`: ErrUnclosedSubst, `"a $(b" c`: ErrUnclosedSubst, "\"a `b\" c": ErrUnclosedSubst,
	} {
		if _, err := Split(in); !errors.Is(err, want) {
			t.Errorf("Split(%q) error = %v; want %v", in, err, want)
		}
	}
}

func TestFields(t *testing.T) {
	for _, c := range []struct {
		in   string
		stop bool
		want []string
		err  error
	}{
		{"", false, nil, nil},
		{`foo 'a b' c # x`, false, []string{"foo", "a b", "c"}, nil},
		{`foo a ";" \&`, false, []string{"foo", "a", ";", "&"}, nil},
		{"foo a && b", false, nil, ErrUnexpectedOp},
		{"foo a > log", false, nil, ErrUnexpectedOp},
		{"foo a && b", true, []string{"foo", "a"}, nil},
		{"foo a>log", true, []string{"foo", "a"}, nil},
		{"(cd x)", true, nil, ErrOpAtStart},
		{"(cd x)", false, nil, ErrUnexpectedOp},
		{"foo 'a", true, nil, ErrUnclosedQuote},
	} {
		var opts []Option
		if c.stop {
			opts = append(opts, StopAtOp())
		}
		got, err := Fields(c.in, opts...)
		if !errors.Is(err, c.err) || !slices.Equal(got, c.want) {
			t.Errorf("Fields(%q, stop=%v) = %q, %v; want %q, %v",
				c.in, c.stop, got, err, c.want, c.err)
		}
	}
}

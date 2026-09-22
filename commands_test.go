package main

import (
	"reflect"
	"testing"
)

func TestSplitCommands(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []string
		want []string
	}{
		{"one line", []string{"set scg ip 10.0.0.5"}, []string{"set scg ip 10.0.0.5"}},
		{"several lines", []string{"a\nb\nc"}, []string{"a", "b", "c"}},
		{"windows line endings", []string{"a\r\nb"}, []string{"a", "b"}},
		{"old mac line endings", []string{"a\rb"}, []string{"a", "b"}},
		// A textarea almost always ends in a newline; sending a bare return to
		// several hundred APs achieves nothing.
		{"trailing newline", []string{"a\n"}, []string{"a"}},
		{"blank lines between", []string{"a\n\n\nb"}, []string{"a", "b"}},
		{"indentation trimmed", []string{"  a  \n\tb\t"}, []string{"a", "b"}},
		{"repeated flag", []string{"a", "b"}, []string{"a", "b"}},
		{"repeated flag with newlines", []string{"a\nb", "c"}, []string{"a", "b", "c"}},
		{"nothing", []string{""}, nil},
		{"whitespace only", []string{"   \n\t\n"}, nil},
	} {
		if got := splitCommands(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: splitCommands(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

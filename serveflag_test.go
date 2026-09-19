package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseServe runs a real flag set so the test exercises the same parsing the
// binary does, not a reimplementation of it.
func parseServe(t *testing.T, argv []string) (serveFlag, []string) {
	t.Helper()
	var s serveFlag
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	fs.Var(&s, "serve", "")
	fs.String("user", "", "")
	if err := fs.Parse(argv); err != nil {
		t.Fatalf("parse %v: %v", argv, err)
	}
	return s, fs.Args()
}

func TestServeDefaultsToTheWorkingDirectory(t *testing.T) {
	s, args := parseServe(t, []string{"-serve", "-user", "admin"})
	dir, rest, err := resolveServeDir(s, args, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/work" {
		t.Errorf("dir = %q, want the working directory", dir)
	}
	if len(rest) != 0 {
		t.Errorf("leftover args %v", rest)
	}
}

func TestServeWithAttachedDirectory(t *testing.T) {
	s, args := parseServe(t, []string{"-serve=/firmware", "-user", "admin"})
	dir, _, err := resolveServeDir(s, args, "/work")
	if err != nil || dir != "/firmware" {
		t.Errorf("dir = %q, err = %v", dir, err)
	}
}

// The space form has to keep working: it is what the earlier builds documented.
func TestServeWithSeparateDirectoryArgument(t *testing.T) {
	real := t.TempDir()
	s, args := parseServe(t, []string{"-user", "admin", "-serve", real})
	dir, rest, err := resolveServeDir(s, args, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if dir != real {
		t.Errorf("dir = %q, want %q", dir, real)
	}
	if len(rest) != 0 {
		t.Errorf("the path should have been consumed, leftovers: %v", rest)
	}
}

// A mistyped path must say so rather than quietly sharing the wrong folder.
func TestServeRejectsABadSeparateArgument(t *testing.T) {
	s, args := parseServe(t, []string{"-serve", "/no/such/place"})
	if _, _, err := resolveServeDir(s, args, "/work"); err == nil {
		t.Error("a nonexistent directory was accepted")
	}

	f := filepath.Join(t.TempDir(), "image.bl7")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, args = parseServe(t, []string{"-serve", f})
	if _, _, err := resolveServeDir(s, args, "/work"); err == nil {
		t.Error("a file was accepted as a directory")
	}
}

func TestServeOffByDefault(t *testing.T) {
	s, args := parseServe(t, []string{"-user", "admin"})
	dir, _, err := resolveServeDir(s, args, "/work")
	if err != nil || dir != "" {
		t.Errorf("dir = %q, err = %v; -serve was not given", dir, err)
	}

	// An explicit -serve=false must stay off too.
	s, args = parseServe(t, []string{"-serve=false"})
	if dir, _, _ := resolveServeDir(s, args, "/work"); dir != "" {
		t.Errorf("-serve=false enabled serving from %q", dir)
	}
}

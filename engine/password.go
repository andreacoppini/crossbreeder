package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// resolvePassword decides where the AP password comes from.
//
// Passing a password as a command-line argument is the fragile option: cmd.exe
// eats "^" as its escape character, PowerShell and POSIX shells each claim a
// different set, and the argument is visible in the process list either way. So
// an explicit source wins, and an interactive terminal is offered a prompt
// before we fall back to whatever the shell handed us.
func resolvePassword(o options) (string, error) {
	if o.passEnv != "" {
		v, ok := os.LookupEnv(o.passEnv)
		if !ok {
			return "", fmt.Errorf("-pass-env %s: environment variable is not set", o.passEnv)
		}
		return v, nil
	}
	if o.askPass {
		return promptPassword(fmt.Sprintf("Password for %s: ", o.user))
	}
	// No password given but a username was: prompt rather than silently trying
	// an empty one, when there is somebody there to type it.
	if o.pass == "" && o.user != "" && term.IsTerminal(int(os.Stdin.Fd())) {
		return promptPassword(fmt.Sprintf("Password for %s: ", o.user))
	}
	return o.pass, nil
}

func promptPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	// Piped input: read a line, so the tool stays scriptable.
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

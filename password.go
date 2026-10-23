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

// resolveNewPassword decides where the replacement password comes from, when
// an AP demands a change at first login. Same reasoning as resolvePassword,
// with one difference: it is never prompted for unasked. Setting a password on
// every factory AP in a list is a deliberate act, so it happens only when one
// of the three sources was named explicitly.
func resolveNewPassword(o options) (string, error) {
	if o.newPassEnv != "" {
		v, ok := os.LookupEnv(o.newPassEnv)
		if !ok {
			return "", fmt.Errorf("-new-pass-env %s: environment variable is not set", o.newPassEnv)
		}
		return v, nil
	}
	if o.askNewPass {
		return promptPassword("New password to set on APs that demand a change: ")
	}
	return o.newPass, nil
}

// resolveR1Secret decides where the RUCKUS One shared secret comes from, with
// the same reasoning as resolvePassword: an explicit source wins, a terminal is
// offered a prompt, and the command-line argument is the last resort because it
// lands in the shell history and the process list.
func resolveR1Secret(o options) (string, error) {
	if !o.r1 {
		return "", nil
	}
	if o.r1SecretEnv != "" {
		v, ok := os.LookupEnv(o.r1SecretEnv)
		if !ok {
			return "", fmt.Errorf("-r1-secret-env %s: environment variable is not set", o.r1SecretEnv)
		}
		return v, nil
	}
	if o.askR1Secret {
		return promptPassword("RUCKUS One shared secret: ")
	}
	if o.r1Secret == "" && term.IsTerminal(int(os.Stdin.Fd())) {
		return promptPassword("RUCKUS One shared secret: ")
	}
	return o.r1Secret, nil
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

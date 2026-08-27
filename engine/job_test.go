package main

import (
	"strings"
	"testing"

	"github.com/andreacoppini/crossbreeder/engine/ap"
)

// The summary is the diagnostic that distinguishes "the AP rejected good
// credentials" from "we quietly tried something else".
func TestCredentialSummaryNamesAccountsAndPasswordLength(t *testing.T) {
	got := credentialSummary([]ap.Credentials{
		{User: "admin", Password: "J^60*k{PH%mp1G5e"},
		{User: "super", Password: "sp-admin"},
	})
	for _, want := range []string{"admin", "16-character", "super", "8-character"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing from %q", want, got)
		}
	}
	// It must never leak the password itself.
	if strings.Contains(got, "J^60") || strings.Contains(got, "sp-admin") {
		t.Errorf("summary leaks a password: %q", got)
	}
}

func TestCredentialSummaryFlagsAnEmptyPassword(t *testing.T) {
	got := credentialSummary([]ap.Credentials{{User: "admin"}})
	if !strings.Contains(got, "no password") {
		t.Errorf("an empty password is not called out: %q", got)
	}
}

// A password with no username used to be dropped in silence, and the run then
// tried the factory defaults and blamed the AP.
func TestPasswordWithoutUsernameIsRefused(t *testing.T) {
	_, _, err := buildConfig(options{}, "J^60*k{PH%mp1G5e", "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "no username") {
		t.Errorf("error = %q", err)
	}
}

func TestNoCredentialsFallsBackToFactoryDefaults(t *testing.T) {
	cfg, _, err := buildConfig(options{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Credentials) != 1 || cfg.Credentials[0].User != "super" {
		t.Errorf("credentials = %+v", cfg.Credentials)
	}
}

func TestAlsoDefaultAppendsTheFactoryPair(t *testing.T) {
	cfg, _, err := buildConfig(options{user: "admin", alsoDefault: true}, "pw", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Credentials) != 2 || cfg.Credentials[0].User != "admin" || cfg.Credentials[1].User != "super" {
		t.Errorf("credentials = %+v", cfg.Credentials)
	}
}

// The AP will not accept a short password, so a run that would have been
// rejected against every factory AP is stopped once, up front.
func TestShortNewPasswordIsRejectedBeforeTheRun(t *testing.T) {
	_, _, err := buildConfig(options{user: "admin", changePass: true}, "pw", "short")
	if err == nil {
		t.Fatal("a 5-character new password was accepted")
	}
	if !strings.Contains(err.Error(), "8 characters or longer") {
		t.Errorf("error = %q, want it to name the length rule", err)
	}

	if _, _, err := buildConfig(options{user: "admin", changePass: true}, "pw", "longenough"); err != nil {
		t.Errorf("a 10-character new password was rejected: %v", err)
	}
}

// The switch is separate from the value, as it was in the original: turning it
// off must leave the operator's password in the box rather than forcing them to
// clear it.
func TestChangePasswordSwitchIsSeparateFromTheValue(t *testing.T) {
	on, _, err := buildConfig(options{user: "admin", changePass: true}, "pw", defaultNewPassword)
	if err != nil {
		t.Fatalf("enabled: %v", err)
	}
	if on.NewPassword != defaultNewPassword {
		t.Errorf("enabled: NewPassword = %q, want %q", on.NewPassword, defaultNewPassword)
	}

	off, _, err := buildConfig(options{user: "admin", changePass: false}, "pw", defaultNewPassword)
	if err != nil {
		t.Fatalf("disabled: %v", err)
	}
	if off.NewPassword != "" {
		t.Errorf("disabled: NewPassword = %q, want it not carried into the run", off.NewPassword)
	}

	// A short password with the switch off is not an error: it is never used.
	if _, _, err := buildConfig(options{user: "admin", changePass: false}, "pw", "short"); err != nil {
		t.Errorf("disabled with a short password should not fail the run: %v", err)
	}
}

// The default is the original's, and APs already flashed by that tool carry it.
func TestDefaultNewPasswordMatchesTheOriginal(t *testing.T) {
	if defaultNewPassword != "Crossbreeder" {
		t.Errorf("defaultNewPassword = %q, want %q", defaultNewPassword, "Crossbreeder")
	}
	if len(defaultNewPassword) < minNewPasswordLen {
		t.Errorf("the default is shorter than the AP will accept")
	}
}

// "fw update" only starts the download. A reboot or factory reset restarts the
// AP before it finishes and throws the image away, so the combination is
// refused rather than half-performed across a whole site list.
func TestFirmwareCannotBeCombinedWithRebootOrFactory(t *testing.T) {
	for _, c := range []struct {
		name string
		opt  options
	}{
		{"reboot", options{user: "u", fw: true, reboot: true}},
		{"factory", options{user: "u", fw: true, factory: true}},
		{"both", options{user: "u", fw: true, reboot: true, factory: true}},
	} {
		_, _, err := buildConfig(c.opt, "pw", "")
		if err == nil {
			t.Errorf("%s: the combination was accepted", c.name)
			continue
		}
		if !strings.Contains(err.Error(), "discards the download") {
			t.Errorf("%s: error = %q, want it to explain why", c.name, err)
		}
	}

	// Each on its own is untouched.
	for _, c := range []struct {
		name string
		opt  options
	}{
		{"firmware alone", options{user: "u", fw: true, fwHost: "10.0.0.1", fwFile: "img.rcks"}},
		{"reboot alone", options{user: "u", reboot: true}},
		{"factory alone", options{user: "u", factory: true}},
	} {
		if _, _, err := buildConfig(c.opt, "pw", ""); err != nil {
			t.Errorf("%s was rejected: %v", c.name, err)
		}
	}
}

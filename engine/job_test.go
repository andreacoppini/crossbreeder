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

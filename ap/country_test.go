package ap

import "testing"

// The transcripts here are taken from a real ZoneFlex H510, so the parser is
// pinned to the exact strings the CLI emits — "get countrycode" (no hyphen),
// "Country is ES", and the abbreviated "Fixed Ctry Code:" — rather than to a
// guess at them. The hyphenated "get country-code" is rejected by the AP, which
// is why it is not the command sent.
const h510BoardData = `rkscli: get boarddata
name:     H510
magic:    35333131
rev:      5.4
Serial#:  111902016180
Customer ID: 4bss
Model:    H510
V54 MAC Address Pool:  yes, size 16, base 1C:3A:60:1B:41:90
Fixed Ctry Code:  no
Antenna Info:  yes, value 0x00007978
OK
rkscli: get countrycode
Country is ES
OK
rkscli: `

func TestCountryParsedFromRealBoardData(t *testing.T) {
	var r Result
	zoneFlex.parse(h510BoardData, &r)
	if r.Country != "ES" {
		t.Errorf("country = %q, want ES", r.Country)
	}
	if r.CountryFixed == nil {
		t.Fatal("fixed flag not read from a board that reported it")
	}
	if *r.CountryFixed {
		t.Errorf("fixed = true, but the board said %q", "no")
	}
}

func TestCountryLockedIsReadAsLocked(t *testing.T) {
	var r Result
	zoneFlex.parse("Fixed Ctry Code:  yes\r\nCountry is US\r\n", &r)
	if r.Country != "US" {
		t.Errorf("country = %q, want US", r.Country)
	}
	if r.CountryFixed == nil || !*r.CountryFixed {
		t.Errorf("fixed = %v, want a reported true", r.CountryFixed)
	}
}

// Older firmware may report neither line. The flag must stay nil — silence, not
// a false — so a CSV or the console does not claim the code is unlocked when the
// AP never said so.
func TestCountryAbsentLeavesFlagUnset(t *testing.T) {
	var r Result
	zoneFlex.parse("get version\r\nVersion: 110.0.0.0.1347\r\nOK\r\n", &r)
	if r.Country != "" {
		t.Errorf("country = %q, want empty", r.Country)
	}
	if r.CountryFixed != nil {
		t.Errorf("fixed = %v, want nil when the AP did not report it", *r.CountryFixed)
	}
}

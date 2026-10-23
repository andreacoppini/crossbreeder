package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeR1 stands in for the RUCKUS One API: the token exchange, the paged AP
// query, and the per-AP password read, shaped exactly like the live responses
// captured from a real tenant.
type fakeR1 struct {
	srv       *httptest.Server
	tenant    string
	clientID  string
	secret    string
	aps       []r1AP
	passwords map[string]string // serial -> password
	denySN    map[string]bool   // serials this account may not read (403)
	pwCalls   atomic.Int64
	pageSize  int // if >0, force paging at this size
}

func newFakeR1(t *testing.T) *fakeR1 {
	f := &fakeR1{
		tenant:    "tenant-123",
		clientID:  "cid",
		secret:    "shh",
		passwords: map[string]string{},
		denySN:    map[string]bool{},
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/oauth2/token/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/"+f.tenant) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" ||
			r.Form.Get("client_id") != f.clientID || r.Form.Get("client_secret") != f.secret {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":[{"message":"bad credentials"}]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "jwt-for-" + f.clientID, "token_type": "Bearer", "expires_in": 7199,
		})
	})

	mux.HandleFunc("/venues/aps/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jwt-for-"+f.clientID {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		size := len(f.aps)
		if f.pageSize > 0 {
			size = f.pageSize
		}
		start := (body.Page - 1) * size
		end := start + size
		if start > len(f.aps) {
			start = len(f.aps)
		}
		if end > len(f.aps) {
			end = len(f.aps)
		}
		var data []map[string]any
		for _, a := range f.aps[start:end] {
			data = append(data, map[string]any{
				"serialNumber":  a.Serial,
				"venueId":       a.VenueID,
				"model":         a.Model,
				"networkStatus": map[string]any{"ipAddress": a.IP},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalCount": len(f.aps), "page": body.Page, "data": data,
		})
	})

	mux.HandleFunc("/venues/", func(w http.ResponseWriter, r *http.Request) {
		// /venues/{venueId}/aps/{serial}/passwords
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 5 || parts[2] != "aps" || parts[4] != "passwords" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sn := parts[3]
		f.pwCalls.Add(1)
		if f.denySN[sn] {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors":[{"message":"not permitted"}]}`))
			return
		}
		pw, ok := f.passwords[sn]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apPassword": pw, "updatedTime": "2026-10-23T12:00:00Z", "expireTime": "2026-10-24T12:00:00Z",
		})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// clientTo builds an r1Client pointed at the fake, bypassing region→host
// mapping (the fake is on 127.0.0.1).
func (f *fakeR1) client(t *testing.T) *r1Client {
	t.Helper()
	c, err := newR1ClientAt(context.Background(), f.srv.URL, f.tenant, f.clientID, f.secret)
	if err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	return c
}

func TestR1SignInRejectsBadSecret(t *testing.T) {
	f := newFakeR1(t)
	_, err := newR1ClientAt(context.Background(), f.srv.URL, f.tenant, f.clientID, "wrong")
	if err == nil {
		t.Fatal("expected a sign-in failure with the wrong secret")
	}
	if !strings.Contains(err.Error(), "bad credentials") {
		t.Errorf("error did not carry the API's reason: %v", err)
	}
}

func TestR1ListAPsPages(t *testing.T) {
	f := newFakeR1(t)
	for i := 0; i < 25; i++ {
		f.aps = append(f.aps, r1AP{Serial: string(rune('a'+i)) + "sn", VenueID: "v1", IP: "10.0.0." + itoa(i), Model: "H510"})
	}
	f.pageSize = 10 // force three pages
	got, err := f.client(t).listAPs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 25 {
		t.Fatalf("got %d APs across pages, want 25", len(got))
	}
}

func TestR1PasswordAndDenied(t *testing.T) {
	f := newFakeR1(t)
	f.aps = []r1AP{{Serial: "SN1", VenueID: "v1", IP: "10.0.0.1"}}
	f.passwords["SN1"] = "s3cret!!"
	c := f.client(t)
	pw, err := c.apPassword(context.Background(), "v1", "SN1")
	if err != nil || pw != "s3cret!!" {
		t.Fatalf("password = %q, err = %v", pw, err)
	}
	f.denySN["SN1"] = true
	if _, err := c.apPassword(context.Background(), "v1", "SN1"); err == nil {
		t.Error("expected a 403 to surface as an error")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

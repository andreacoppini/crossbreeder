package netprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchBreaksTheRequestIntoPhases(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond) // stand in for server think time
		w.Write([]byte("welcome to the intranet"))
	}))
	defer srv.Close()

	r := Fetch(context.Background(), WebTest{
		Name: "intranet", URL: srv.URL, Insecure: true, ExpectBody: "intranet",
	})
	if !r.OK() {
		t.Fatalf("fetch failed: %v", r.Err)
	}
	if r.Status != 200 || r.Bytes == 0 {
		t.Fatalf("status = %d, bytes = %d", r.Status, r.Bytes)
	}
	if r.Connect <= 0 {
		t.Error("no connect time was recorded")
	}
	if r.TLS <= 0 {
		t.Error("no TLS handshake time was recorded")
	}
	if r.TTFB < 20*time.Millisecond {
		t.Errorf("TTFB = %v, which is shorter than the server's own delay", r.TTFB)
	}
	if r.Total < r.TTFB {
		t.Errorf("total %v is shorter than TTFB %v", r.Total, r.TTFB)
	}
	if r.CertExpiry.IsZero() || r.TLSVersion == "" {
		t.Errorf("no certificate detail was captured: %+v", r)
	}
	if r.Reused {
		t.Error("the connection was reused, which would hide the handshake cost")
	}
}

func TestFetchFailsOnAnUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sign in first", http.StatusForbidden)
	}))
	defer srv.Close()

	r := Fetch(context.Background(), WebTest{URL: srv.URL})
	if r.OK() {
		t.Fatal("HTTP 403 was treated as a healthy application")
	}
	if r.Status != 403 {
		t.Errorf("status = %d", r.Status)
	}
}

func TestFetchRecordsRedirectsWithoutFollowingThem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		w.Write([]byte("login page"))
	}))
	defer srv.Close()

	r := Fetch(context.Background(), WebTest{URL: srv.URL, ExpectStatus: 200})
	if r.OK() {
		t.Fatal("a redirect to a login page passed as a working application")
	}
	if len(r.Redirects) != 1 {
		t.Fatalf("redirects = %v", r.Redirects)
	}
}

func TestDetectCaptivePortal(t *testing.T) {
	clean := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer clean.Close()
	if st := DetectCaptivePortal(context.Background(), clean.URL, time.Second); st.Detected {
		t.Errorf("a clean 204 was reported as a portal: %+v", st)
	}

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://wifi.example.com/login?mac=aa:bb", http.StatusFound)
	}))
	defer portal.Close()
	st := DetectCaptivePortal(context.Background(), portal.URL, time.Second)
	if !st.Detected {
		t.Fatal("an intercepted probe was not reported as a portal")
	}
	if st.PortalURL != "https://wifi.example.com/login?mac=aa:bb" {
		t.Errorf("portal URL = %q", st.PortalURL)
	}
}

func TestCertificateExpiryReadsTheLeaf(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	addr := srv.Listener.Addr().String()

	cert, err := CertificateExpiry(context.Background(), addr, 3*time.Second)
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	if cert.NotAfter.Before(time.Now()) {
		t.Errorf("the test server's certificate reads as expired: %v", cert.NotAfter)
	}
}

func TestFetchReportsAConnectionRefusedPlainly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	r := Fetch(context.Background(), WebTest{URL: url, Timeout: time.Second})
	if r.OK() {
		t.Fatal("a closed port answered")
	}
	if got := r.Err.Error(); len(got) > 0 && got[0] == 'G' {
		t.Errorf("the URL wrapper was not stripped: %v", r.Err)
	}
}

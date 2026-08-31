package netprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"
)

// WebTest is one HTTP or HTTPS fetch, timed phase by phase. Every application
// test — Microsoft 365, Salesforce, an intranet page — is one of these.
type WebTest struct {
	Name    string
	URL     string
	Method  string
	Host    string        // override the Host header / SNI, for testing one node of a pool
	Timeout time.Duration // whole request, default 10s
	// ExpectStatus, when non-zero, is the status the page must answer with.
	// Anything else is a failure even though bytes came back — which is how a
	// captive portal's 302 to a login page reads.
	ExpectStatus int
	// ExpectBody, when set, must appear in the first 64 KiB of the response.
	ExpectBody string
	// Resolver, when set, is the address of the DNS server to use for this
	// fetch, so an application test measures the resolver the client would
	// actually use rather than the sensor's system resolver.
	Resolver string
	// Insecure skips certificate verification. Off by default: a broken chain
	// on the path to a SaaS application is a finding, not something to hide.
	Insecure bool
	// MaxRedirects caps the chain. Default 10; 0 with Follow=false means the
	// first response is the answer.
	Follow       bool
	MaxRedirects int
}

// WebResult breaks the fetch into the phases an operator can act on. A page
// that takes four seconds because DNS took 3.9 of them is a DNS problem, and
// only per-phase timing says so.
type WebResult struct {
	Name       string
	URL        string
	DNS        time.Duration
	Connect    time.Duration
	TLS        time.Duration
	TTFB       time.Duration // request written to first byte of the response
	Total      time.Duration
	Status     int
	Bytes      int64
	RemoteAddr string
	Reused     bool
	Redirects  []string

	TLSVersion  string
	CipherSuite string
	CertSubject string
	CertIssuer  string
	CertExpiry  time.Time

	Err error
}

// OK reports whether the fetch produced the response the test asked for.
func (r WebResult) OK() bool { return r.Err == nil }

// DaysToCertExpiry is negative once the certificate has expired.
func (r WebResult) DaysToCertExpiry() int {
	if r.CertExpiry.IsZero() {
		return 0
	}
	return int(time.Until(r.CertExpiry).Hours() / 24)
}

// Fetch runs one web test. Each call opens its own connection: a reused
// connection would hide exactly the handshake costs the test exists to
// measure.
func Fetch(ctx context.Context, t WebTest) WebResult {
	res := WebResult{Name: t.Name, URL: t.URL}
	if t.Timeout <= 0 {
		t.Timeout = 10 * time.Second
	}
	if t.Method == "" {
		t.Method = http.MethodGet
	}
	if t.MaxRedirects == 0 {
		t.MaxRedirects = 10
	}
	if _, err := url.Parse(t.URL); err != nil {
		res.Err = err
		return res
	}

	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	var dnsStart, connStart, tlsStart, wroteAt time.Time
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone: func(httptrace.DNSDoneInfo) {
			if !dnsStart.IsZero() {
				res.DNS = time.Since(dnsStart)
			}
		},
		ConnectStart: func(string, string) { connStart = time.Now() },
		ConnectDone: func(_, addr string, err error) {
			if err == nil && !connStart.IsZero() {
				res.Connect = time.Since(connStart)
				res.RemoteAddr = addr
			}
		},
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			if !tlsStart.IsZero() {
				res.TLS = time.Since(tlsStart)
			}
			if err == nil {
				res.TLSVersion = tlsVersionName(cs.Version)
				res.CipherSuite = tls.CipherSuiteName(cs.CipherSuite)
				if len(cs.PeerCertificates) > 0 {
					leaf := cs.PeerCertificates[0]
					res.CertSubject = leaf.Subject.CommonName
					res.CertIssuer = leaf.Issuer.CommonName
					res.CertExpiry = leaf.NotAfter
				}
			}
		},
		GotConn:              func(i httptrace.GotConnInfo) { res.Reused = i.Reused },
		WroteRequest:         func(httptrace.WroteRequestInfo) { wroteAt = time.Now() },
		GotFirstResponseByte: func() { res.TTFB = time.Since(wroteAt) },
	}

	transport := &http.Transport{
		DisableKeepAlives:   true,
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: t.Timeout,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: t.Insecure, ServerName: t.Host},
		DialContext:         dialerFor(t.Resolver, t.Timeout).DialContext,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			res.Redirects = append(res.Redirects, req.URL.String())
			if !t.Follow {
				return http.ErrUseLastResponse
			}
			if len(via) >= t.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), t.Method, t.URL, nil)
	if err != nil {
		res.Err = err
		return res
	}
	req.Header.Set("User-Agent", UserAgent)
	if t.Host != "" {
		req.Host = t.Host
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Total = time.Since(start)
		res.Err = unwrapURLError(err)
		return res
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	res.Bytes = int64(len(body))
	// Drain whatever is left so the size is the page's, not the cap's.
	if n, _ := io.Copy(io.Discard, resp.Body); n > 0 {
		res.Bytes += n
	}
	res.Total = time.Since(start)
	if err != nil {
		res.Err = err
		return res
	}

	switch {
	case t.ExpectStatus != 0 && resp.StatusCode != t.ExpectStatus:
		res.Err = fmt.Errorf("answered HTTP %d, expected %d", resp.StatusCode, t.ExpectStatus)
	case t.ExpectStatus == 0 && resp.StatusCode >= 400:
		res.Err = fmt.Errorf("answered HTTP %d", resp.StatusCode)
	case t.ExpectBody != "" && !strings.Contains(string(body), t.ExpectBody):
		res.Err = fmt.Errorf("the response did not contain %q", t.ExpectBody)
	}
	return res
}

// UserAgent identifies the sensor in web tests. Some SaaS endpoints answer a
// blank agent with a challenge page, which would read as an outage.
var UserAgent = "CrossbreederSensor/1.0 (+https://github.com/andreacoppini/crossbreeder)"

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	}
	return fmt.Sprintf("0x%04x", v)
}

// dialerFor builds a dialer that resolves through a named server when one is
// given, so a web test measures the DNS the client under test would use.
func dialerFor(resolver string, timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	if resolver == "" {
		return d
	}
	server := addPort(resolver, "53")
	d.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var inner net.Dialer
			return inner.DialContext(ctx, network, server)
		},
	}
	return d
}

// unwrapURLError strips the wrapper the http client adds, so the reason an
// operator reads is "connection refused" rather than a repetition of the URL.
func unwrapURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err
	}
	return err
}

// PortalStatus is the outcome of a captive-portal check.
type PortalStatus struct {
	Detected bool
	// PortalURL is where the network tried to send us, when it said.
	PortalURL string
	Status    int
	Err       error
}

// DetectCaptivePortal asks an endpoint that is defined to answer 204 with an
// empty body. Anything else — a redirect, a login page, an injected 200 — means
// something on the path is intercepting, which is the difference between "the
// internet is down" and "this network wants you to sign in".
func DetectCaptivePortal(ctx context.Context, probeURL string, timeout time.Duration) PortalStatus {
	if probeURL == "" {
		probeURL = DefaultPortalProbe
	}
	r := Fetch(ctx, WebTest{
		Name: "captive portal", URL: probeURL, Timeout: timeout,
		ExpectStatus: http.StatusNoContent, Follow: false,
	})
	st := PortalStatus{Status: r.Status}
	switch {
	case r.Status == http.StatusNoContent && r.Bytes == 0:
		return st
	case r.Status == 0:
		st.Err = r.Err
		return st
	}
	st.Detected = true
	if len(r.Redirects) > 0 {
		st.PortalURL = r.Redirects[0]
	}
	return st
}

// DefaultPortalProbe is the endpoint used when a template does not name one.
// It is plain HTTP on purpose: an interception that only rewrites HTTP is
// still an interception, and HTTPS would fail closed instead of showing us the
// portal.
const DefaultPortalProbe = "http://connectivitycheck.gstatic.com/generate_204"

// CertificateExpiry connects, completes a handshake and reports what the far
// end presented, without fetching anything. It is how a sensor watches the
// expiry of a RADIUS or portal certificate it never browses.
func CertificateExpiry(ctx context.Context, hostport string, timeout time.Duration) (*x509.Certificate, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host, hostport = hostport, hostport+":443"
	}
	d := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(d, "tcp", hostport, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true, // we are reporting on the certificate, not trusting it
	})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("the server presented no certificate")
	}
	return certs[0], nil
}

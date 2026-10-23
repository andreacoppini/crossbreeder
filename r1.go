package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RUCKUS One (R1) keeps standalone APs on a per-AP CLI password that it rotates
// daily, so the operator cannot know it in advance. This pulls that password
// from the R1 API for each AP the run is about to touch, keyed by the AP's LAN
// address — the same address the operator already feeds in — and hands it to
// the SSH engine as one more credential to try.
//
// The flow, confirmed against a live tenant, is three calls:
//
//	POST  {host}/oauth2/token/{tenantId}         client_credentials -> bearer JWT
//	POST  {host}/venues/aps/query                -> every AP, with its IP + venue
//	GET   {host}/venues/{venueId}/aps/{sn}/passwords  -> {"apPassword": "..."}
//
// The password endpoint is role-gated on RUCKUS's side: a VAR/partner token
// reads it, a plain end-customer token may be refused. That is the AP owner's
// account to sort out, not something this tool can route around.

// r1MediaType is the versioned content type every R1 data call expects; the API
// answers oddly without it.
const r1MediaType = "application/vnd.ruckus.v1+json"

// r1Hosts maps the region the operator picks to the API host. These are the
// public api.* hosts, not the bare console hosts.
var r1Hosts = map[string]string{
	"us":   "api.ruckus.cloud",
	"eu":   "api.eu.ruckus.cloud",
	"asia": "api.asia.ruckus.cloud",
}

// r1Region normalises what the operator typed (US, eu, Europe, …) to a key in
// r1Hosts, or returns "" if it is not one of the three.
func r1Region(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "us", "na", "usa", "america", "":
		return "us" // the API's own default region
	case "eu", "europe":
		return "eu"
	case "asia", "ap", "apac":
		return "asia"
	default:
		return ""
	}
}

// r1Client is an authenticated session against one region's API.
type r1Client struct {
	host  string
	token string
	base  string // https://{host}
	hc    *http.Client
}

// r1AP is the slice of an AP record this tool cares about.
type r1AP struct {
	Serial  string
	VenueID string
	IP      string
	Model   string
}

// newR1Client exchanges the client credentials for a bearer token. A failure
// here is fatal to the run — it means the region, tenant or key is wrong — so
// the caller surfaces it rather than pressing on and failing every login.
func newR1Client(ctx context.Context, region, tenant, clientID, secret string) (*r1Client, error) {
	host, ok := r1Hosts[r1Region(region)]
	if !ok {
		return nil, fmt.Errorf("unknown RUCKUS One region %q: use us, eu or asia", region)
	}
	return newR1ClientAt(ctx, "https://"+host, tenant, clientID, secret)
}

// newR1ClientAt is newR1Client against an explicit base URL, so a test can
// point it at a local server. The region→host mapping is the only thing it
// skips.
func newR1ClientAt(ctx context.Context, base, tenant, clientID, secret string) (*r1Client, error) {
	if tenant == "" || clientID == "" || secret == "" {
		return nil, fmt.Errorf("RUCKUS One needs a tenant ID, client ID and secret")
	}
	host := base
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		host = u.Host
	}
	hc := &http.Client{Timeout: 30 * time.Second}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {secret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/oauth2/token/"+url.PathEscape(tenant), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RUCKUS One sign-in: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RUCKUS One sign-in failed (HTTP %d): %s", resp.StatusCode, r1Reason(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return nil, fmt.Errorf("RUCKUS One sign-in returned no token")
	}
	return &r1Client{host: host, token: tok.AccessToken, base: base, hc: hc}, nil
}

// do issues an authenticated request and returns the body on a 2xx.
func (c *r1Client) do(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", r1MediaType)
	if body != nil {
		req.Header.Set("Content-Type", r1MediaType)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return b, resp.StatusCode, nil
}

// listAPs returns every AP in the tenant, paged. The list is the whole tenant,
// not a per-address lookup, because one query is cheaper than one query per
// address and the match happens locally on the IP.
func (c *r1Client) listAPs(ctx context.Context) ([]r1AP, error) {
	const pageSize = 1000
	var out []r1AP
	for page := 1; ; page++ {
		reqBody, _ := json.Marshal(map[string]any{
			"fields":   []string{"serialNumber", "venueId", "networkStatus", "model"},
			"page":     page,
			"pageSize": pageSize,
		})
		body, code, err := c.do(ctx, http.MethodPost, "/venues/aps/query", reqBody)
		if err != nil {
			return nil, fmt.Errorf("RUCKUS One AP query: %w", err)
		}
		if code != http.StatusOK {
			return nil, fmt.Errorf("RUCKUS One AP query failed (HTTP %d): %s", code, r1Reason(body))
		}
		var pageResp struct {
			TotalCount int `json:"totalCount"`
			Data       []struct {
				SerialNumber  string `json:"serialNumber"`
				VenueID       string `json:"venueId"`
				Model         string `json:"model"`
				NetworkStatus struct {
					IPAddress string `json:"ipAddress"`
				} `json:"networkStatus"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &pageResp); err != nil {
			return nil, fmt.Errorf("RUCKUS One AP query: unreadable response: %w", err)
		}
		for _, d := range pageResp.Data {
			out = append(out, r1AP{
				Serial:  d.SerialNumber,
				VenueID: d.VenueID,
				IP:      d.NetworkStatus.IPAddress,
				Model:   d.Model,
			})
		}
		// Drive paging off the reported total rather than page fullness: a
		// server that returns a smaller page than asked for is not necessarily
		// at the end. An empty page is the backstop against a bad total.
		if len(pageResp.Data) == 0 || len(out) >= pageResp.TotalCount {
			break
		}
	}
	return out, nil
}

// apPassword fetches one AP's current CLI password. A 403/404 is not fatal: it
// means this account cannot read that AP's password, or the AP is not there, so
// the caller falls back to whatever other credentials were supplied.
func (c *r1Client) apPassword(ctx context.Context, venueID, serial string) (string, error) {
	path := fmt.Sprintf("/venues/%s/aps/%s/passwords", url.PathEscape(venueID), url.PathEscape(serial))
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", code, r1Reason(body))
	}
	var p struct {
		APPassword string `json:"apPassword"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", fmt.Errorf("unreadable response")
	}
	return p.APPassword, nil
}

// r1Reason pulls a short human message out of an R1 error body, falling back to
// the raw text. R1 errors carry an "errors":[{"message":...}] shape.
func r1Reason(body []byte) string {
	var e struct {
		Errors []struct {
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"errors"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil {
		if len(e.Errors) > 0 && e.Errors[0].Message != "" {
			return e.Errors[0].Message
		}
		if e.Message != "" {
			return e.Message
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "no detail"
	}
	return s
}

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/andreacoppini/crossbreeder/ap"
)

// r1Resolver holds an authenticated R1 session and the tenant's APs indexed by
// address. It is built once at the start of a run — so a wrong region, tenant
// or key fails before any AP is touched — and then asked, after the sweep, to
// pull passwords for the addresses that actually answered.
type r1Resolver struct {
	client *r1Client
	byIP   map[string][]r1AP
	user   string
	conc   int
}

// newR1Resolver signs in and lists the tenant's APs. Both calls are fatal on
// failure: they mean the credentials are wrong, not that one AP is unreachable.
func newR1Resolver(ctx context.Context, opt options) (*r1Resolver, error) {
	c, err := newR1Client(ctx, opt.r1Region, opt.r1Tenant, opt.r1ClientID, opt.r1Secret)
	if err != nil {
		return nil, err
	}
	aps, err := c.listAPs(ctx)
	if err != nil {
		return nil, err
	}
	byIP := make(map[string][]r1AP)
	for _, a := range aps {
		if a.IP != "" {
			byIP[a.IP] = append(byIP[a.IP], a)
		}
	}
	user := opt.r1User
	if user == "" {
		user = "admin"
	}
	conc := opt.concurrency
	if conc < 1 {
		conc = 1
	}
	return &r1Resolver{client: c, byIP: byIP, user: user, conc: conc}, nil
}

// tenantAPs is how many APs the tenant holds, for the sign-in message.
func (r *r1Resolver) tenantAPs() int {
	n := 0
	for _, v := range r.byIP {
		n += len(v)
	}
	return n
}

// r1PullSummary turns a pull into the log lines the operator sees. The plain
// line is always there; a masked result adds a warning, and when nothing but
// masks came back it says outright what that means, because that is a
// configuration problem the operator has to fix in RUCKUS One, not something
// any number of retries will change.
func r1PullSummary(p r1Pull, reachable int) []string {
	line := fmt.Sprintf("RUCKUS One: matched %d of %d reachable address(es); pulled %d password(s)",
		p.Matched, reachable, p.Pulled)
	if p.Failed > 0 {
		line += fmt.Sprintf("; %d could not be read", p.Failed)
	}
	out := []string{line}
	switch {
	case p.Masked > 0 && p.Pulled == 0:
		out = append(out, fmt.Sprintf(
			"WARNING: RUCKUS One returned every password masked (%d of %d as \"*****\"). "+
				"This API key can see the APs but not their passwords — a read-only key does this. "+
				"No R1 password will be tried; only the credentials you entered. "+
				"Use an API key with administrator rights to pull the real passwords.",
			p.Masked, p.Masked))
	case p.Masked > 0:
		out = append(out, fmt.Sprintf(
			"WARNING: RUCKUS One returned %d password(s) masked as \"*****\"; those were skipped, not sent. "+
				"This API key may lack the rights to read them.", p.Masked))
	}
	return out
}

// r1Pull is what pulling passwords for a run produced. Creds is read-only once
// returned. The counts are for the operator: how many reachable addresses R1
// matched, how many real passwords came back, how many R1 answered with a
// masked placeholder, and how many could not be read at all.
type r1Pull struct {
	Creds   map[string][]ap.Credentials
	Matched int
	Pulled  int
	Masked  int
	Failed  int
}

// credentialsFor pulls the current password for every given address that R1
// knows, concurrently. A duplicate address — the same private IP in two venues
// — yields one credential per candidate, so the engine can try them all. An
// address R1 does not know, or whose password this account cannot read, is
// simply absent.
//
// A masked password ("**********", what a read-only API key gets) is the case
// to be careful with: it is a 200 with a value in it, so it looks like success.
// It is counted and dropped here, never turned into a credential — sending it
// fails every login and makes the AP look like the problem.
func (r *r1Resolver) credentialsFor(ctx context.Context, ips []string) r1Pull {
	type task struct {
		ip string
		ap r1AP
	}
	var tasks []task
	seenIP := make(map[string]bool)
	for _, ip := range ips {
		for _, a := range r.byIP[ip] {
			tasks = append(tasks, task{ip: ip, ap: a})
			seenIP[ip] = true
		}
	}

	out := r1Pull{Creds: make(map[string][]ap.Credentials), Matched: len(seenIP)}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.conc)
	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pw, err := r.client.apPassword(ctx, t.ap.VenueID, t.ap.Serial)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, errR1Masked):
				out.Masked++
			case err != nil || pw == "":
				out.Failed++
			default:
				out.Creds[t.ip] = append(out.Creds[t.ip], ap.Credentials{User: r.user, Password: pw})
				out.Pulled++
			}
		}(t)
	}
	wg.Wait()
	return out
}

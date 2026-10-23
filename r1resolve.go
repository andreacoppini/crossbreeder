package main

import (
	"context"
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

// credentialsFor pulls the current password for every given address that R1
// knows, concurrently, and returns a read-only map of address to the
// credentials to try first. A duplicate address — the same private IP in two
// venues — yields one credential per candidate, so the engine can try them all,
// which is exactly the case this was asked to handle. An address R1 does not
// know, or whose password this account cannot read, is simply absent.
//
// The two counts are for the operator: how many of the reachable addresses R1
// matched, and how many passwords actually came back.
func (r *r1Resolver) credentialsFor(ctx context.Context, ips []string) (creds map[string][]ap.Credentials, matched, pulled int) {
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

	creds = make(map[string][]ap.Credentials)
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
			if err != nil || pw == "" {
				return
			}
			mu.Lock()
			creds[t.ip] = append(creds[t.ip], ap.Credentials{User: r.user, Password: pw})
			pulled++
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	return creds, len(seenIP), pulled
}

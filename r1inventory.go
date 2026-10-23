package main

import "sort"

// The browser's "pull addresses from RUCKUS One" flow needs the tenant's APs
// grouped by venue, with each AP's address and name, so it can offer the venues
// to choose from and drop the chosen ones into the Targets box. Conflict
// detection — the same address in more than one venue — is left to the browser,
// because it depends on which venues the operator ticks.

// r1InvAP is one selectable AP: its address and its name, which rides along as
// the second column of the Targets box and is ignored by the run.
type r1InvAP struct {
	IP   string `json:"ip"`
	Name string `json:"name"`
}

// r1InvVenue is a venue and the APs in it that have an address. NoIP counts the
// ones skipped for having none, so the console can say so rather than silently
// showing a short list.
type r1InvVenue struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	APs  []r1InvAP `json:"aps"`
	NoIP int       `json:"noIp"`
}

// groupR1Inventory folds the venue list and the AP list into the per-venue
// shape the console consumes. Venues with no APs at all are dropped — there is
// nothing to pull from them — but a venue whose APs all lack an address is
// kept, so the operator sees why it yielded nothing. The order is by venue
// name, so the picker is stable and readable.
func groupR1Inventory(venues []r1Venue, aps []r1AP) []r1InvVenue {
	nameOf := make(map[string]string, len(venues))
	for _, v := range venues {
		nameOf[v.ID] = v.Name
	}

	idx := make(map[string]*r1InvVenue)
	var order []*r1InvVenue
	get := func(venueID string) *r1InvVenue {
		if g, ok := idx[venueID]; ok {
			return g
		}
		name := nameOf[venueID]
		if name == "" {
			name = venueID // an AP in a venue the venue list did not name
		}
		g := &r1InvVenue{ID: venueID, Name: name}
		idx[venueID] = g
		order = append(order, g)
		return g
	}

	for _, a := range aps {
		g := get(a.VenueID)
		if a.IP == "" {
			g.NoIP++
			continue
		}
		g.APs = append(g.APs, r1InvAP{IP: a.IP, Name: a.Name})
	}

	out := make([]r1InvVenue, 0, len(order))
	for _, g := range order {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

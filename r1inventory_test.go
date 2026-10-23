package main

import (
	"context"
	"testing"
)

func TestListVenues(t *testing.T) {
	f := newFakeR1(t)
	f.venues = []r1Venue{{ID: "v2", Name: "Seville"}, {ID: "v1", Name: "Madliena"}}
	got, err := f.client(t).listVenues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d venues, want 2", len(got))
	}
}

// Grouping names each venue, keeps APs with an address, counts the ones without,
// drops empty venues, and orders by name — the shape the picker relies on.
func TestGroupR1Inventory(t *testing.T) {
	venues := []r1Venue{
		{ID: "v1", Name: "Madliena"},
		{ID: "v2", Name: "Seville"},
		{ID: "v3", Name: "Empty"}, // no APs — must not appear
	}
	aps := []r1AP{
		{Serial: "a", VenueID: "v2", IP: "192.168.77.34", Name: "Casita"},
		{Serial: "b", VenueID: "v1", IP: "192.168.5.160", Name: "Lift"},
		{Serial: "c", VenueID: "v1", IP: "", Name: "Offline"},        // no address
		{Serial: "d", VenueID: "v9", IP: "10.0.0.9", Name: "Orphan"}, // venue not listed
	}

	got := groupR1Inventory(venues, aps)
	if len(got) != 3 {
		t.Fatalf("got %d venues, want 3 (Empty dropped, orphan kept)", len(got))
	}
	// Ordered by name: Casita's venue "Seville", "Madliena", and the orphan whose
	// name falls back to its id "v9" — sort is Madliena < Seville < v9.
	if got[0].Name != "Madliena" || got[1].Name != "Seville" || got[2].Name != "v9" {
		t.Errorf("order = %q/%q/%q", got[0].Name, got[1].Name, got[2].Name)
	}

	var mad r1InvVenue
	for _, v := range got {
		if v.ID == "v1" {
			mad = v
		}
	}
	if len(mad.APs) != 1 || mad.APs[0].IP != "192.168.5.160" || mad.APs[0].Name != "Lift" {
		t.Errorf("Madliena APs = %+v", mad.APs)
	}
	if mad.NoIP != 1 {
		t.Errorf("Madliena NoIP = %d, want 1 (the offline AP)", mad.NoIP)
	}
	// The orphan venue is named after its id when the venue list did not carry it.
	var orphan r1InvVenue
	for _, v := range got {
		if v.ID == "v9" {
			orphan = v
		}
	}
	if orphan.Name != "v9" || len(orphan.APs) != 1 {
		t.Errorf("orphan venue = %+v", orphan)
	}
}

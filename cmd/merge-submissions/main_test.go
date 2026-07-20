package main

import (
	"testing"
)

func TestUpsertEntryReplacesOlderVersion(t *testing.T) {
	entries := []registryEntry{
		{ID: "jira", Version: "0.1.0", Name: "Jira"},
	}
	err := upsertEntry(&entries, "plugin", registryEntry{ID: "jira", Version: "0.1.1", Name: "Jira"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Version != "0.1.1" {
		t.Fatalf("want 0.1.1, got %s", entries[0].Version)
	}
}

func TestUpsertEntryRejectsOlderVersion(t *testing.T) {
	entries := []registryEntry{
		{ID: "jira", Version: "0.1.1", Name: "Jira"},
	}
	err := upsertEntry(&entries, "plugin", registryEntry{ID: "jira", Version: "0.1.0", Name: "Jira"})
	if err == nil {
		t.Fatal("expected error for older version")
	}
}

func TestUpsertEntryRejectsDuplicateSameVersion(t *testing.T) {
	entries := []registryEntry{
		{ID: "jira", Version: "0.1.1", Name: "Jira"},
	}
	err := upsertEntry(&entries, "plugin", registryEntry{ID: "jira", Version: "0.1.1", Name: "Jira"})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.1", -1},
		{"0.1.1", "0.1.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.0-beta", 1},
	}
	for _, tc := range cases {
		got, err := compareSemver(tc.a, tc.b)
		if err != nil {
			t.Fatalf("%s vs %s: %v", tc.a, tc.b, err)
		}
		if got != tc.want {
			t.Fatalf("%s vs %s: got %d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

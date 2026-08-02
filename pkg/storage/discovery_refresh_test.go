package storage

import (
	"testing"
	"time"
)

func TestDiscoveryRefreshPersistsOnlyAggregateFields(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	started := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	refresh := DiscoveryRefresh{StartedAt: started, FinishedAt: started.Add(time.Minute), NewEligible: 3, ErrorClass: "NETWORK"}
	if err := SetDiscoveryRefresh(refresh); err != nil {
		t.Fatalf("SetDiscoveryRefresh: %v", err)
	}
	got, found, err := GetDiscoveryRefresh()
	if err != nil || !found {
		t.Fatalf("GetDiscoveryRefresh = (%+v, %v, %v)", got, found, err)
	}
	if got.NewEligible != 3 || got.ErrorClass != "network" || !got.StartedAt.Equal(started) {
		t.Errorf("stored refresh = %+v", got)
	}
	if err := SetDiscoveryRefresh(DiscoveryRefresh{StartedAt: started, FinishedAt: started, NewEligible: 0, ErrorClass: "https://private.example/job"}); err != nil {
		t.Fatalf("SetDiscoveryRefresh unknown class: %v", err)
	}
	got, _, _ = GetDiscoveryRefresh()
	if got.ErrorClass != "unknown" {
		t.Errorf("unapproved error class = %q, want unknown", got.ErrorClass)
	}
}

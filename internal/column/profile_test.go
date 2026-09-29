package column

import (
	"math"
	"sync"
	"testing"
)

func TestPutRejectsBadReadingsWithoutCorruptingProfile(t *testing.T) {
	profile := NewProfile()
	if err := profile.Put(TrayReading{Tray: 1, Temperature: 100, Pressure: 1.0}); err != nil {
		t.Fatalf("valid reading rejected: %v", err)
	}
	if err := profile.Put(TrayReading{Tray: 20, Temperature: 80, Pressure: 0.9}); err != nil {
		t.Fatalf("valid reading rejected: %v", err)
	}

	bad := []TrayReading{
		{Tray: 0, Temperature: 90, Pressure: 1.0},
		{Tray: 201, Temperature: 90, Pressure: 1.0},
		{Tray: 5, Temperature: math.NaN(), Pressure: 1.0},
		{Tray: 5, Temperature: 90, Pressure: math.NaN()},
	}
	for _, reading := range bad {
		if err := profile.Put(reading); err == nil {
			t.Errorf("expected error for invalid reading %+v", reading)
		}
	}

	if got := profile.Count(); got != 2 {
		t.Fatalf("valid profile mutated by bad readings: count=%d, want 2", got)
	}
	snapshot := profile.Snapshot()
	if len(snapshot.Ordered()) != 2 {
		t.Fatalf("snapshot should still hold the two valid readings, got %d", len(snapshot.Ordered()))
	}
}

func TestQueryRunsAgainstOneConsistentSnapshot(t *testing.T) {
	profile := NewProfile()
	for _, reading := range []TrayReading{
		{Tray: 1, Temperature: 100, Pressure: 1.0},
		{Tray: 10, Temperature: 95, Pressure: 1.0},
		{Tray: 20, Temperature: 90, Pressure: 1.0},
	} {
		if err := profile.Put(reading); err != nil {
			t.Fatalf("seed reading rejected: %v", err)
		}
	}

	snapshot := profile.Snapshot()
	// A new reading after the snapshot was taken must not appear in it.
	if err := profile.Put(TrayReading{Tray: 15, Temperature: 92, Pressure: 1.0}); err != nil {
		t.Fatalf("update rejected: %v", err)
	}
	if got := len(snapshot.Ordered()); got != 3 {
		t.Fatalf("snapshot mutated by later Put: got %d readings, want 3", got)
	}
	if got := snapshot.Count(); got != 3 {
		t.Fatalf("snapshot count = %d, want 3", got)
	}
	gradient, err := snapshot.Gradient()
	if err != nil {
		t.Fatalf("snapshot gradient error: %v", err)
	}
	if gradient != -10.0/19.0 {
		t.Fatalf("snapshot gradient = %v, want %v", gradient, -10.0/19.0)
	}
	if !snapshot.Stable(40) {
		t.Fatal("snapshot should be stable")
	}
	// Live profile did accept the update.
	if got := profile.Count(); got != 4 {
		t.Fatalf("live profile count = %d, want 4", got)
	}
}

// TestProfileConcurrentPutAndQuery must pass under -race: continuous
// ingestion alongside concurrent gradient/stability/snapshot reads.
func TestProfileConcurrentPutAndQuery(t *testing.T) {
	profile := NewProfile()
	for _, reading := range []TrayReading{
		{Tray: 1, Temperature: 118, Pressure: 1.1},
		{Tray: 20, Temperature: 82, Pressure: 0.9},
	} {
		if err := profile.Put(reading); err != nil {
			t.Fatalf("seed reading rejected: %v", err)
		}
	}

	var wg sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				tray := (i+seed)%200 + 1
				_ = profile.Put(TrayReading{Tray: tray, Temperature: 80 + float64(tray%30), Pressure: 1.0})
				// Invalid readings sprinkled in must never corrupt state.
				_ = profile.Put(TrayReading{Tray: 999, Temperature: math.NaN(), Pressure: 1.0})
			}
		}(writer)
	}
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				snapshot := profile.Snapshot()
				readings := snapshot.Ordered()
				_, _ = snapshot.Gradient()
				_ = snapshot.Stable(40)
				// Snapshot reads must agree with each other.
				if snapshot.Count() != len(readings) {
					t.Errorf("inconsistent snapshot: count=%d ordered=%d", snapshot.Count(), len(readings))
				}
				// Backwards-compatible direct accessors must also be safe.
				_ = profile.Ordered()
				_, _ = profile.Gradient()
				_ = profile.Stable(40)
				_ = profile.Count()
			}
		}()
	}
	wg.Wait()

	if profile.Count() != 200 {
		t.Fatalf("expected all 200 valid trays present, got %d", profile.Count())
	}
}

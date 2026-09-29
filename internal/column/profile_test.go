package column

import (
	"math"
	"sync"
	"testing"
)

func TestPutRejectsBadReadingsWithoutTouchingProfile(t *testing.T) {
	profile := NewProfile()
	if err := profile.Put(TrayReading{Tray: 1, Temperature: 118, Pressure: 1.1}); err != nil {
		t.Fatalf("valid put rejected: %v", err)
	}
	if err := profile.Put(TrayReading{Tray: 20, Temperature: 82, Pressure: 0.9}); err != nil {
		t.Fatalf("valid put rejected: %v", err)
	}

	for _, bad := range []TrayReading{
		{Tray: 0, Temperature: 100, Pressure: 1},
		{Tray: 201, Temperature: 100, Pressure: 1},
		{Tray: 5, Temperature: math.NaN(), Pressure: 1},
		{Tray: 5, Temperature: 100, Pressure: math.NaN()},
	} {
		if err := profile.Put(bad); err == nil {
			t.Fatalf("bad reading accepted: %+v", bad)
		}
	}

	if got := profile.Count(); got != 2 {
		t.Fatalf("bad readings altered profile, count = %d, want 2", got)
	}
	report := profile.Report(40)
	if !report.GradientReady || len(report.Readings) != 2 {
		t.Fatalf("valid profile lost after bad puts: %+v", report)
	}
}

func TestReportIsConsistentSnapshot(t *testing.T) {
	profile := NewProfile()
	_ = profile.Put(TrayReading{Tray: 1, Temperature: 100, Pressure: 1})
	_ = profile.Put(TrayReading{Tray: 2, Temperature: 90, Pressure: 1})

	report := profile.Report(10)
	if len(report.Readings) != 2 || !report.Stable {
		t.Fatalf("unexpected report: %+v", report)
	}
	// Mutating the returned slice must not affect later reports.
	report.Readings[0] = TrayReading{}
	again := profile.Report(10)
	if again.Readings[0].Temperature != 100 {
		t.Fatalf("caller mutated shared snapshot: %+v", again.Readings)
	}
}

func TestConcurrentUpdatesAndReports(t *testing.T) {
	profile := NewProfile()
	_ = profile.Put(TrayReading{Tray: 1, Temperature: 110, Pressure: 1})
	_ = profile.Put(TrayReading{Tray: 20, Temperature: 80, Pressure: 1})

	var wg sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				tray := 2 + (i+id)%18
				if err := profile.Put(TrayReading{Tray: tray, Temperature: 95 + float64(i%5), Pressure: 1}); err != nil {
					t.Errorf("put rejected: %v", err)
				}
				// Invalid samples race with queries and must never corrupt data.
				_ = profile.Put(TrayReading{Tray: 999, Temperature: 1, Pressure: 1})
			}
		}(writer)
	}
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				report := profile.Report(40)
				if len(report.Readings) < 2 || !report.GradientReady {
					t.Errorf("report lost seeded trays: %+v", report)
				}
				// Gradient must match the trays of the same snapshot.
				first := report.Readings[0]
				last := report.Readings[len(report.Readings)-1]
				want := (last.Temperature - first.Temperature) / float64(last.Tray-first.Tray)
				if report.Gradient != want {
					t.Errorf("gradient %v does not match snapshot %v", report.Gradient, want)
				}
				for index := 1; index < len(report.Readings); index++ {
					if report.Readings[index].Tray <= report.Readings[index-1].Tray {
						t.Errorf("report readings not ordered: %+v", report.Readings)
					}
				}
			}
		}()
	}
	wg.Wait()
}

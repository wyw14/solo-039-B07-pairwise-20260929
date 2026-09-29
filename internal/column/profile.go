package column

import (
	"errors"
	"math"
	"sort"
	"sync"
)

type TrayReading struct {
	Tray        int
	Temperature float64
	Pressure    float64
}

type Profile struct {
	mu       sync.RWMutex
	readings map[int]TrayReading
}

// ProfileSnapshot is an immutable point-in-time copy of a Profile. All analysis
// on a snapshot sees the same set of tray readings, even if the live profile
// keeps accepting updates afterwards.
type ProfileSnapshot struct {
	readings []TrayReading
}

func NewProfile() *Profile { return &Profile{readings: make(map[int]TrayReading)} }

func validateReading(reading TrayReading) error {
	if reading.Tray < 1 || reading.Tray > 200 {
		return errors.New("tray outside configured column")
	}
	if math.IsNaN(reading.Temperature) || math.IsNaN(reading.Pressure) {
		return errors.New("tray reading is not numeric")
	}
	return nil
}

// Put ingests one tray reading. Invalid readings are rejected before the
// profile is touched, so a bad reading cannot corrupt the valid profile.
func (p *Profile) Put(reading TrayReading) error {
	if err := validateReading(reading); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readings[reading.Tray] = reading
	return nil
}

// Snapshot takes a consistent copy of all current readings under a single
// read lock; analysis afterwards needs no lock and never blocks ingestion.
func (p *Profile) Snapshot() ProfileSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	keys := make([]int, 0, len(p.readings))
	for tray := range p.readings {
		keys = append(keys, tray)
	}
	sort.Ints(keys)
	readings := make([]TrayReading, 0, len(keys))
	for _, tray := range keys {
		readings = append(readings, p.readings[tray])
	}
	return ProfileSnapshot{readings: readings}
}

func (s ProfileSnapshot) Ordered() []TrayReading {
	out := make([]TrayReading, len(s.readings))
	copy(out, s.readings)
	return out
}

func (s ProfileSnapshot) Gradient() (float64, error) {
	if len(s.readings) < 2 {
		return 0, errors.New("profile needs at least two trays")
	}
	first := s.readings[0]
	last := s.readings[len(s.readings)-1]
	return (last.Temperature - first.Temperature) / float64(last.Tray-first.Tray), nil
}

func (s ProfileSnapshot) Stable(maxDelta float64) bool {
	if len(s.readings) < 2 {
		return false
	}
	for index := 1; index < len(s.readings); index++ {
		delta := s.readings[index].Temperature - s.readings[index-1].Temperature
		if delta > maxDelta || delta < -maxDelta {
			return false
		}
	}
	return true
}

func (s ProfileSnapshot) Count() int { return len(s.readings) }

// The methods below keep the Profile API compatible; each one is a single
// consistent read. Callers needing readings, gradient and stability from the
// same generation should take one Snapshot and use it for all three.

func (p *Profile) Ordered() []TrayReading                  { return p.Snapshot().Ordered() }
func (p *Profile) Gradient() (float64, error)             { return p.Snapshot().Gradient() }
func (p *Profile) Stable(maxDelta float64) bool           { return p.Snapshot().Stable(maxDelta) }
func (p *Profile) Count() int                             { return p.Snapshot().Count() }

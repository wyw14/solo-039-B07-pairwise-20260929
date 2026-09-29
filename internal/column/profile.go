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

// ProfileReport is a point-in-time view of a profile. Readings, gradient and
// stability are all derived from the same snapshot, so a query answer can
// never mix measurements from concurrent updates.
type ProfileReport struct {
	Readings      []TrayReading
	Gradient      float64
	Stable        bool
	GradientReady bool
}

type Profile struct {
	mu       sync.RWMutex
	readings map[int]TrayReading
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

// Put records a reading. Validation runs before the profile is touched, so a
// bad measurement is rejected and cannot corrupt the readings already stored.
func (p *Profile) Put(reading TrayReading) error {
	if err := validateReading(reading); err != nil {
		return err
	}
	p.mu.Lock()
	p.readings[reading.Tray] = reading
	p.mu.Unlock()
	return nil
}

// orderedLocked returns an ordered copy owned by the caller. The caller must
// hold p.mu (at least read).
func (p *Profile) orderedLocked() []TrayReading {
	keys := make([]int, 0, len(p.readings))
	for tray := range p.readings {
		keys = append(keys, tray)
	}
	sort.Ints(keys)
	out := make([]TrayReading, 0, len(keys))
	for _, tray := range keys {
		out = append(out, p.readings[tray])
	}
	return out
}

func (p *Profile) Ordered() []TrayReading {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.orderedLocked()
}

func gradientOf(values []TrayReading) (float64, bool) {
	if len(values) < 2 {
		return 0, false
	}
	first := values[0]
	last := values[len(values)-1]
	return (last.Temperature - first.Temperature) / float64(last.Tray-first.Tray), true
}

func stableOn(values []TrayReading, maxDelta float64) bool {
	if len(values) < 2 {
		return false
	}
	for index := 1; index < len(values); index++ {
		delta := values[index].Temperature - values[index-1].Temperature
		if delta > maxDelta || delta < -maxDelta {
			return false
		}
	}
	return true
}

func (p *Profile) Gradient() (float64, error) {
	gradient, ok := gradientOf(p.Ordered())
	if !ok {
		return 0, errors.New("profile needs at least two trays")
	}
	return gradient, nil
}

func (p *Profile) Stable(maxDelta float64) bool {
	return stableOn(p.Ordered(), maxDelta)
}

func (p *Profile) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.readings)
}

// Report builds readings, gradient and stability from a single snapshot taken
// under one read lock. The returned slice is a private copy: concurrent Puts
// keep progressing as soon as the lock is released and can never mutate data
// the caller is working from.
func (p *Profile) Report(maxDelta float64) ProfileReport {
	p.mu.RLock()
	readings := p.orderedLocked()
	p.mu.RUnlock()
	gradient, ready := gradientOf(readings)
	return ProfileReport{
		Readings:      readings,
		Gradient:      gradient,
		Stable:        stableOn(readings, maxDelta),
		GradientReady: ready,
	}
}

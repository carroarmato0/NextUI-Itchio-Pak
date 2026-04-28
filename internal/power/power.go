package power

import (
	"fmt"
	"path/filepath"
	"time"

	evdev "github.com/holoplot/go-evdev"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const (
	holdThreshold = 2 * time.Second
	cooldown      = 1 * time.Second
)

// Action is the power button action to perform.
type Action int

const (
	ActionSleep    Action = iota
	ActionShutdown
)

// Manager watches the power button input device and invokes notify when a
// sleep or shutdown action is detected. notify is called from a background
// goroutine and must be safe to call concurrently.
type Manager struct {
	notify func(Action)
}

// NewManager creates a Manager. notify must not be nil.
func NewManager(notify func(Action)) *Manager {
	if notify == nil {
		panic("power.NewManager: notify must not be nil")
	}
	return &Manager{notify: notify}
}

// Start launches the background goroutine. Returns immediately.
// If no power button device is found, it logs a warning and returns.
func (m *Manager) Start() {
	go m.run()
}

func (m *Manager) run() {
	dev, err := findPowerDeviceWithPattern("/dev/input/event*")
	if err != nil {
		logger.Warn("power: no power button device found: %v", err)
		return
	}
	defer dev.Close()
	logger.Info("power: monitoring %s for KEY_POWER events", dev.Path())

	var pressTime time.Time
	var cooldownUntil time.Time

	for {
		event, err := dev.ReadOne()
		if err != nil {
			logger.Error("power: read error: %v", err)
			return
		}
		if time.Now().Before(cooldownUntil) {
			continue
		}
		if event.Type != evdev.EV_KEY || event.Code != evdev.KEY_POWER {
			continue
		}
		switch event.Value {
		case 1: // key down
			pressTime = time.Now()
		case 2: // key held
			if !pressTime.IsZero() && time.Since(pressTime) >= holdThreshold {
				logger.Info("power: long press detected — shutdown")
				m.notify(ActionShutdown)
				pressTime = time.Time{}
				cooldownUntil = time.Now().Add(cooldown)
			}
		case 0: // key up
			if !pressTime.IsZero() {
				if time.Since(pressTime) < holdThreshold {
					logger.Info("power: short press detected — sleep")
					m.notify(ActionSleep)
				}
				pressTime = time.Time{}
				cooldownUntil = time.Now().Add(cooldown)
			}
		}
	}
}

// findPowerDeviceWithPattern scans devices matching pattern for one with
// KEY_POWER capability. Package-private; tests call it directly.
func findPowerDeviceWithPattern(pattern string) (*evdev.InputDevice, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("no devices found matching %q", pattern)
	}
	for _, path := range paths {
		dev, err := evdev.Open(path)
		if err != nil {
			continue
		}
		if deviceHasPowerKey(dev) {
			return dev, nil
		}
		dev.Close()
	}
	return nil, fmt.Errorf("no device with KEY_POWER found")
}

func deviceHasPowerKey(dev *evdev.InputDevice) bool {
	for _, t := range dev.CapableTypes() {
		if t != evdev.EV_KEY {
			continue
		}
		for _, code := range dev.CapableEvents(evdev.EV_KEY) {
			if code == evdev.KEY_POWER {
				return true
			}
		}
	}
	return false
}

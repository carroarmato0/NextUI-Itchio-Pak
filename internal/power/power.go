package power

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	evdev "github.com/holoplot/go-evdev"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const (
	holdThreshold = 2 * time.Second
	cooldown      = 1 * time.Second

	// powerKeyAlt is the keycode some my355 firmware stacks use for the power
	// button (rk805 pwrkey) instead of the standard KEY_POWER (116).
	powerKeyAlt evdev.EvCode = 102
)

// Action is the power button action to perform.
type Action int

const (
	ActionSleep Action = iota
	ActionShutdown
)

// Manager watches the power button input device and invokes notify when a
// sleep or shutdown action is detected. notify is called from a background
// goroutine and must be safe to call concurrently.
type Manager struct {
	notify        func(Action)
	postWakeUntil atomic.Int64 // unix nanos; goroutine ignores events before this time
}

// PostWake tells the Manager the device just woke from sleep. Call this
// immediately after the suspend command returns. Suppresses the buffered
// wake-up KEY_POWER event that would otherwise trigger an immediate re-sleep.
func (m *Manager) PostWake() {
	m.postWakeUntil.Store(time.Now().Add(3 * time.Second).UnixNano())
	logger.Info("power: post-wake inhibit active for 3s")
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
	dev, err := openPowerDevice()
	if err != nil {
		logger.Warn("power: no power button device found: %v", err)
		return
	}
	defer dev.Close()
	logger.Info("power: monitoring %s for power key events", dev.Path())

	var pressTime time.Time
	var cooldownUntil time.Time

	for {
		event, err := dev.ReadOne()
		if err != nil {
			logger.Error("power: read error: %v", err)
			return
		}
		now := time.Now()
		if now.Before(cooldownUntil) {
			continue
		}
		if t := m.postWakeUntil.Load(); t != 0 && now.Before(time.Unix(0, t)) {
			continue
		}
		if event.Type != evdev.EV_KEY || !isPowerKey(event.Code) {
			continue
		}
		switch event.Value {
		case 1: // key down
			pressTime = time.Now()
		case 2: // key held (autorepeat — not emitted by all drivers)
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
				} else {
					// Drivers that never emit autorepeat (e.g. my355 rk805 pwrkey)
					// reach here for long presses.
					logger.Info("power: long press detected — shutdown")
					m.notify(ActionShutdown)
				}
				pressTime = time.Time{}
				cooldownUntil = time.Now().Add(cooldown)
			}
		}
	}
}

// openPowerDevice returns the power button input device for the current platform.
// The my355 (Miyoo Flip) rk805 pwrkey lives at /dev/input/event2 and may advertise
// only keycode 102 rather than KEY_POWER, so we open it directly rather than scanning.
func openPowerDevice() (*evdev.InputDevice, error) {
	if firmware.Active().Device() == "my355" {
		logger.Debug("power: my355 platform — opening /dev/input/event2 directly")
		return evdev.Open("/dev/input/event2")
	}
	return findPowerDeviceWithPattern("/dev/input/event*")
}

// isPowerKey reports whether code is a power button keycode. Some my355 firmware
// stacks expose the power button as keycode 102 instead of the standard KEY_POWER.
func isPowerKey(code evdev.EvCode) bool {
	return code == evdev.KEY_POWER || code == powerKeyAlt
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

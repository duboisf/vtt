package hotkeys

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/keybind"
	"github.com/BurntSushi/xgbutil/xevent"
)

const autoRepeatReleaseDelay = 80 * time.Millisecond

type Registration struct {
	shortcut string
	x        *xgbutil.XUtil
	down     chan struct{}
	up       chan struct{}

	mu           sync.Mutex
	isDown       bool
	releaseTimer *time.Timer
}

func Register(shortcut string) (*Registration, error) {
	sequence, err := parse(shortcut)
	if err != nil {
		return nil, err
	}

	xu, err := xgbutil.NewConn()
	if err != nil {
		return nil, err
	}
	keybind.Initialize(xu)

	r := &Registration{
		shortcut: shortcut,
		x:        xu,
		down:     make(chan struct{}, 1),
		up:       make(chan struct{}, 1),
	}

	err = keybind.KeyPressFun(func(_ *xgbutil.XUtil, _ xevent.KeyPressEvent) {
		r.handlePress()
	}).Connect(xu, xu.RootWin(), sequence, true)
	if err != nil {
		return nil, err
	}

	err = keybind.KeyReleaseFun(func(_ *xgbutil.XUtil, _ xevent.KeyReleaseEvent) {
		r.handleRelease()
	}).Connect(xu, xu.RootWin(), sequence, true)
	if err != nil {
		return nil, err
	}

	go xevent.Main(xu)
	return r, nil
}

func (r *Registration) Shortcut() string {
	return r.shortcut
}

func (r *Registration) Down() <-chan struct{} {
	return r.down
}

func (r *Registration) Up() <-chan struct{} {
	return r.up
}

func (r *Registration) Close() error {
	r.mu.Lock()
	if r.releaseTimer != nil {
		r.releaseTimer.Stop()
		r.releaseTimer = nil
	}
	r.mu.Unlock()

	keybind.Detach(r.x, r.x.RootWin())
	xevent.Quit(r.x)
	r.x.Conn().Close()
	return nil
}

func (r *Registration) handlePress() {
	r.mu.Lock()
	if r.releaseTimer != nil {
		r.releaseTimer.Stop()
		r.releaseTimer = nil
		r.mu.Unlock()
		return
	}
	if r.isDown {
		r.mu.Unlock()
		return
	}
	r.isDown = true
	r.mu.Unlock()

	r.emit(r.down)
}

func (r *Registration) handleRelease() {
	r.mu.Lock()
	if !r.isDown || r.releaseTimer != nil {
		r.mu.Unlock()
		return
	}

	timer := time.NewTimer(autoRepeatReleaseDelay)
	r.releaseTimer = timer
	r.mu.Unlock()

	go r.awaitRelease(timer)
}

func (r *Registration) awaitRelease(timer *time.Timer) {
	<-timer.C

	r.mu.Lock()
	if r.releaseTimer != timer {
		r.mu.Unlock()
		return
	}
	r.releaseTimer = nil
	if !r.isDown {
		r.mu.Unlock()
		return
	}
	r.isDown = false
	r.mu.Unlock()

	r.emit(r.up)
}

func (r *Registration) emit(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func parse(shortcut string) (string, error) {
	parts := strings.FieldsFunc(strings.ToLower(shortcut), func(r rune) bool {
		return r == '+' || r == '-'
	})
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid hotkey %q", shortcut)
	}

	mods := make([]string, 0, len(parts)-1)
	for _, part := range parts[:len(parts)-1] {
		mod, ok := parseModifier(part)
		if !ok {
			return "", fmt.Errorf("unsupported modifier %q", part)
		}
		mods = append(mods, mod)
	}

	key, ok := parseKey(parts[len(parts)-1])
	if !ok {
		return "", fmt.Errorf("unsupported key %q", parts[len(parts)-1])
	}
	return strings.Join(append(mods, key), "-"), nil
}

func parseModifier(part string) (string, bool) {
	switch part {
	case "ctrl", "control":
		return "control", true
	case "alt", "option":
		return "mod1", true
	case "shift":
		return "shift", true
	case "cmd", "super", "meta", "win":
		return "mod4", true
	default:
		return "", false
	}
}

func parseKey(part string) (string, bool) {
	if len(part) == 1 {
		r := rune(part[0])
		if r >= 'a' && r <= 'z' {
			return part, true
		}
		if r >= '0' && r <= '9' {
			return part, true
		}
	}

	switch part {
	case "space":
		return "space", true
	case "enter", "return":
		return "Return", true
	case "tab":
		return "Tab", true
	case "escape", "esc":
		return "Escape", true
	case "comma":
		return "comma", true
	case "period", "dot":
		return "period", true
	case "slash":
		return "slash", true
	case "semicolon":
		return "semicolon", true
	case "apostrophe", "quote":
		return "apostrophe", true
	case "minus":
		return "minus", true
	case "equal", "equals":
		return "equal", true
	case "leftbracket":
		return "bracketleft", true
	case "rightbracket":
		return "bracketright", true
	case "backslash":
		return "backslash", true
	case "grave":
		return "grave", true
	}

	if strings.HasPrefix(part, "f") {
		n, err := strconv.Atoi(strings.TrimPrefix(part, "f"))
		if err == nil {
			switch {
			case n >= 1 && n <= 12:
				return fmt.Sprintf("F%d", n), true
			}
		}
	}

	return "", false
}

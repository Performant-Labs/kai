package doublecopy

import (
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Status is what the Settings page shows for the feature.
type Status string

const (
	// StatusOff: the setting is off (or the listener is not running for another reason).
	StatusOff Status = "off"
	// StatusRunning: listening.
	StatusRunning Status = "running"
	// StatusMissingPermission: the setting is on but Input Monitoring is not granted.
	StatusMissingPermission Status = "missing_permission"
	// StatusUnsupported: this platform has no listener.
	StatusUnsupported Status = "unsupported"
	// StatusError: the listener failed to start for another reason (see the log).
	StatusError Status = "error"
)

const (
	pollInterval = 30 * time.Millisecond
	// A copy is written by the app after the key-down: Chrome and Electron take tens of ms, a big
	// Sheets range more. Wait this long for the pasteboard's change count to move past the
	// key-down's, else nothing was copied (a Cmd+C with no selection) and nothing fires.
	settleInterval = 25 * time.Millisecond
	settleTimeout  = 600 * time.Millisecond
)

// Config wires a Service. Source, Pasteboard and Deliver are required.
type Config struct {
	Source     Source
	Pasteboard Pasteboard
	// Enabled reports the live setting; nil means enabled. It is checked again right before Deliver,
	// so switching the feature off during the wait for the pasteboard still wins.
	Enabled func() bool
	// Deliver receives the copied text: it shows the window and fills the input through the app's
	// one arrival path.
	Deliver func(text string)
	// RequestPermission asks the OS for the missing permission. Called only when the user switches
	// the feature on, never at launch and never again for the same switch-on.
	RequestPermission func()
	// OnPermissionMissing tells the user, once per switch-on (or launch), that the permission is
	// missing and how to grant it.
	OnPermissionMissing func()
	Sleep               func(time.Duration)
	Log                 *slog.Logger
}

// Service owns the listener's lifecycle and the double-copy pipeline.
type Service struct {
	cfg Config

	mu       sync.Mutex // guards the fields below
	status   Status
	applied  bool // Apply has run at least once: later calls are not "launch"
	prev     bool // the last requested state
	notified bool // the missing-permission message has been shown for this switch-on
	stop     chan struct{}

	detMu sync.Mutex
	det   Detector

	gen atomic.Int64 // bumped on every start/stop; a wait that outlives its generation delivers nothing
}

// New builds a Service. It does not start listening: Apply does.
func New(cfg Config) *Service {
	if cfg.Sleep == nil {
		cfg.Sleep = time.Sleep
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Service{cfg: cfg, status: StatusOff}
}

// Status reports the current state.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Apply makes the listener follow the setting: on starts it, off stops it. It is idempotent and
// cheap, so every settings save may call it. The first call is app launch; a later off-to-on call
// is the user switching the feature on, the only moment the permission is requested.
func (s *Service) Apply(on bool) {
	var after []func()
	s.mu.Lock()
	first := !s.applied
	wasOn := s.prev
	s.applied, s.prev = true, on

	switch {
	case !on:
		if s.stop != nil {
			close(s.stop)
			s.stop = nil
			s.cfg.Source.Stop()
		}
		s.gen.Add(1)
		s.status, s.notified = StatusOff, false
		s.resetDetector()
	case s.stop != nil:
		// Already listening.
	default:
		err := s.cfg.Source.Start()
		switch err {
		case nil:
			s.gen.Add(1)
			s.resetDetector()
			stop := make(chan struct{})
			s.stop = stop
			s.status, s.notified = StatusRunning, false
			go s.loop(stop)
		case ErrNoPermission:
			s.status = StatusMissingPermission
			if !first && !wasOn && s.cfg.RequestPermission != nil {
				after = append(after, s.cfg.RequestPermission)
			}
			if !s.notified {
				s.notified = true
				if s.cfg.OnPermissionMissing != nil {
					after = append(after, s.cfg.OnPermissionMissing)
				}
			}
		case ErrUnsupported:
			s.status = StatusUnsupported
		default:
			s.status = StatusError
			s.cfg.Log.Warn("double copy: listener failed to start", slog.Any("error", err))
		}
	}
	s.mu.Unlock()
	for _, f := range after {
		f()
	}
}

func (s *Service) resetDetector() {
	s.detMu.Lock()
	s.det.Reset()
	s.detMu.Unlock()
}

func (s *Service) loop(stop <-chan struct{}) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.Step()
		}
	}
}

// Step drains the source once and processes what it delivered. The loop calls it on a timer; tests
// call it directly.
func (s *Service) Step() {
	for _, ev := range s.cfg.Source.Poll() {
		s.detMu.Lock()
		pair := s.det.Feed(ev)
		s.detMu.Unlock()
		if pair {
			s.onPair(ev)
		}
	}
}

func (s *Service) enabled() bool { return s.cfg.Enabled == nil || s.cfg.Enabled() }

// onPair runs when ev is the second press of a quick pair. It never logs the text.
func (s *Service) onPair(ev Event) {
	if !s.enabled() {
		return
	}
	gen := s.gen.Load()
	if reason, blocked := Blocked(ev.Bundle, nil); blocked {
		s.cfg.Log.Debug("double copy: skipped", slog.String("reason", reason))
		return
	}
	if ev.ChangeCount <= 0 {
		s.cfg.Log.Debug("double copy: skipped, pasteboard change count unknown at key-down")
		return
	}
	if !s.waitForCopy(ev.ChangeCount) {
		s.cfg.Log.Debug("double copy: skipped, the pasteboard did not change (nothing was copied)")
		return
	}
	if reason, blocked := Blocked(ev.Bundle, s.cfg.Pasteboard.Types()); blocked {
		s.cfg.Log.Debug("double copy: skipped", slog.String("reason", reason))
		return
	}
	text := s.cfg.Pasteboard.Text()
	if strings.TrimSpace(text) == "" {
		s.cfg.Log.Debug("double copy: skipped, the copy holds no text")
		return
	}
	if !s.enabled() || s.gen.Load() != gen {
		return
	}
	s.cfg.Log.Info("double copy: translating the copied text", slog.Int("length", len(text)))
	s.cfg.Deliver(text)
}

// waitForCopy waits, bounded, for the pasteboard's change count to move past base.
func (s *Service) waitForCopy(base int64) bool {
	for waited := time.Duration(0); ; waited += settleInterval {
		if s.cfg.Pasteboard.ChangeCount() > base {
			return true
		}
		if waited >= settleTimeout {
			return false
		}
		s.cfg.Sleep(settleInterval)
	}
}

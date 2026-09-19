package engine

import (
	"context"
	"errors"
	"time"
)

type StopResult struct {
	Confirmed     bool   `json:"confirmed"`
	Forced        bool   `json:"forced"`
	GracefulError string `json:"gracefulError,omitempty"`
}

// Stop always controls the already-verified native handle. A console/FIFO
// failure can trigger force, but an identity failure cannot trigger PID killing.
func Stop(id Identity, graceful, force time.Duration) (StopResult, error) {
	r := StopResult{}
	h, err := Open(id)
	if errors.Is(err, ErrGone) {
		r.Confirmed = true
		return r, nil
	}
	if err != nil {
		return r, err
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), graceful)
	err = h.Quit(ctx)
	cancel()
	if err != nil {
		r.GracefulError = err.Error()
	}
	alive, checkErr := h.Alive()
	if checkErr != nil {
		return r, checkErr
	}
	if !alive {
		r.Confirmed = true
		return r, nil
	}
	if err = h.Kill(); err != nil && !errors.Is(err, ErrGone) {
		return r, err
	}
	r.Forced = true
	deadline := time.NewTimer(force)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		alive, err = h.Alive()
		if err != nil {
			return r, err
		}
		if !alive {
			r.Confirmed = true
			return r, nil
		}
		select {
		case <-deadline.C:
			return r, context.DeadlineExceeded
		case <-tick.C:
		}
	}
}

package auth

import (
	"context"
	"errors"
	"time"
)

// errBcryptBusy is returned by bcryptGate.run when no slot frees up within
// the wait. Callers translate it into a 429 rather than a login verdict.
var errBcryptBusy = errors.New("bcrypt gate busy")

// bcryptGate bounds how many bcrypt comparisons run at once. bcrypt at cost
// 12 pins a core for a quarter of a second, so an unbounded burst of login
// attempts would starve every other request of CPU; the gate holds one slot
// per core and makes a caller that cannot get one within wait back off with
// 429 instead of queueing indefinitely.
type bcryptGate struct {
	slots chan struct{}
	wait  time.Duration
}

func newBcryptGate(size int, wait time.Duration) *bcryptGate {
	return &bcryptGate{slots: make(chan struct{}, size), wait: wait}
}

// run executes fn inside a slot. It returns errBcryptBusy when no slot frees
// up within the wait, or ctx's error when the caller goes away first.
func (g *bcryptGate) run(ctx context.Context, fn func()) error {
	timer := time.NewTimer(g.wait)
	defer timer.Stop()
	select {
	case g.slots <- struct{}{}:
	case <-timer.C:
		return errBcryptBusy
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-g.slots }()
	fn()
	return nil
}

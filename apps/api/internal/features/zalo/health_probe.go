package zalo

import (
	"context"
	"crypto/rand"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
)

// defaultProbeInterval is the base wait between sweeps. Verification costs a
// real login against Zalo, and frequent programmatic logins are what makes an
// account look automated, so it is deliberately long rather than tight.
const defaultProbeInterval = 15 * time.Minute

// jitterFraction is the share of the interval a sweep may be delayed by when no
// jitter is configured, so sweeps never settle into a fixed pattern.
const jitterFraction = 3

// ProbeOptions tunes the sweep. The zero value is the production configuration.
type ProbeOptions struct {
	// Interval is the base wait between sweeps.
	Interval time.Duration
	// Jitter is the maximum extra wait added to each interval. It defaults to a
	// third of Interval.
	Jitter time.Duration
}

// HealthProbe periodically checks that linked accounts still work, so the
// profile page can say "reconnect needed" before a teacher discovers it by
// having a send fail.
//
// It holds no state of its own: listing and verifying are supplied by the
// service, which already knows how to mark an account expired and drop its
// session. One sweep loop covers every account — never a goroutine per account,
// which would turn a roster into a burst of simultaneous logins.
type HealthProbe struct {
	accounts func(ctx context.Context) ([]uuid.UUID, error)
	verify   func(ctx context.Context, teacherID uuid.UUID) error
	log      *slog.Logger
	interval time.Duration
	jitter   time.Duration
}

// NewHealthProbe builds a probe. A nil logger falls back to slog.Default.
func NewHealthProbe(
	accounts func(ctx context.Context) ([]uuid.UUID, error),
	verify func(ctx context.Context, teacherID uuid.UUID) error,
	log *slog.Logger,
	opts ProbeOptions,
) *HealthProbe {
	if log == nil {
		log = slog.Default()
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultProbeInterval
	}
	if opts.Jitter <= 0 {
		opts.Jitter = opts.Interval / jitterFraction
	}
	return &HealthProbe{
		accounts: accounts,
		verify:   verify,
		log:      log,
		interval: opts.Interval,
		jitter:   opts.Jitter,
	}
}

// Run sweeps on an interval until ctx is cancelled. It blocks, so callers start
// it in a goroutine and cancel ctx at shutdown.
func (p *HealthProbe) Run(ctx context.Context) {
	for {
		timer := time.NewTimer(p.wait())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			p.Sweep(ctx)
		}
	}
}

// Sweep verifies every linked account once. A failure is not fatal to the
// sweep: one dead session must not hide the state of the rest. Verification
// itself records the outcome, so there is nothing to do here but keep going.
func (p *HealthProbe) Sweep(ctx context.Context) {
	teachers, err := p.accounts(ctx)
	if err != nil {
		p.log.Error("zalo: could not list linked accounts to verify", "error", err)
		return
	}
	if len(teachers) == 0 {
		return
	}

	for _, teacherID := range teachers {
		if ctx.Err() != nil {
			return
		}
		if err := p.verify(ctx, teacherID); err != nil {
			p.log.Warn("zalo: linked session failed verification", "teacher_id", teacherID, "error", err)
		}
	}
}

// wait spreads sweeps out over the jitter window. crypto/rand keeps this out of
// the "insecure randomness" conversation entirely; the cost is irrelevant a few
// times an hour.
func (p *HealthProbe) wait() time.Duration {
	if p.jitter <= 0 {
		return p.interval
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(p.jitter)))
	if err != nil {
		return p.interval
	}
	return p.interval + time.Duration(n.Int64())
}

package kanban

import (
	"time"

	"github.com/google/uuid"
)

// Config holds Service's tunables. Zero value is never used directly; use
// NewService, which applies defaults before Options run.
type Config struct {
	// MaxColumns is the per-tenant column cap enforced by CreateColumn.
	MaxColumns int
	// MaxNameLen is the maximum column name length (after trimming)
	// enforced by CreateColumn.
	MaxNameLen int
	clock      Clock
	idGen      func() uuid.UUID
}

const (
	defaultMaxColumns = 8
	defaultMaxNameLen = 40
)

// Option customizes a Service at construction time without breaking
// NewService's signature when a new tunable is added.
type Option func(*Config)

// WithMaxColumns overrides the default per-tenant column cap (8).
func WithMaxColumns(n int) Option {
	return func(c *Config) { c.MaxColumns = n }
}

// WithMaxNameLen overrides the default maximum column name length (40).
func WithMaxNameLen(n int) Option {
	return func(c *Config) { c.MaxNameLen = n }
}

// WithClock overrides the default wall-clock Clock, for deterministic tests.
func WithClock(clock Clock) Option {
	return func(c *Config) { c.clock = clock }
}

// WithIDGen overrides the default uuid.New id generator, for deterministic
// tests.
func WithIDGen(gen func() uuid.UUID) Option {
	return func(c *Config) { c.idGen = gen }
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Service is the Kanban use-case layer. Construct it with NewService; the
// zero value is not usable.
type Service struct {
	repos   Repositories
	uow     UnitOfWork
	policy  Policy
	members MemberChecker
	sink    EventSink
	cfg     Config
}

// NewService wires the ports into a Service. repos, uow, pol, members, and
// sink must all be non-nil; NewService does not validate this since a nil
// port fails loudly on first use, which is preferable to swallowing a
// programmer error.
func NewService(repos Repositories, uow UnitOfWork, pol Policy, members MemberChecker, sink EventSink, opts ...Option) *Service {
	cfg := Config{
		MaxColumns: defaultMaxColumns,
		MaxNameLen: defaultMaxNameLen,
		clock:      realClock{},
		idGen:      uuid.New,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Service{
		repos:   repos,
		uow:     uow,
		policy:  pol,
		members: members,
		sink:    sink,
		cfg:     cfg,
	}
}

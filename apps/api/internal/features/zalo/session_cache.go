package zalo

import (
	"sync"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/zalo/protocol"
)

// SessionCache holds the live Zalo session of each linked teacher so a restored
// session is paid for once rather than on every use. A session is an
// *http.Client with a cookie jar; it cannot be shared across processes, so this
// cache assumes the single API replica Teka runs. A second replica would keep
// its own sessions and re-login independently — correct, but wasteful, and it
// would double the login traffic Zalo sees.
//
// The cache is authoritative for nothing: every entry can be rebuilt from the
// stored credentials, so dropping one only costs a re-login.
type SessionCache struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*protocol.Session
	// evictions counts how many times each teacher's session has been dropped.
	// Restoring a session takes a network round-trip, and an unlink can land in
	// the middle of one; the count lets the restore notice on the way back that
	// what it holds is no longer wanted. It outlives the entry it describes, so
	// it cannot be folded into sessions.
	evictions map[uuid.UUID]uint64
}

// NewSessionCache returns an empty cache, ready for concurrent use.
func NewSessionCache() *SessionCache {
	return &SessionCache{
		sessions:  make(map[uuid.UUID]*protocol.Session),
		evictions: make(map[uuid.UUID]uint64),
	}
}

// Get returns the teacher's live session, if one has been restored.
func (c *SessionCache) Get(teacherID uuid.UUID) (*protocol.Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sess, ok := c.sessions[teacherID]
	return sess, ok
}

// Put stores a session, replacing any previous one for that teacher.
func (c *SessionCache) Put(teacherID uuid.UUID, sess *protocol.Session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions[teacherID] = sess
}

// Evict drops the teacher's session. It is called whenever the session stops
// being trustworthy: unlink, a rejected re-login, or a failed health check.
func (c *SessionCache) Evict(teacherID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, teacherID)
	c.evictions[teacherID]++
}

// Evictions reports how many times the teacher's session has been dropped.
// Read it before starting a restore and hand it back to PutUnlessEvicted, which
// is how a restore that an unlink overtook is recognised and discarded.
func (c *SessionCache) Evictions(teacherID uuid.UUID) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.evictions[teacherID]
}

// PutUnlessEvicted stores a session unless the teacher's session was evicted
// since the caller read the count, and reports whether it was stored.
//
// Deleting Teka's row revokes nothing on Zalo's side, so a login that started
// before an unlink still succeeds after it. Without this check the cache would
// end up holding a working account session for a teacher who withdrew consent,
// and nothing would ever drop it: the health probe only sweeps accounts that
// are still linked.
func (c *SessionCache) PutUnlessEvicted(teacherID uuid.UUID, sess *protocol.Session, since uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.evictions[teacherID] != since {
		return false
	}
	c.sessions[teacherID] = sess
	return true
}

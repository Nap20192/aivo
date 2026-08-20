package session

import (
	"sync"
	"time"

	"aivo/internal/menu/domain"

	"github.com/google/uuid"
)

// ponytail: in-memory, swap for shared store (Redis) if scaled beyond one
// process. Also no eviction sweep for stale sessionID/IP entries — they
// live for the lifetime of the process. Fine for MVP traffic; add a
// periodic sweep (or an LRU cap) if that ever shows up as real memory
// growth.

const (
	orderDebounce     = 30 * time.Second
	serviceRequestTTL = 2 * time.Minute
	ipWindow          = time.Minute
	ipLimit           = 20
)

type serviceKey struct {
	tableID uuid.UUID
	kind    domain.ServiceRequestKind
}

type ipCount struct {
	windowStart time.Time
	count       int
}

var (
	mu sync.Mutex

	lastOrder   = make(map[string]time.Time)
	openService = make(map[serviceKey]time.Time)
	ipCounts    = make(map[string]*ipCount)
)

// AllowOrder reports whether sessionID may submit an Order now: at most
// once per 30s per session.
func AllowOrder(sessionID string) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	if last, ok := lastOrder[sessionID]; ok && now.Sub(last) < orderDebounce {
		return false
	}
	lastOrder[sessionID] = now
	return true
}

// AllowServiceRequest reports whether tableID may open a new ServiceRequest
// of kind now: true only if no open (unacknowledged, unexpired) request of
// that kind is currently tracked for that table. Callers must call
// MarkAcknowledged once staff handles the request; unacknowledged requests
// auto-expire after 2 minutes regardless.
func AllowServiceRequest(tableID uuid.UUID, kind domain.ServiceRequestKind) bool {
	mu.Lock()
	defer mu.Unlock()

	key := serviceKey{tableID, kind}
	now := time.Now()
	if created, ok := openService[key]; ok && now.Sub(created) < serviceRequestTTL {
		return false
	}
	openService[key] = now
	return true
}

// MarkAcknowledged clears the open ServiceRequest tracked for tableID/kind,
// so a new one may be opened before the 2-minute auto-expiry.
func MarkAcknowledged(tableID uuid.UUID, kind domain.ServiceRequestKind) {
	mu.Lock()
	defer mu.Unlock()
	delete(openService, serviceKey{tableID, kind})
}

// AllowIP reports whether ip may make another request now: a fixed-window
// limit of 20 requests/minute, shared across AllowOrder and
// AllowServiceRequest callers.
func AllowIP(ip string) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	c, ok := ipCounts[ip]
	if !ok || now.Sub(c.windowStart) >= ipWindow {
		c = &ipCount{windowStart: now}
		ipCounts[ip] = c
	}
	c.count++
	return c.count <= ipLimit
}

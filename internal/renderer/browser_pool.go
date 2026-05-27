package renderer

import "sync"

// semaphore limits concurrent browser fetches using a buffered channel.
type semaphore struct {
	mu     sync.Mutex
	sem    chan struct{}
	closed bool
}

// newSemaphore creates a concurrency limiter with the provided slot count.
func newSemaphore(limit int) *semaphore {
	return &semaphore{sem: make(chan struct{}, limit)}
}

// acquire reserves one slot in the semaphore.
func (s *semaphore) acquire() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.sem <- struct{}{}
}

// release returns one slot to the semaphore.
func (s *semaphore) release() {
	select {
	case <-s.sem:
	default:
	}
}

// close permanently disables the semaphore.
func (s *semaphore) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	close(s.sem)
}

var globalPools sync.Map

// browserPoolForURL returns a shared semaphore for one browser endpoint.
func browserPoolForURL(wsURL string, size int) *semaphore {
	if pool, ok := globalPools.Load(wsURL); ok {
		return pool.(*semaphore)
	}
	pool := newSemaphore(size)
	globalPools.Store(wsURL, pool)
	return pool
}

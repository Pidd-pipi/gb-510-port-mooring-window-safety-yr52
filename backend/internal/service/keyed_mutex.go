package service

import "sync"

// keyedMutex serializes work per key (here: per berth). Combined with a
// SERIALIZABLE database transaction it guarantees that two concurrent approvals
// for the same berth slot cannot both observe "free" and both insert an active
// occupancy. Different berths still proceed in parallel.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	inUse sync.Mutex
	count int
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*keyedLock)}
}

func (m *keyedMutex) lock(key string) func() {
	m.mu.Lock()
	lock, exists := m.locks[key]
	if !exists {
		lock = &keyedLock{}
		m.locks[key] = lock
	}
	lock.count++
	m.mu.Unlock()

	lock.inUse.Lock()
	return func() {
		lock.inUse.Unlock()
		m.mu.Lock()
		lock.count--
		if lock.count == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}

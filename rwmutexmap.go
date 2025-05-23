package locker

import (
	"sync"
	"sync/atomic"
)

// RWMutexMap is a more convenient map[T]sync.RWMutex. It automatically makes
// and deletes mutexes as needed. Unlocked mutexes consume no memory.
//
// The zero value is a valid MutexMap.
type RWMutexMap[T comparable] struct {
	mu    sync.Mutex
	locks map[T]*rwlockCtr
}

// rwlockCtr is used by RWLocker to represent a lock with a given key.
type rwlockCtr struct {
	sync.RWMutex
	waiters atomic.Int32 // Number of callers waiting to acquire the lock
	readers atomic.Int32 // Number of readers currently holding the lock
}

var rwlockCtrPool = sync.Pool{New: func() any { return new(rwlockCtr) }}

func (l *RWMutexMap[T]) get(key T) *rwlockCtr {
	if l.locks == nil {
		l.locks = make(map[T]*rwlockCtr)
	}

	nameLock, exists := l.locks[key]
	if !exists {
		nameLock = rwlockCtrPool.Get().(*rwlockCtr)
		l.locks[key] = nameLock
	}
	return nameLock
}

// Lock locks the RWMutex identified by key for writing.
func (l *RWMutexMap[T]) Lock(key T) {
	l.mu.Lock()
	nameLock := l.get(key)

	// Increment the nameLock waiters while inside the main mutex.
	// This makes sure that the lock isn't deleted if `Lock` and `Unlock` are called concurrently.
	nameLock.waiters.Add(1)
	l.mu.Unlock()

	// Lock the nameLock outside the main mutex so we don't block other operations.
	// Once locked then we can decrement the number of waiters for this lock.
	nameLock.Lock()
	nameLock.waiters.Add(-1)
}

// RLock locks the RWMutex identified by key for reading.
func (l *RWMutexMap[T]) RLock(key T) {
	l.mu.Lock()
	nameLock := l.get(key)

	nameLock.waiters.Add(1)
	l.mu.Unlock()

	nameLock.RLock()
	// Increment the number of readers before decrementing the waiters
	// so concurrent calls to RUnlock will not see a glitch where both
	// waiters and readers are 0.
	nameLock.readers.Add(1)
	nameLock.waiters.Add(-1)
}

// Unlock unlocks the RWMutex identified by key.
//
// It is a run-time error if the lock is not locked for writing on entry to Unlock.
func (l *RWMutexMap[T]) Unlock(key T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	nameLock := l.get(key)
	// We don't have to do anything special to handle the error case:
	// l.get(key) will return an unlocked mutex.

	if nameLock.waiters.Load() <= 0 && nameLock.readers.Load() <= 0 {
		delete(l.locks, key)
		defer rwlockCtrPool.Put(nameLock)
	}
	nameLock.Unlock()
}

// RUnlock unlocks the RWMutex identified by key for reading.
//
// It is a run-time error if the lock is not locked for reading on entry to RUnlock.
func (l *RWMutexMap[T]) RUnlock(key T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	nameLock := l.get(key)
	nameLock.readers.Add(-1)

	if nameLock.waiters.Load() <= 0 && nameLock.readers.Load() <= 0 {
		delete(l.locks, key)
		defer rwlockCtrPool.Put(nameLock)
	}
	nameLock.RUnlock()
}

// Locker returns a [sync.Locker] interface that implements
// the [sync.Locker.Lock] and [sync.Locker.Unlock] methods
// by calling l.Lock(name) and l.Unlock(name).
func (l *RWMutexMap[T]) Locker(key T) sync.Locker {
	return nameRWLocker[T]{l: l, key: key}
}

// RLocker returns a [sync.Locker] interface that implements
// the [sync.Locker.Lock] and [sync.Locker.Unlock] methods
// by calling l.RLock(name) and l.RUnlock(name).
func (l *RWMutexMap[T]) RLocker(key T) sync.Locker {
	return nameRLocker[T]{l: l, key: key}
}

type nameRWLocker[T comparable] struct {
	l   *RWMutexMap[T]
	key T
}
type nameRLocker[T comparable] nameRWLocker[T]

func (n nameRWLocker[T]) Lock() {
	n.l.Lock(n.key)
}
func (n nameRWLocker[T]) Unlock() {
	n.l.Unlock(n.key)
}

func (n nameRLocker[T]) Lock() {
	n.l.RLock(n.key)
}
func (n nameRLocker[T]) Unlock() {
	n.l.RUnlock(n.key)
}

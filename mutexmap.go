/*
Package locker provides a mechanism for creating finer-grained locking to help
free up more global locks to handle other tasks.
*/
package locker

import (
	"strconv"
	"sync"
	"sync/atomic"
)

// MutexMap is a more convenient map[T]sync.Mutex. It automatically makes and
// deletes mutexes as needed. Unlocked mutexes consume no memory.
//
// The zero value is a valid MutexMap.
type MutexMap[T comparable] struct {
	mu    sync.Mutex
	locks map[T]*lockCtr
}

// lockCtr is used by Locker to represent a lock with a given key.
type lockCtr struct {
	sync.Mutex
	waiters atomic.Int32 // Number of callers waiting to acquire the lock
}

var lockCtrPool = sync.Pool{New: func() any { return new(lockCtr) }}

// Lock locks the mutex identified by key.
func (l *MutexMap[T]) Lock(key T) {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[T]*lockCtr)
	}

	nameLock, exists := l.locks[key]
	if !exists {
		nameLock = lockCtrPool.Get().(*lockCtr)
		l.locks[key] = nameLock

		if isTesting() {
			if !nameLock.TryLock() {
				panic("MutexMap: lock taken from pool is already locked")
			}
			nameLock.Unlock()
			if w := nameLock.waiters.Load(); w != 0 {
				panic("MutexMap: lock taken from pool has nonzero waiters counter" +
					" (waiters=" + strconv.Itoa(int(w)) + ")")
			}
		}
		// Defensive coding: force the waiters counter to a known value
		// just in case the lockCtr we got from the pool was recycled
		// and not properly reset.
		nameLock.waiters.Store(1)
	} else {
		// Increment the nameLock waiters while inside the main mutex.
		// This makes sure that the lock isn't deleted if `Lock` and
		// `Unlock` are called concurrently.
		nameLock.waiters.Add(1)
	}

	l.mu.Unlock()
	// Lock the nameLock outside the main mutex so we don't block other operations.
	// Once locked then we can decrement the number of waiters for this lock.
	nameLock.Lock()
	nameLock.waiters.Add(-1)
}

// Unlock unlocks the mutex identified by key.
//
// It is a run-time error if the mutex is not locked on entry to Unlock.
func (l *MutexMap[T]) Unlock(key T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	nameLock, exists := l.locks[key]
	if !exists {
		// Generate an un-recover()-able error without reaching into runtime internals.
		(&sync.Mutex{}).Unlock()
	}

	if w := nameLock.waiters.Load(); w <= 0 {
		delete(l.locks, key)

		// Putting a lock back into the pool when the waiters counter is
		// nonzero would result in some really nasty, subtle
		// misbehaviour -- which would only manifest when pool items get
		// reused, i.e. in situations with lots of lock churn. The
		// counters should be 0 when the lock is deleted from the map
		// and returned to the pool.
		if isTesting() && w < 0 {
			panic("MutexMap: lock unlocked with negative counter" +
				" (waiters=" + strconv.Itoa(int(w)) + ")")
		}
		// But given the consequences of that invariant being violated,
		// zero out the counter just in case to minimize the impact of
		// such bugs.
		nameLock.waiters.Store(0)
		defer lockCtrPool.Put(nameLock)
	}
	nameLock.Unlock()
}

type nameLocker[T comparable] struct {
	l   *MutexMap[T]
	key T
}

// Locker returns a [sync.Locker] interface that implements
// the [sync.Locker.Lock] and [sync.Locker.Unlock] methods
// by calling l.Lock(key) and l.Unlock(key).
func (l *MutexMap[T]) Locker(key T) sync.Locker {
	return nameLocker[T]{l: l, key: key}
}

func (n nameLocker[T]) Lock() {
	n.l.Lock(n.key)
}
func (n nameLocker[T]) Unlock() {
	n.l.Unlock(n.key)
}

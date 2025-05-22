package locker

import (
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRWMutex_Lock(t *testing.T) {
	var l RWMutexMap[string]
	l.Lock("test")
	ctr := l.locks["test"]

	if w := ctr.waiters.Load(); w != 0 {
		t.Fatalf("expected waiters to be 0, got %d", w)
	}

	chDone := make(chan struct{})
	go func() {
		l.Lock("test")
		close(chDone)
	}()

	chWaiting := make(chan struct{})
	go func() {
		for range time.Tick(1 * time.Millisecond) {
			if ctr.waiters.Load() == 1 {
				close(chWaiting)
				break
			}
		}
	}()

	select {
	case <-chWaiting:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for lock waiters to be incremented")
	}

	select {
	case <-chDone:
		t.Fatal("lock should not have returned while it was still held")
	default:
	}

	l.Unlock("test")

	select {
	case <-chDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("lock should have completed")
	}

	if w := ctr.waiters.Load(); w != 0 {
		t.Fatalf("expected waiters to be 0, got %d", w)
	}
}

func TestRWMutex_Unlock(t *testing.T) {
	var l RWMutexMap[string]

	l.Lock("test")
	l.Unlock("test")

	chDone := make(chan struct{})
	go func() {
		l.Lock("test")
		close(chDone)
	}()

	select {
	case <-chDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("lock should not be blocked")
	}
}

func TestRWMutex_RLock(t *testing.T) {
	var l RWMutexMap[string]
	rlocked := make(chan bool, 1)
	wlocked := make(chan bool, 1)
	n := 10

	go func() {
		for i := 0; i < n; i++ {
			l.RLock("test")
			l.RLock("test")
			rlocked <- true
			l.Lock("test")
			wlocked <- true
		}
	}()

	for i := 0; i < n; i++ {
		<-rlocked
		l.RUnlock("test")
		select {
		case <-wlocked:
			t.Fatal("RLock() didn't block Lock()")
		default:
		}
		l.RUnlock("test")
		<-wlocked
		select {
		case <-rlocked:
			t.Fatal("Lock() didn't block RLock()")
		default:
		}
		l.Unlock("test")
	}

	if len(l.locks) != 0 {
		t.Fatalf("expected no locks to be present in the map, got %d", len(l.locks))
	}
}

func TestRWMutex_Concurrency(t *testing.T) {
	var l RWMutexMap[string]

	var wg sync.WaitGroup
	for i := 0; i <= 10000; i++ {
		wg.Add(1)
		go func() {
			l.Lock("test")
			// if there is a concurrency issue, will very likely panic here
			l.Unlock("test")
			l.RLock("test")
			l.RUnlock("test")
			wg.Done()
		}()
	}

	chDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(chDone)
	}()

	select {
	case <-chDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for locks to complete")
	}

	// Since everything has unlocked this should not exist anymore
	if ctr, exists := l.locks["test"]; exists {
		t.Fatalf("lock should not exist: %v", ctr)
	}
}

func BenchmarkRWMutex(b *testing.B) {
	var l RWMutexMap[string]
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.Lock("test")
		l.Lock(strconv.Itoa(i))
		l.Unlock(strconv.Itoa(i))
		l.Unlock("test")
	}
}

func BenchmarkRWMutex_Parallel(b *testing.B) {
	var l RWMutexMap[string]
	b.SetParallelism(128)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Lock("test")
			l.Unlock("test")
		}
	})
}

func BenchmarkRWMutex_MoreKeys(b *testing.B) {
	var l RWMutexMap[string]
	var keys []string
	for i := 0; i < 64; i++ {
		keys = append(keys, strconv.Itoa(i))
	}
	b.SetParallelism(128)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			k := keys[rand.Intn(len(keys))]
			l.Lock(k)
			l.Unlock(k)
		}
	})
}

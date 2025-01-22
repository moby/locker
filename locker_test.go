package locker

import (
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestLockCounter(t *testing.T) {
	l := &lockCtr{}
	l.inc()

	if l.waiters != 1 {
		t.Fatal("counter inc failed")
	}

	l.dec()
	if l.waiters != 0 {
		t.Fatal("counter dec failed")
	}
}

func TestLockerLock(t *testing.T) {
	l := New[string]()
	Lock(l, "test")
	ctr := l.locks["test"]

	if ctr.count() != 0 {
		t.Fatalf("expected waiters to be 0, got :%d", ctr.waiters)
	}

	chDone := make(chan struct{})
	go func() {
		Lock(l, "test")
		close(chDone)
	}()

	chWaiting := make(chan struct{})
	go func() {
		for range time.Tick(1 * time.Millisecond) {
			if ctr.count() == 1 {
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

	if err := Unlock(l, "test"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-chDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("lock should have completed")
	}

	if ctr.count() != 0 {
		t.Fatalf("expected waiters to be 0, got: %d", ctr.count())
	}
}

func TestLockerUnlock(t *testing.T) {
	l := New[string]()

	Lock(l, "test")
	Unlock(l, "test")

	chDone := make(chan struct{})
	go func() {
		Lock(l, "test")
		close(chDone)
	}()

	select {
	case <-chDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("lock should not be blocked")
	}
}

func TestLockerConcurrency(t *testing.T) {
	l := New[string]()

	var wg sync.WaitGroup
	for i := 0; i <= 10000; i++ {
		wg.Add(1)
		go func() {
			Lock(l, "test")
			// if there is a concurrency issue, will very likely panic here
			Unlock(l, "test")
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

func BenchmarkLocker(b *testing.B) {
	l := New[string]()
	for i := 0; i < b.N; i++ {
		Lock(l, "test")
		Unlock(l, "test")
	}
}

func BenchmarkLockerParallel(b *testing.B) {
	l := New[string]()
	b.SetParallelism(128)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Lock(l, "test")
			Unlock(l, "test")
		}
	})
}

func BenchmarkLockerMoreKeys(b *testing.B) {
	l := New[string]()
	var keys []string
	for i := 0; i < 64; i++ {
		keys = append(keys, strconv.Itoa(i))
	}
	b.SetParallelism(128)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			k := keys[rand.Intn(len(keys))]
			Lock(l, k)
			Unlock(l, k)
		}
	})
}

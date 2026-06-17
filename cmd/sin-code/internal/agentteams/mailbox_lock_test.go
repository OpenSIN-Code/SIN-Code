// SPDX-License-Identifier: MIT
// Purpose: Tests for platform-specific mailbox locking (issue #342).
package agentteams

import (
	"sync"
	"testing"
)

func TestLockUnlock(t *testing.T) {
	m := newMailbox(t)
	if err := m.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := m.Unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestDoubleLockErrors(t *testing.T) {
	m := newMailbox(t)
	if err := m.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := m.Lock(); err == nil {
		t.Fatal("double lock must error")
	}
	_ = m.Unlock()
}

func TestUnlockWithoutLock(t *testing.T) {
	m := newMailbox(t)
	if err := m.Unlock(); err != nil {
		t.Fatalf("unlock without lock should be nil-error, got %v", err)
	}
}

func TestLockThenSendAfterUnlock(t *testing.T) {
	m := newMailbox(t)
	if err := m.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := m.Unlock(); err != nil {
		t.Fatal(err)
	}
	// Send should work after Unlock releases the file lock.
	_, _, err := m.Send(Message{ID: "lock-test", From: "x", Subject: "s", Body: "b"})
	if err != nil {
		t.Fatalf("send after unlock: %v", err)
	}
}

func TestConcurrentLockExclusion(t *testing.T) {
	m := newMailbox(t)
	const N = 10
	var wg sync.WaitGroup
	wg.Add(N)
	wins := make(chan int, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			if err := m.Lock(); err == nil {
				wins <- i
				_ = m.Unlock()
			}
		}(i)
	}
	wg.Wait()
	close(wins)
	count := 0
	for range wins {
		count++
	}
	if count == 0 {
		t.Fatal("at least one goroutine should acquire the lock")
	}
}

func TestLockUnlockRepeatable(t *testing.T) {
	m := newMailbox(t)
	for i := 0; i < 5; i++ {
		if err := m.Lock(); err != nil {
			t.Fatalf("lock iteration %d: %v", i, err)
		}
		if err := m.Unlock(); err != nil {
			t.Fatalf("unlock iteration %d: %v", i, err)
		}
	}
}

package transfer

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestManagerQueued(t *testing.T) {
	var m Manager
	m.Enqueue(Job{Name: "a", Run: func(context.Context, Progress) error { return nil }})
	if m.Queued() != 1 {
		t.Fatalf("queued=%d", m.Queued())
	}
	ctx := context.Background()
	done := make(chan struct{}, 1)
	m.DrainAsync(ctx, 1, nil, func(Job, error) { done <- struct{}{} }, nil)
	<-done
	if m.Queued() != 0 {
		t.Fatalf("após drenar queued=%d", m.Queued())
	}
}

func TestManagerRunningDuringJob(t *testing.T) {
	var m Manager
	m.Enqueue(Job{Name: "slow", Run: func(context.Context, Progress) error {
		time.Sleep(45 * time.Millisecond)
		return nil
	}})
	ctx := context.Background()
	var mu sync.Mutex
	maxR := 0
	m.DrainAsync(ctx, 1, func(_ Job) {
		mu.Lock()
		r := m.Running()
		if r > maxR {
			maxR = r
		}
		mu.Unlock()
	}, nil, nil)
	time.Sleep(120 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if maxR != 1 {
		t.Fatalf("pico running=%d", maxR)
	}
}

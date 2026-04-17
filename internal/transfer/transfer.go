package transfer

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Progress reporta bytes transferidos e total (total pode ser -1 se desconhecido).
type Progress func(done, total int64)

// Job descreve uma transferência executada em background.
type Job struct {
	Name string
	Run  func(ctx context.Context, on Progress) error
}

// Manager fila jobs e drena com um ou mais workers em paralelo.
type Manager struct {
	mu     sync.Mutex
	queue  []Job
	workMu sync.Mutex
}

// Enqueue adiciona um job à fila.
func (m *Manager) Enqueue(j Job) {
	m.mu.Lock()
	m.queue = append(m.queue, j)
	m.mu.Unlock()
}

func (m *Manager) pop() (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.queue) == 0 {
		return Job{}, false
	}
	j := m.queue[0]
	m.queue = m.queue[1:]
	return j, true
}

// DrainAsync processa jobs pendentes. parallel < 2 é estritamente sequencial.
func (m *Manager) DrainAsync(ctx context.Context, parallel int, onStart func(Job), onDone func(Job, error), onProgress Progress) {
	if parallel < 1 {
		parallel = 1
	}
	go func() {
		m.workMu.Lock()
		defer m.workMu.Unlock()
		if parallel == 1 {
			for {
				j, ok := m.pop()
				if !ok {
					return
				}
				if onStart != nil {
					onStart(j)
				}
				err := j.Run(ctx, onProgress)
				if onDone != nil {
					onDone(j, err)
				}
			}
		}
		sem := make(chan struct{}, parallel)
		var wg sync.WaitGroup
		for {
			j, ok := m.pop()
			if !ok {
				break
			}
			sem <- struct{}{}
			wg.Add(1)
			go func(job Job) {
				defer func() {
					<-sem
					wg.Done()
				}()
				if onStart != nil {
					onStart(job)
				}
				err := job.Run(ctx, onProgress)
				if onDone != nil {
					onDone(job, err)
				}
			}(j)
		}
		wg.Wait()
	}()
}

// CountingReader wraps io.Reader para acompanhar bytes lidos.
type CountingReader struct {
	R     io.Reader
	N     *atomic.Int64
	Total int64
}

func (c *CountingReader) Read(p []byte) (int, error) {
	n, err := c.R.Read(p)
	if n > 0 && c.N != nil {
		c.N.Add(int64(n))
	}
	return n, err
}

// CountingWriter wraps io.Writer.
type CountingWriter struct {
	W     io.Writer
	N     *atomic.Int64
	Total int64
}

func (c *CountingWriter) Write(p []byte) (int, error) {
	n, err := c.W.Write(p)
	if n > 0 && c.N != nil {
		c.N.Add(int64(n))
	}
	return n, err
}

// FormatBytes legível curto.
func FormatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	const u = 1024
	switch {
	case n < u*u:
		return fmt.Sprintf("%.1f KiB", f/u)
	case n < u*u*u:
		return fmt.Sprintf("%.1f MiB", f/(u*u))
	default:
		return fmt.Sprintf("%.1f GiB", f/(u*u*u))
	}
}

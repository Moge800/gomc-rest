package main

import (
	"math"
	"sync"
	"sync/atomic"
)

const recentWindow = 100

type plcMetrics struct {
	requests   atomic.Int64 // total PLC operations attempted
	reconnects atomic.Int64 // total reconnect attempts
	timeouts   atomic.Int64 // context.DeadlineExceeded while waiting for PLC op
	plcErrors  atomic.Int64 // total PLC errors (connection + protocol)
	totalNs    atomic.Int64 // cumulative nanoseconds for avg latency

	mu     sync.Mutex
	ring   [recentWindow]int64 // ring buffer of recent latencies in nanoseconds
	ringN  int                 // total writes (used to compute fill level)
	ringAt int                 // next write position
}

func (m *plcMetrics) recordLatency(ns int64) {
	m.totalNs.Add(ns)
	m.mu.Lock()
	m.ring[m.ringAt] = ns
	m.ringAt = (m.ringAt + 1) % recentWindow
	m.ringN++
	m.mu.Unlock()
}

// clientMetrics tracks HTTP-client-visible request metrics.
type clientMetrics struct {
	requests  atomic.Int64
	busyCount atomic.Int64
	totalNs   atomic.Int64

	mu     sync.Mutex
	ring   [recentWindow]int64
	ringN  int
	ringAt int
}

func (m *clientMetrics) recordLatency(ns int64) {
	m.totalNs.Add(ns)
	m.mu.Lock()
	m.ring[m.ringAt] = ns
	m.ringAt = (m.ringAt + 1) % recentWindow
	m.ringN++
	m.mu.Unlock()
}

func (m *clientMetrics) recentAvgMs() float64 {
	m.mu.Lock()
	n := m.ringN
	ring := m.ring
	m.mu.Unlock()

	if n == 0 {
		return 0
	}
	count := n
	if count > recentWindow {
		count = recentWindow
	}
	var sum int64
	for i := 0; i < count; i++ {
		sum += ring[i]
	}
	return math.Round(float64(sum)/float64(count)/1e6*100) / 100
}

func (m *plcMetrics) recentAvgMs() float64 {
	m.mu.Lock()
	n := m.ringN
	ring := m.ring
	m.mu.Unlock()

	if n == 0 {
		return 0
	}
	count := n
	if count > recentWindow {
		count = recentWindow
	}
	var sum int64
	for i := 0; i < count; i++ {
		sum += ring[i]
	}
	return math.Round(float64(sum)/float64(count)/1e6*100) / 100
}

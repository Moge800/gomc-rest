package main

import (
	"context"
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

var errQueueClosed = errors.New("PLC communication queue is closed")

type busyErr struct{}

func (e *busyErr) Error() string { return "PLC communication queue is full" }

type workJob struct {
	ctx     context.Context
	execute func() (any, error)
	result  chan workResult
}

type workResult struct {
	value any
	err   error
}

type WorkQueue struct {
	mu         sync.RWMutex
	jobs       chan workJob
	done       chan struct{}
	workerDone chan struct{}
	closed     bool
	stopOnce   sync.Once
}

func NewWorkQueue(size int) *WorkQueue {
	q := &WorkQueue{
		jobs:       make(chan workJob, size),
		done:       make(chan struct{}),
		workerDone: make(chan struct{}),
	}
	go q.run()
	return q
}

func (q *WorkQueue) Do(ctx context.Context, execute func() (any, error)) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, errQueueClosed
	}

	job := workJob{
		ctx:     ctx,
		execute: execute,
		result:  make(chan workResult, 1),
	}

	select {
	case q.jobs <- job:
		q.mu.RUnlock()
	case <-ctx.Done():
		q.mu.RUnlock()
		return nil, ctx.Err()
	default:
		q.mu.RUnlock()
		return nil, &busyErr{}
	}

	select {
	case result := <-job.result:
		return result.value, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *WorkQueue) isClosed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

func (q *WorkQueue) Shutdown(ctx context.Context) error {
	q.stopOnce.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.done)
		q.mu.Unlock()
	})

	select {
	case <-q.workerDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *WorkQueue) run() {
	defer close(q.workerDone)
	for {
		select {
		case <-q.done:
			q.rejectPendingJobs()
			return
		case job := <-q.jobs:
			if q.isClosed() {
				q.finishJob(job, workResult{err: errQueueClosed})
				q.rejectPendingJobs()
				return
			}
			q.runJob(job)
		}
	}
}

func (q *WorkQueue) rejectPendingJobs() {
	for {
		select {
		case job := <-q.jobs:
			q.finishJob(job, workResult{err: errQueueClosed})
		default:
			return
		}
	}
}

func (q *WorkQueue) runJob(job workJob) {
	result := workResult{}
	if err := job.ctx.Err(); err != nil {
		result.err = err
	} else {
		result.value, result.err = job.execute()
	}
	q.finishJob(job, result)
}

func (q *WorkQueue) finishJob(job workJob, result workResult) {
	select {
	case job.result <- result:
	case <-job.ctx.Done():
	}
}

type PLCQueue struct {
	plc    *PLCClient
	work   *WorkQueue
	client clientMetrics
}

func newPLCQueue(plc *PLCClient, size int) *PLCQueue {
	return &PLCQueue{plc: plc, work: NewWorkQueue(size)}
}

// exec wraps work.Do with client-visible metric tracking.
// busy rejections increment busyCount but are excluded from latency averages.
func (q *PLCQueue) exec(ctx context.Context, fn func() (any, error)) (any, error) {
	q.client.requests.Add(1)
	start := time.Now()
	value, err := q.work.Do(ctx, fn)
	var busyE *busyErr
	if errors.As(err, &busyE) {
		q.client.busyCount.Add(1)
	} else {
		q.client.recordLatency(time.Since(start).Nanoseconds())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		q.plc.metrics.timeouts.Add(1)
	}
	return value, err
}

func (q *PLCQueue) InitialConnect() {
	_, _ = q.work.Do(context.Background(), func() (any, error) {
		q.plc.initialConnect()
		return nil, nil
	})
}

func (q *PLCQueue) IsConnected() bool {
	return q.plc.isConnectedSafe()
}

func (q *PLCQueue) Shutdown(ctx context.Context) error {
	return q.work.Shutdown(ctx)
}

func (q *PLCQueue) Close() {
	q.plc.close()
}

func (q *PLCQueue) ReadWords(ctx context.Context, device string, start, count int) ([]uint16, error) {
	t := time.Now()
	value, err := q.exec(ctx, func() (any, error) {
		var words []uint16
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			var readErr error
			words, readErr = c.ReadWords(device, start, count)
			writePLCLatency(ctx, time.Since(plcStart))
			return readErr
		})
		return words, doErr
	})
	logPLCOp(device+strconv.Itoa(start), time.Since(t), err)
	if err != nil {
		return nil, err
	}
	return value.([]uint16), nil
}

func (q *PLCQueue) ReadBits(ctx context.Context, device string, start, count int) ([]bool, error) {
	t := time.Now()
	value, err := q.exec(ctx, func() (any, error) {
		var bits []bool
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			var readErr error
			bits, readErr = c.ReadBits(device, start, count)
			writePLCLatency(ctx, time.Since(plcStart))
			return readErr
		})
		return bits, doErr
	})
	logPLCOp(device+strconv.Itoa(start), time.Since(t), err)
	if err != nil {
		return nil, err
	}
	return value.([]bool), nil
}

func (q *PLCQueue) WriteWords(ctx context.Context, device string, start int, values []uint16) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			writeErr := c.WriteWords(device, start, values)
			writePLCLatency(ctx, time.Since(plcStart))
			return writeErr
		})
		return nil, doErr
	})
	logPLCOp(device+strconv.Itoa(start), time.Since(t), err)
	return err
}

func (q *PLCQueue) WriteBits(ctx context.Context, device string, start int, values []bool) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			writeErr := c.WriteBits(device, start, values)
			writePLCLatency(ctx, time.Since(plcStart))
			return writeErr
		})
		return nil, doErr
	})
	logPLCOp(device+strconv.Itoa(start), time.Since(t), err)
	return err
}

func (q *PLCQueue) ReadWordBits(ctx context.Context, device string, addr, bit, count int) ([]bool, error) {
	t := time.Now()
	value, err := q.exec(ctx, func() (any, error) {
		var result []bool
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			words, readErr := c.ReadWords(device, addr, 1)
			writePLCLatency(ctx, time.Since(plcStart))
			if readErr != nil {
				return readErr
			}
			result = make([]bool, count)
			for i := range count {
				result[i] = (words[0]>>uint(bit+i))&1 == 1
			}
			return nil
		})
		return result, doErr
	})
	logPLCOp(device+strconv.Itoa(addr)+"."+strconv.Itoa(bit), time.Since(t), err)
	if err != nil {
		return nil, err
	}
	return value.([]bool), nil
}

func (q *PLCQueue) WriteWordBits(ctx context.Context, device string, addr, bit int, values []bool) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			defer func() { writePLCLatency(ctx, time.Since(plcStart)) }()
			words, err := c.ReadWords(device, addr, 1)
			if err != nil {
				return err
			}
			for i, v := range values {
				if v {
					words[0] |= 1 << uint(bit+i)
				} else {
					words[0] &^= 1 << uint(bit+i)
				}
			}
			return c.WriteWords(device, addr, words)
		})
		return nil, doErr
	})
	logPLCOp(device+strconv.Itoa(addr)+"."+strconv.Itoa(bit), time.Since(t), err)
	return err
}

func (q *PLCQueue) RemoteRun(ctx context.Context, clear int, force bool) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			runErr := c.RemoteRun(clear, force)
			writePLCLatency(ctx, time.Since(plcStart))
			return runErr
		})
		return nil, doErr
	})
	logPLCOp("remote_run", time.Since(t), err)
	return err
}

func (q *PLCQueue) RemoteStop(ctx context.Context) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			stopErr := c.RemoteStop()
			writePLCLatency(ctx, time.Since(plcStart))
			return stopErr
		})
		return nil, doErr
	})
	logPLCOp("remote_stop", time.Since(t), err)
	return err
}

func (q *PLCQueue) RemotePause(ctx context.Context, force bool) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			pauseErr := c.RemotePause(force)
			writePLCLatency(ctx, time.Since(plcStart))
			return pauseErr
		})
		return nil, doErr
	})
	logPLCOp("remote_pause", time.Since(t), err)
	return err
}

func (q *PLCQueue) RemoteLatchClear(ctx context.Context) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			clearErr := c.RemoteLatchClear()
			writePLCLatency(ctx, time.Since(plcStart))
			return clearErr
		})
		return nil, doErr
	})
	logPLCOp("remote_latch_clear", time.Since(t), err)
	return err
}

func (q *PLCQueue) RemoteReset(ctx context.Context) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		return nil, q.plc.doReset(ctx)
	})
	logPLCOp("remote_reset", time.Since(t), err)
	return err
}

const maxRandomCount = 255

// maxRandomBitReads caps native bit devices in a single /random-read, since each
// one costs a separate ReadBits round-trip (MC random read has no bit unit).
const maxRandomBitReads = 16

// RandomRead reads word and dword devices in a single MC random read (0x0403).
// Native bit devices (bits) have no random-read command, so each is read with an
// individual ReadBits (0x0401); all reads run within one serialized job.
func (q *PLCQueue) RandomRead(ctx context.Context, words, dwords, bits []mc.DeviceAddr) ([]uint16, []uint32, []bool, error) {
	t := time.Now()
	type result struct {
		words  []uint16
		dwords []uint32
		bits   []bool
	}
	value, err := q.exec(ctx, func() (any, error) {
		var r result
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			defer func() { writePLCLatency(ctx, time.Since(plcStart)) }()
			var readErr error
			r.words, r.dwords, readErr = c.RandomRead(words, dwords)
			if readErr != nil {
				return readErr
			}
			r.bits = make([]bool, len(bits))
			for i, b := range bits {
				vals, err := c.ReadBits(b.Device, b.Addr, 1)
				if err != nil {
					return err
				}
				r.bits[i] = len(vals) > 0 && vals[0]
			}
			return nil
		})
		return r, doErr
	})
	logPLCOp("random_read", time.Since(t), err)
	if err != nil {
		return nil, nil, nil, err
	}
	r := value.(result)
	return r.words, r.dwords, r.bits, nil
}

// WordBitWrite is a single bit-access write to a word device (e.g. D100.1),
// resolved into a read-modify-write within RandomWrite.
type WordBitWrite struct {
	Device string
	Addr   int
	Bit    int
	Value  bool
}

func (q *PLCQueue) RandomWrite(ctx context.Context,
	words []mc.DeviceAddr, wordVals []uint16,
	dwords []mc.DeviceAddr, dwordVals []uint32,
	bits []mc.DeviceAddr, bitVals []bool,
	wordBits []WordBitWrite,
) error {
	t := time.Now()
	_, err := q.exec(ctx, func() (any, error) {
		doErr := q.plc.do(func(c plcConnection) error {
			plcStart := time.Now()
			defer func() { writePLCLatency(ctx, time.Since(plcStart)) }()
			if len(words)+len(dwords) > 0 {
				if err := c.RandomWrite(words, wordVals, dwords, dwordVals); err != nil {
					return err
				}
			}
			if len(bits) > 0 {
				if err := c.RandomWriteBits(bits, bitVals); err != nil {
					return err
				}
			}
			return writeWordBits(c, wordBits)
		})
		return nil, doErr
	})
	logPLCOp("random_write", time.Since(t), err)
	return err
}

// writeWordBits applies word-device bit writes via read-modify-write, grouping
// bits in the same word into a single read/write so they don't clobber each other.
func writeWordBits(c plcConnection, wordBits []WordBitWrite) error {
	type key struct {
		device string
		addr   int
	}
	groups := map[key][]WordBitWrite{}
	var order []key
	for _, wb := range wordBits {
		k := key{wb.Device, wb.Addr}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], wb)
	}
	for _, k := range order {
		words, err := c.ReadWords(k.device, k.addr, 1)
		if err != nil {
			return err
		}
		for _, wb := range groups[k] {
			if wb.Value {
				words[0] |= 1 << uint(wb.Bit)
			} else {
				words[0] &^= 1 << uint(wb.Bit)
			}
		}
		if err := c.WriteWords(k.device, k.addr, words); err != nil {
			return err
		}
	}
	return nil
}

func (q *PLCQueue) Metrics() map[string]any {
	m := &q.plc.metrics
	reqs := m.requests.Load()
	var avgMs float64
	if reqs > 0 {
		avgMs = math.Round(float64(m.totalNs.Load())/float64(reqs)/1e6*100) / 100
	}

	c := &q.client
	cBusy := c.busyCount.Load()
	cReqs := c.requests.Load()
	cNonBusy := cReqs - cBusy
	if cNonBusy < 0 {
		cNonBusy = 0
	}
	var cAvgMs float64
	if cNonBusy > 0 {
		cAvgMs = math.Round(float64(c.totalNs.Load())/float64(cNonBusy)/1e6*100) / 100
	}

	return map[string]any{
		// HTTP client layer
		"client_request_count":         cReqs,
		"busy_count":                   cBusy,
		"client_avg_latency_ms":        cAvgMs,
		"client_recent_avg_latency_ms": c.recentAvgMs(),
		// queue
		"queue_length": len(q.work.jobs),
		// PLC layer
		"request_count":         reqs,
		"reconnect_count":       m.reconnects.Load(),
		"timeout_count":         m.timeouts.Load(),
		"plc_error_count":       m.plcErrors.Load(),
		"avg_latency_ms":        avgMs,
		"recent_avg_latency_ms": m.recentAvgMs(),
	}
}

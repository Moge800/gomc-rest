package main

import (
	"context"
	"errors"
	"sync"
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
	jobs       chan workJob
	done       chan struct{}
	workerDone chan struct{}
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

	job := workJob{
		ctx:     ctx,
		execute: execute,
		result:  make(chan workResult, 1),
	}

	select {
	case q.jobs <- job:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-q.done:
		return nil, errQueueClosed
	default:
		return nil, &busyErr{}
	}

	select {
	case result := <-job.result:
		return result.value, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-q.done:
		return nil, errQueueClosed
	}
}

func (q *WorkQueue) Shutdown(ctx context.Context) error {
	q.stopOnce.Do(func() {
		close(q.done)
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
			return
		case job := <-q.jobs:
			q.runJob(job)
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

	select {
	case job.result <- result:
	case <-job.ctx.Done():
	case <-q.done:
	}
}

type PLCQueue struct {
	plc  *PLCClient
	work *WorkQueue
}

func newPLCQueue(plc *PLCClient, size int) *PLCQueue {
	return &PLCQueue{plc: plc, work: NewWorkQueue(size)}
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
	value, err := q.work.Do(ctx, func() (any, error) {
		var words []uint16
		err := q.plc.do(func(c plcConnection) error {
			var readErr error
			words, readErr = c.ReadWords(device, start, count)
			return readErr
		})
		return words, err
	})
	if err != nil {
		return nil, err
	}
	return value.([]uint16), nil
}

func (q *PLCQueue) ReadBits(ctx context.Context, device string, start, count int) ([]bool, error) {
	value, err := q.work.Do(ctx, func() (any, error) {
		var bits []bool
		err := q.plc.do(func(c plcConnection) error {
			var readErr error
			bits, readErr = c.ReadBits(device, start, count)
			return readErr
		})
		return bits, err
	})
	if err != nil {
		return nil, err
	}
	return value.([]bool), nil
}

func (q *PLCQueue) WriteWords(ctx context.Context, device string, start int, values []uint16) error {
	_, err := q.work.Do(ctx, func() (any, error) {
		return nil, q.plc.do(func(c plcConnection) error {
			return c.WriteWords(device, start, values)
		})
	})
	return err
}

func (q *PLCQueue) WriteBits(ctx context.Context, device string, start int, values []bool) error {
	_, err := q.work.Do(ctx, func() (any, error) {
		return nil, q.plc.do(func(c plcConnection) error {
			return c.WriteBits(device, start, values)
		})
	})
	return err
}

func (q *PLCQueue) RemoteRun(ctx context.Context, clear int, force bool) error {
	_, err := q.work.Do(ctx, func() (any, error) {
		return nil, q.plc.do(func(c plcConnection) error {
			return c.RemoteRun(clear, force)
		})
	})
	return err
}

func (q *PLCQueue) RemoteStop(ctx context.Context) error {
	_, err := q.work.Do(ctx, func() (any, error) {
		return nil, q.plc.do(func(c plcConnection) error {
			return c.RemoteStop()
		})
	})
	return err
}

func (q *PLCQueue) RemotePause(ctx context.Context, force bool) error {
	_, err := q.work.Do(ctx, func() (any, error) {
		return nil, q.plc.do(func(c plcConnection) error {
			return c.RemotePause(force)
		})
	})
	return err
}

func (q *PLCQueue) RemoteLatchClear(ctx context.Context) error {
	_, err := q.work.Do(ctx, func() (any, error) {
		return nil, q.plc.do(func(c plcConnection) error {
			return c.RemoteLatchClear()
		})
	})
	return err
}

func (q *PLCQueue) RemoteReset(ctx context.Context) error {
	_, err := q.work.Do(ctx, func() (any, error) {
		return nil, q.plc.doReset()
	})
	return err
}

package main

import "sync/atomic"

type plcMetrics struct {
	requests   atomic.Int64 // total PLC operations attempted
	reconnects atomic.Int64 // total reconnect attempts
	plcErrors  atomic.Int64 // total PLC errors (connection + protocol)
	totalNs    atomic.Int64 // cumulative nanoseconds for avg latency
}

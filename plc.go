package main

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

type plcConnection interface {
	ReadWords(device string, start, count int) ([]uint16, error)
	WriteWords(device string, start int, values []uint16) error
	ReadBits(device string, start, count int) ([]bool, error)
	WriteBits(device string, start int, values []bool) error
	RemoteRun(clearMode int, force bool) error
	RemoteStop() error
	RemotePause(force bool) error
	RemoteLatchClear() error
	RemoteReset() error
	Close() error
}

type plcDialer interface {
	plcConnection
	Connect() error
	SetTimeout(time.Duration)
}

type PLCClient struct {
	mu      sync.Mutex
	conn    plcConnection // nil = disconnected
	metrics plcMetrics

	host      string
	port      int
	mode      mc.Mode
	frame     PLCFrame
	transport PLCTransport
	timeout   time.Duration
}

func newPLCClient(host string, port int, mode mc.Mode) *PLCClient {
	return newConfiguredPLCClient(ServerConfig{
		Host:      host,
		Port:      port,
		Mode:      mode,
		Frame:     frame3E,
		Transport: transportTCP,
		Timeout:   5 * time.Second,
	})
}

func newConfiguredPLCClient(cfg ServerConfig) *PLCClient {
	return &PLCClient{
		host:      cfg.Host,
		port:      cfg.Port,
		mode:      cfg.Mode,
		frame:     cfg.Frame,
		transport: cfg.Transport,
		timeout:   cfg.Timeout,
	}
}

func (p *PLCClient) reconnect() error {
	p.metrics.reconnects.Add(1)
	c, err := p.newConnection()
	if err != nil {
		return err
	}
	c.SetTimeout(p.timeout)
	if err := c.Connect(); err != nil {
		return err
	}
	p.conn = c
	return nil
}

func (p *PLCClient) newConnection() (plcDialer, error) {
	switch p.frame {
	case frame3E:
		if p.transport == transportUDP {
			return mc.New3EClientUDP(p.host, p.port, p.mode)
		}
		return mc.New3EClient(p.host, p.port, p.mode)
	case frame4E:
		return mc.New4EClient(p.host, p.port, p.mode)
	default:
		return nil, fmt.Errorf("unsupported frame %q", p.frame)
	}
}

// initialConnect attempts connection at startup; logs failure but does not exit.
func (p *PLCClient) initialConnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.reconnect(); err != nil {
		slog.Warn("PLC initial connect failed, will retry on first request", "error", err)
	}
}

// isConnected reports current connection state. Caller must hold mu.
func (p *PLCClient) isConnected() bool {
	return p.conn != nil
}

func (p *PLCClient) isConnectedSafe() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isConnected()
}

// do runs fn under the mutex. On MCProtocolConnectionError the connection is
// cleared so the next call triggers a reconnect.
func (p *PLCClient) do(fn func(plcConnection) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		if err := p.reconnect(); err != nil {
			p.metrics.plcErrors.Add(1)
			return &connErrWrap{err}
		}
	}

	p.metrics.requests.Add(1)
	start := time.Now()
	err := fn(p.conn)
	p.metrics.recordLatency(time.Since(start).Nanoseconds())

	if err == nil {
		return nil
	}

	p.metrics.plcErrors.Add(1)
	var connErr *mc.MCProtocolConnectionError
	if errors.As(err, &connErr) {
		p.conn = nil
		return &connErrWrap{err}
	}
	return err
}

func (p *PLCClient) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// doReset is like do but always clears the connection afterwards (RemoteReset
// closes the TCP connection on the PLC side).
func (p *PLCClient) doReset() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		if err := p.reconnect(); err != nil {
			p.metrics.plcErrors.Add(1)
			return &connErrWrap{err}
		}
	}

	p.metrics.requests.Add(1)
	start := time.Now()
	err := p.conn.RemoteReset()
	p.metrics.recordLatency(time.Since(start).Nanoseconds())
	p.conn = nil // PLC closes connection on reset regardless of error

	var connErr *mc.MCProtocolConnectionError
	if errors.As(err, &connErr) {
		p.metrics.plcErrors.Add(1)
		return &connErrWrap{err}
	}
	if err != nil {
		p.metrics.plcErrors.Add(1)
		return err
	}
	return nil
}

// connErrWrap marks an error as a connection-level error for HTTP 503.
type connErrWrap struct{ cause error }

func (e *connErrWrap) Error() string { return e.cause.Error() }
func (e *connErrWrap) Unwrap() error { return e.cause }

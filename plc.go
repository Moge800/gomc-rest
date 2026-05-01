package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

type PLCClient struct {
	mu   sync.Mutex
	conn *mc.Client3E // nil = disconnected

	host string
	port int
	mode mc.Mode
}

func newPLCClient(host string, port int, mode mc.Mode) *PLCClient {
	return &PLCClient{host: host, port: port, mode: mode}
}

func (p *PLCClient) reconnect() error {
	c, err := mc.New3EClient(p.host, p.port, p.mode)
	if err != nil {
		return err
	}
	c.SetTimeout(5 * time.Second)
	if err := c.Connect(); err != nil {
		return err
	}
	p.conn = c
	return nil
}

// initialConnect attempts connection at startup; logs failure but does not exit.
func (p *PLCClient) initialConnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.reconnect(); err != nil {
		fmt.Printf("PLC initial connect failed: %v (will retry on first request)\n", err)
	}
}

// isConnected reports current connection state. Caller must hold mu.
func (p *PLCClient) isConnected() bool {
	return p.conn != nil
}

// do runs fn under the mutex. On MCProtocolConnectionError the connection is
// cleared so the next call triggers a reconnect.
func (p *PLCClient) do(fn func(*mc.Client3E) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		if err := p.reconnect(); err != nil {
			return &connErrWrap{err}
		}
	}

	err := fn(p.conn)
	if err == nil {
		return nil
	}

	var connErr *mc.MCProtocolConnectionError
	if errors.As(err, &connErr) {
		p.conn = nil
		return &connErrWrap{err}
	}
	return err
}

// doReset is like do but always clears the connection afterwards (RemoteReset
// closes the TCP connection on the PLC side).
func (p *PLCClient) doReset() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		if err := p.reconnect(); err != nil {
			return &connErrWrap{err}
		}
	}

	err := p.conn.RemoteReset()
	p.conn = nil // PLC closes connection on reset regardless of error
	var connErr *mc.MCProtocolConnectionError
	if err != nil && !errors.As(err, &connErr) {
		return err
	}
	return nil
}

// connErrWrap marks an error as a connection-level error for HTTP 503.
type connErrWrap struct{ cause error }

func (e *connErrWrap) Error() string { return e.cause.Error() }
func (e *connErrWrap) Unwrap() error { return e.cause }
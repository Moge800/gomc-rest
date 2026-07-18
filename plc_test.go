package main

import (
	"testing"

	mc "github.com/moge800/gomcprotocol"
)

// connLossMock embeds mockConn (all stubs) and injects a connection-level
// error on ReadWords while recording whether Close was called.
type connLossMock struct {
	*mockConn
	closed bool
}

func (m *connLossMock) ReadWords(string, int, int) ([]uint16, error) {
	return nil, &mc.MCProtocolConnectionError{}
}

func (m *connLossMock) Close() error {
	m.closed = true
	return nil
}

// TestDoClosesConnOnConnectionError verifies that a connection-level failure
// closes the dead socket (so the OS can send FIN/RST and the PLC can free its
// session) before clearing p.conn for the next reconnect.
func TestDoClosesConnOnConnectionError(t *testing.T) {
	m := &connLossMock{mockConn: &mockConn{words: map[string]uint16{}}}
	plc := newPLCClient("127.0.0.1", 5007, mc.ModeBinary)
	plc.conn = m

	err := plc.do(func(c plcConnection) error {
		_, e := c.ReadWords("D", 0, 1)
		return e
	})

	if err == nil {
		t.Fatal("expected a connection error, got nil")
	}
	if !m.closed {
		t.Error("Close() was not called on the dead connection")
	}
	if plc.conn != nil {
		t.Error("p.conn should be nil after a connection error")
	}
}

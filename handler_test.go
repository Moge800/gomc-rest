package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mc "github.com/moge800/gomcprotocol"
)

func TestHandleReadRequiresGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/read?addr=D100", nil)
	rec := httptest.NewRecorder()

	handleRead(newPLCClient("127.0.0.1", 5007, mc.ModeBinary))(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
	}
}

func TestHandleReadRejectsTooLargeCount(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100&count=1025", nil)
	rec := httptest.NewRecorder()

	handleRead(newPLCClient("127.0.0.1", 5007, mc.ModeBinary))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteRejectsTooManyWordValues(t *testing.T) {
	body := `{"values":[` + strings.TrimRight(strings.Repeat("1,", maxWriteValues+1), ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newPLCClient("127.0.0.1", 5007, mc.ModeBinary))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteRejectsTooManyBitValues(t *testing.T) {
	body := `{"values":[` + strings.TrimRight(strings.Repeat("true,", maxWriteValues+1), ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=M0", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newPLCClient("127.0.0.1", 5007, mc.ModeBinary))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteRejectsTooLargeBody(t *testing.T) {
	prefix := `{"values":"`
	suffix := `"}`
	body := prefix + strings.Repeat("a", maxWriteBody+1-len(prefix)-len(suffix)) + suffix
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newPLCClient("127.0.0.1", 5007, mc.ModeBinary))(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

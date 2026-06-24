package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

func newTestPLCQueue(t *testing.T) *PLCQueue {
	t.Helper()

	plcQueue := newPLCQueue(newPLCClient("127.0.0.1", 5007, mc.ModeBinary), 32)
	t.Cleanup(func() {
		_ = plcQueue.Shutdown(context.Background())
		plcQueue.Close()
	})
	return plcQueue
}

func emptyEnv(string) string { return "" }

func TestParseConfigDefaultsAndNewFlags(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-frame", "4e",
		"-transport", "tcp",
		"-queue-size", "7",
		"-timeout", "250ms",
	}, emptyEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Host != "192.168.0.1" || cfg.Port != 5007 {
		t.Fatalf("PLC default = %s:%d, want 192.168.0.1:5007", cfg.Host, cfg.Port)
	}
	if cfg.Frame != frame4E {
		t.Fatalf("frame = %q, want %q", cfg.Frame, frame4E)
	}
	if cfg.Transport != transportTCP {
		t.Fatalf("transport = %q, want %q", cfg.Transport, transportTCP)
	}
	if cfg.QueueSize != 7 {
		t.Fatalf("queue size = %d, want 7", cfg.QueueSize)
	}
	if cfg.Timeout.String() != "250ms" {
		t.Fatalf("timeout = %s, want 250ms", cfg.Timeout)
	}
}

func TestNormalizeListen(t *testing.T) {
	cases := []struct{ in, want string }{
		{"8080", ":8080"},
		{":8080", ":8080"},
		{"127.0.0.1:8080", "127.0.0.1:8080"},
		{"0.0.0.0:9000", "0.0.0.0:9000"},
	}
	for _, tc := range cases {
		if got := normalizeListen(tc.in); got != tc.want {
			t.Errorf("normalizeListen(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseConfigListenNormalized(t *testing.T) {
	cfg, err := parseConfig([]string{"-listen", "9090"}, emptyEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Listen != ":9090" {
		t.Fatalf("Listen = %q, want %q", cfg.Listen, ":9090")
	}
}

func TestParseConfigRejectsInvalidQueueSize(t *testing.T) {
	_, err := parseConfig([]string{"-queue-size", "0"}, emptyEnv, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid queue-size") {
		t.Fatalf("err = %v, want invalid queue-size", err)
	}
}

func TestParseConfigRejectsInvalidTimeout(t *testing.T) {
	_, err := parseConfig([]string{"-timeout", "0s"}, emptyEnv, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid timeout") {
		t.Fatalf("err = %v, want invalid timeout", err)
	}
}

func TestParseConfigRejects4EUDP(t *testing.T) {
	_, err := parseConfig([]string{"-frame", "4e", "-transport", "udp"}, emptyEnv, nil)
	if err == nil || !strings.Contains(err.Error(), "frame 4e supports tcp only") {
		t.Fatalf("err = %v, want 4e udp rejection", err)
	}
}

func TestParseConfigLogFileFlag(t *testing.T) {
	path := t.TempDir() + "/test.log"
	cfg, err := parseConfig([]string{"-log-file", path}, emptyEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.LogFile != path {
		t.Fatalf("LogFile = %q, want %q", cfg.LogFile, path)
	}
}

func TestParseConfigLogFileEnv(t *testing.T) {
	lookupEnv := func(key string) string {
		if key == "GOMCR_LOG_FILE" {
			return "/var/log/gomc.log"
		}
		return ""
	}
	cfg, err := parseConfig(nil, lookupEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.LogFile != "/var/log/gomc.log" {
		t.Fatalf("LogFile = %q, want /var/log/gomc.log", cfg.LogFile)
	}
}

func TestParseConfigLogFileFlagOverridesEnv(t *testing.T) {
	flagPath := t.TempDir() + "/flag.log"
	lookupEnv := func(key string) string {
		if key == "GOMCR_LOG_FILE" {
			return t.TempDir() + "/env.log"
		}
		return ""
	}
	cfg, err := parseConfig([]string{"-log-file", flagPath}, lookupEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.LogFile != flagPath {
		t.Fatalf("LogFile = %q, want %q", cfg.LogFile, flagPath)
	}
}

func TestParseConfigRejectsInvalidReadOnlyEnv(t *testing.T) {
	lookupEnv := func(key string) string {
		if key == "GOMCR_READONLY" {
			return "maybe"
		}
		return ""
	}

	_, err := parseConfig(nil, lookupEnv, nil)
	if err == nil || !strings.Contains(err.Error(), `invalid GOMCR_READONLY "maybe"`) {
		t.Fatalf("err = %v, want invalid GOMCR_READONLY", err)
	}
}

func TestParseConfigReadOnlyFlagOverridesInvalidEnv(t *testing.T) {
	lookupEnv := func(key string) string {
		if key == "GOMCR_READONLY" {
			return "maybe"
		}
		return ""
	}

	cfg, err := parseConfig([]string{"-readonly"}, lookupEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.ReadOnly {
		t.Fatal("ReadOnly = false, want true")
	}
}

func assertReadOnlyError(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/json; charset=utf-8")
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["status"] != float64(http.StatusForbidden) {
		t.Fatalf("status body = %v, want %d", body["status"], http.StatusForbidden)
	}
	if body["code"] != "forbidden" {
		t.Fatalf("code = %q, want %q", body["code"], "forbidden")
	}
	if body["error"] != "operation not allowed in read-only mode" {
		t.Fatalf("error = %q, want %q", body["error"], "operation not allowed in read-only mode")
	}
}

func assertRequiresGET(t *testing.T, handler http.Handler) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	const wantAllow = "GET, HEAD"
	if got := rec.Header().Get("Allow"); got != wantAllow {
		t.Fatalf("Allow = %q, want %q", got, wantAllow)
	}
}

func TestHandleReadRequiresGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/read?addr=D100", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow = %q, want %q", got, "GET, HEAD")
	}
}

func TestInfoEndpointsRequireGET(t *testing.T) {
	q := newTestPLCQueue(t)
	cfg := ServerConfig{Host: "192.168.0.1", Port: 5007, Frame: frame3E, Transport: transportTCP, ModeString: "binary", Listen: "127.0.0.1:8080"}
	cases := []struct {
		name    string
		handler http.Handler
	}{
		{"version", handleVersion()},
		{"info", handleInfo(cfg)},
		{"health", handleHealth(q)},
		{"metrics", handleMetrics(q)},
		{"openapi", handleOpenAPI(openAPISpec)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRequiresGET(t, tc.handler)
		})
	}
}

func TestHandleInfo(t *testing.T) {
	cfg := ServerConfig{
		Host:         "192.168.0.1",
		Port:         5007,
		Frame:        frame3E,
		Transport:    transportTCP,
		ModeString:   "binary",
		Listen:       "127.0.0.1:8080",
		ReadOnly:     true,
		EnableRemote: false,
	}
	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	rec := httptest.NewRecorder()
	handleInfo(cfg)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"version", "gomcprotocol_version", "host", "port", "frame", "transport", "mode", "listen_addrs", "readonly", "enable_remote"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}
	if body["host"] != "192.168.0.1" {
		t.Errorf("host = %v, want 192.168.0.1", body["host"])
	}
	if body["readonly"] != true {
		t.Errorf("readonly = %v, want true", body["readonly"])
	}
}

func TestListenAddrs(t *testing.T) {
	cases := []struct {
		listen  string
		wantLen int    // -1 = any (including nil); -2 = nil only
		wantVal string // non-empty = check first element
	}{
		{"192.168.1.10:8080", 1, "192.168.1.10:8080"},
		{"127.0.0.1:8080", 1, "127.0.0.1:8080"},
		{":8080", -1, ""},      // may be nil in environments with no non-loopback IPv4
		{"[::]:8080", -1, ""},  // same
		{"not-valid", -2, ""},
	}
	for _, tc := range cases {
		addrs := listenAddrs(tc.listen)
		switch tc.wantLen {
		case -2:
			if addrs != nil {
				t.Errorf("listenAddrs(%q) = %v, want nil", tc.listen, addrs)
			}
		case -1:
			// accept any result including nil
		default:
			if len(addrs) != tc.wantLen {
				t.Errorf("listenAddrs(%q) len = %d, want %d", tc.listen, len(addrs), tc.wantLen)
			}
			if tc.wantVal != "" && len(addrs) > 0 && addrs[0] != tc.wantVal {
				t.Errorf("listenAddrs(%q)[0] = %q, want %q", tc.listen, addrs[0], tc.wantVal)
			}
		}
	}
}

func TestHandleReadRejectsTooLargeCount(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100&count=1025", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteRejectsTooManyWordValues(t *testing.T) {
	body := `{"values":[` + strings.TrimRight(strings.Repeat("1,", maxWriteValues+1), ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteRejectsTooManyBitValues(t *testing.T) {
	body := `{"values":[` + strings.TrimRight(strings.Repeat("true,", maxWriteValues+1), ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=M0", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

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

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleReadDwordRejectsTooLargeCount(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100&dword=true&count=513", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleReadDwordRejectsBitDevice(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/read?addr=M0&dword=true", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteDwordRejectsBitDevice(t *testing.T) {
	body := `{"values":[100000]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=M0&dword=true", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteDwordRejectsTooManyValues(t *testing.T) {
	body := `{"values":[` + strings.TrimRight(strings.Repeat("100000,", maxWriteValues/2+1), ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100&dword=true", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteDwordRejectsOutOfRange(t *testing.T) {
	body := `{"values":[-1]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100&dword=true", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteDwordRejectsAboveMaxUint32(t *testing.T) {
	body := `{"values":[4294967296]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100&dword=true", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleReadSintRejectsBitDevice(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/read?addr=M0&sint=true", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteSintRejectsBitDevice(t *testing.T) {
	body := `{"values":[-1]}`
	req := httptest.NewRequest(http.MethodPost, "/write?addr=M0&sint=true", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteSintWordRejectsOutOfRange(t *testing.T) {
	for _, body := range []string{`{"values":[-32769]}`, `{"values":[32768]}`} {
		req := httptest.NewRequest(http.MethodPost, "/write?addr=D100&sint=true", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handleWrite(newTestPLCQueue(t), false)(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleWriteSintDwordRejectsOutOfRange(t *testing.T) {
	for _, body := range []string{`{"values":[-2147483649]}`, `{"values":[2147483648]}`} {
		req := httptest.NewRequest(http.MethodPost, "/write?addr=D100&dword=true&sint=true", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handleWrite(newTestPLCQueue(t), false)(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleReadBoolParams(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{"D100": 0, "D101": 0})
	cases := []struct {
		query string
		want  int
	}{
		{"/read?addr=D100&dword=True", http.StatusOK},
		{"/read?addr=D100&dword=TRUE", http.StatusOK},
		{"/read?addr=D100&dword=true", http.StatusOK},
		{"/read?addr=D100&dword=false", http.StatusOK},
		{"/read?addr=D100&dword=1", http.StatusBadRequest},
		{"/read?addr=D100&dword=0", http.StatusBadRequest},
		{"/read?addr=D100&dword=abc", http.StatusBadRequest},
		{"/read?addr=D100&sint=True", http.StatusOK},
		{"/read?addr=D100&sint=1", http.StatusBadRequest},
		{"/read?addr=D100&foo=bar", http.StatusBadRequest},
		{"/read?addr=D100&dword=true&extra=1", http.StatusBadRequest},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.query, nil)
		rec := httptest.NewRecorder()
		handleRead(q)(rec, req)
		if rec.Code != tc.want {
			t.Errorf("query=%s status=%d, want=%d", tc.query, rec.Code, tc.want)
		}
	}
}

// TestHandleReadDwordCaseInsensitive verifies that dword=True/TRUE actually
// enables double-word mode — count=513 exceeds the dword limit (512) but is
// within the word limit (1024), so it is rejected only when dword is true.
func TestHandleReadDwordCaseInsensitive(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{})
	for _, param := range []string{"true", "True", "TRUE"} {
		req := httptest.NewRequest(http.MethodGet, "/read?addr=D100&dword="+param+"&count=513", nil)
		rec := httptest.NewRecorder()
		handleRead(q)(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("dword=%s count=513: status=%d want 400 (dword count limit is 512)", param, rec.Code)
		}
	}
	// dword=false: count=513 is within the word limit (1024) → 200
	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100&dword=false&count=513", nil)
	rec := httptest.NewRecorder()
	handleRead(q)(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("dword=false count=513: status=%d want 200 (word limit is 1024)", rec.Code)
	}
}

func TestHandleWriteBoolParams(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{})
	cases := []struct {
		query string
		body  string
		want  int
	}{
		{"/write?addr=D100&dword=True", `{"values":[0]}`, http.StatusOK},
		{"/write?addr=D100&dword=TRUE", `{"values":[0]}`, http.StatusOK},
		{"/write?addr=D100&dword=true", `{"values":[0]}`, http.StatusOK},
		{"/write?addr=D100&dword=false", `{"values":[0]}`, http.StatusOK},
		{"/write?addr=D100&dword=1", `{"values":[0]}`, http.StatusBadRequest},
		{"/write?addr=D100&dword=0", `{"values":[0]}`, http.StatusBadRequest},
		{"/write?addr=D100&dword=abc", `{"values":[0]}`, http.StatusBadRequest},
		{"/write?addr=D100&sint=True", `{"values":[0]}`, http.StatusOK},
		{"/write?addr=D100&sint=1", `{"values":[0]}`, http.StatusBadRequest},
		{"/write?addr=D100&foo=bar", `{"values":[0]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.query, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		handleWrite(q, false)(rec, req)
		if rec.Code != tc.want {
			t.Errorf("query=%s status=%d, want=%d", tc.query, rec.Code, tc.want)
		}
	}
}

// TestHandleWriteDwordCaseInsensitive verifies that dword=True/TRUE actually
// enables double-word mode — 70000 (>uint16 max) succeeds only when dword is true.
func TestHandleWriteDwordCaseInsensitive(t *testing.T) {
	for _, param := range []string{"true", "True", "TRUE"} {
		q := newMockPLCQueue(t, map[string]uint16{})
		req := httptest.NewRequest(http.MethodPost, "/write?addr=D100&dword="+param,
			strings.NewReader(`{"values":[70000]}`))
		rec := httptest.NewRecorder()
		handleWrite(q, false)(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("dword=%s status=%d want 200 (70000 must be accepted in dword mode)", param, rec.Code)
		}
	}
	// dword=false: 70000 > uint16 max → JSON unmarshal into []uint16 fails → 400
	q := newMockPLCQueue(t, map[string]uint16{})
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100&dword=false",
		strings.NewReader(`{"values":[70000]}`))
	rec := httptest.NewRecorder()
	handleWrite(q, false)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("dword=false with 70000: status=%d want 400", rec.Code)
	}
}

func TestHandleReadBitAccessRejectsBitDevice(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/read?addr=M0.0", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleReadBitAccessRejectsWordBoundaryExceeded(t *testing.T) {
	// bit=0xF(15) + count=2 = 17 > 16: must reject
	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100.F&count=2", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleReadBitAccessRejectsDword(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100.0&dword=true", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteBitAccessRejectsBitDevice(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/write?addr=M0.0", strings.NewReader(`{"values":[true]}`))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteBitAccessRejectsDword(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100.0&dword=true", strings.NewReader(`{"values":[true]}`))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWriteBitAccessRejectsWordBoundaryExceeded(t *testing.T) {
	// bit=0xF(15) + 2 values = 17 > 16: must reject
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100.F", strings.NewReader(`{"values":[true,false]}`))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), false)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// mockConn implements plcConnection with a simple word register map.
type mockConn struct {
	words map[string]uint16
}

func (m *mockConn) ReadWords(device string, start, count int) ([]uint16, error) {
	key := device + strconv.Itoa(start)
	out := make([]uint16, count)
	if v, ok := m.words[key]; ok {
		out[0] = v
	}
	return out, nil
}
func (m *mockConn) WriteWords(device string, start int, values []uint16) error {
	m.words[device+strconv.Itoa(start)] = values[0]
	return nil
}
func (m *mockConn) ReadBits(_ string, _ int, _ int) ([]bool, error)  { return nil, nil }
func (m *mockConn) WriteBits(_ string, _ int, _ []bool) error         { return nil }
func (m *mockConn) RemoteRun(_ int, _ bool) error                     { return nil }
func (m *mockConn) RemoteStop() error                                  { return nil }
func (m *mockConn) RemotePause(_ bool) error                           { return nil }
func (m *mockConn) RemoteLatchClear() error                            { return nil }
func (m *mockConn) RemoteReset() error                                 { return nil }
func (m *mockConn) Close() error                                       { return nil }
func (m *mockConn) RandomRead(words, dwords []mc.DeviceAddr) ([]uint16, []uint32, error) {
	wVals := make([]uint16, len(words))
	for i, da := range words {
		wVals[i] = m.words[da.Device+strconv.Itoa(da.Addr)]
	}
	return wVals, make([]uint32, len(dwords)), nil
}
func (m *mockConn) RandomWrite(words []mc.DeviceAddr, wordVals []uint16, _ []mc.DeviceAddr, _ []uint32) error {
	for i, da := range words {
		m.words[da.Device+strconv.Itoa(da.Addr)] = wordVals[i]
	}
	return nil
}
func (m *mockConn) RandomWriteBits(_ []mc.DeviceAddr, _ []bool) error { return nil }

func newMockPLCQueue(t *testing.T, words map[string]uint16) *PLCQueue {
	t.Helper()
	plc := newPLCClient("127.0.0.1", 5007, mc.ModeBinary)
	plc.conn = &mockConn{words: words}
	q := newPLCQueue(plc, 32)
	t.Cleanup(func() {
		_ = q.Shutdown(context.Background())
		q.Close()
	})
	return q
}

func TestHandleReadBitAccessReturnsBoolean(t *testing.T) {
	// D100 = 0b0000_0000_0000_0101 → bit0=true, bit1=false, bit2=true
	q := newMockPLCQueue(t, map[string]uint16{"D100": 0b101})

	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100.0", nil)
	rec := httptest.NewRecorder()
	handleRead(q)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	vals, _ := body["values"].([]any)
	if len(vals) != 1 || vals[0] != true {
		t.Errorf("values = %v, want [true]", vals)
	}

	// bit1 should be false
	req2 := httptest.NewRequest(http.MethodGet, "/read?addr=D100.1", nil)
	rec2 := httptest.NewRecorder()
	handleRead(q)(rec2, req2)
	var body2 map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&body2)
	vals2, _ := body2["values"].([]any)
	if len(vals2) != 1 || vals2[0] != false {
		t.Errorf("values = %v, want [false]", vals2)
	}
}

func TestHandleWriteBitAccessSetsAndClears(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{"D100": 0})

	// set bit0
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100.0", strings.NewReader(`{"values":[true]}`))
	rec := httptest.NewRecorder()
	handleWrite(q, false)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("set bit: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// read back: bit0 should be true
	req2 := httptest.NewRequest(http.MethodGet, "/read?addr=D100.0", nil)
	rec2 := httptest.NewRecorder()
	handleRead(q)(rec2, req2)
	var body map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&body)
	vals, _ := body["values"].([]any)
	if len(vals) != 1 || vals[0] != true {
		t.Errorf("after set: values = %v, want [true]", vals)
	}

	// clear bit0
	req3 := httptest.NewRequest(http.MethodPost, "/write?addr=D100.0", strings.NewReader(`{"values":[false]}`))
	rec3 := httptest.NewRecorder()
	handleWrite(q, false)(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("clear bit: status = %d, want %d", rec3.Code, http.StatusOK)
	}
}

func TestHandleReadBitAccessMultiBit(t *testing.T) {
	// D100 = 0b0000_0000_0011_0010 → bit1=true, bit2=false, bit3=false, bit4=true, bit5=true
	q := newMockPLCQueue(t, map[string]uint16{"D100": 0b110010})

	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100.1&count=5", nil)
	rec := httptest.NewRecorder()
	handleRead(q)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	vals, _ := body["values"].([]any)
	want := []bool{true, false, false, true, true}
	if len(vals) != len(want) {
		t.Fatalf("values = %v, want %v", vals, want)
	}
	for i, wv := range want {
		if vals[i] != wv {
			t.Errorf("values[%d] = %v, want %v", i, vals[i], wv)
		}
	}
}

func TestHandleWriteBitAccessMultiBit(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{"D100": 0})

	// set bits 2,3,4 to true,false,true starting at bit2
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100.2", strings.NewReader(`{"values":[true,false,true]}`))
	rec := httptest.NewRecorder()
	handleWrite(q, false)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// read back bits 2-4: should be true,false,true
	req2 := httptest.NewRequest(http.MethodGet, "/read?addr=D100.2&count=3", nil)
	rec2 := httptest.NewRecorder()
	handleRead(q)(rec2, req2)
	var body map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&body)
	vals, _ := body["values"].([]any)
	want := []bool{true, false, true}
	if len(vals) != len(want) {
		t.Fatalf("values = %v, want %v", vals, want)
	}
	for i, wv := range want {
		if vals[i] != wv {
			t.Errorf("values[%d] = %v, want %v", i, vals[i], wv)
		}
	}
}

func TestHandleWriteReadOnlyRejects(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/write?addr=D100", strings.NewReader(`{"values":[1]}`))
	rec := httptest.NewRecorder()

	handleWrite(newTestPLCQueue(t), true)(rec, req)

	assertReadOnlyError(t, rec)
}

func TestHandleRemoteReadOnlyRejects(t *testing.T) {
	readonly := true
	endpoints := []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{"run", "/remote/run", handleRemoteRun(newTestPLCQueue(t), readonly, true)},
		{"stop", "/remote/stop", handleRemoteStop(newTestPLCQueue(t), readonly, true)},
		{"pause", "/remote/pause", handleRemotePause(newTestPLCQueue(t), readonly, true)},
		{"latch-clear", "/remote/latch-clear", handleRemoteLatchClear(newTestPLCQueue(t), readonly, true)},
		{"reset", "/remote/reset", handleRemoteReset(newTestPLCQueue(t), readonly, true)},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, endpoint.path, nil)
			rec := httptest.NewRecorder()

			endpoint.handler(rec, req)

			assertReadOnlyError(t, rec)
		})
	}
}

func TestHandleRemoteDisabledRejects(t *testing.T) {
	endpoints := []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{"run", "/remote/run", handleRemoteRun(newTestPLCQueue(t), false, false)},
		{"stop", "/remote/stop", handleRemoteStop(newTestPLCQueue(t), false, false)},
		{"pause", "/remote/pause", handleRemotePause(newTestPLCQueue(t), false, false)},
		{"latch-clear", "/remote/latch-clear", handleRemoteLatchClear(newTestPLCQueue(t), false, false)},
		{"reset", "/remote/reset", handleRemoteReset(newTestPLCQueue(t), false, false)},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, endpoint.path, nil)
			rec := httptest.NewRecorder()

			endpoint.handler(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != "forbidden" {
				t.Fatalf("code = %q, want forbidden", body["code"])
			}
			if body["error"] != "remote-control operations are disabled (use -enable-remote to enable)" {
				t.Fatalf("error = %q, want remote-control disabled message", body["error"])
			}
		})
	}
}

func TestHandleRemoteRunForceParam(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"/remote/run", http.StatusOK},
		{"/remote/run?force=true", http.StatusOK},
		{"/remote/run?force=True", http.StatusOK},
		{"/remote/run?force=TRUE", http.StatusOK},
		{"/remote/run?force=false", http.StatusOK},
		{"/remote/run?force=False", http.StatusOK},
		{"/remote/run?clear=0", http.StatusOK},
		{"/remote/run?clear=2", http.StatusOK},
		{"/remote/run?force=true&clear=1", http.StatusOK},
		{"/remote/run?force=", http.StatusBadRequest},
		{"/remote/run?force=1", http.StatusBadRequest},
		{"/remote/run?force=true&force=false", http.StatusBadRequest},
		{"/remote/run?clear=3", http.StatusBadRequest},
		{"/remote/run?foo=bar", http.StatusBadRequest},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.query, nil)
		rec := httptest.NewRecorder()
		handleRemoteRun(newMockPLCQueue(t, nil), false, true)(rec, req)
		if rec.Code != tc.want {
			t.Errorf("query=%s status=%d, want=%d", tc.query, rec.Code, tc.want)
		}
	}
}

func TestHandleRemotePauseForceParam(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"/remote/pause", http.StatusOK},
		{"/remote/pause?force=true", http.StatusOK},
		{"/remote/pause?force=True", http.StatusOK},
		{"/remote/pause?force=TRUE", http.StatusOK},
		{"/remote/pause?force=false", http.StatusOK},
		{"/remote/pause?force=False", http.StatusOK},
		{"/remote/pause?force=", http.StatusBadRequest},
		{"/remote/pause?force=1", http.StatusBadRequest},
		{"/remote/pause?force=abc", http.StatusBadRequest},
		{"/remote/pause?foo=bar", http.StatusBadRequest},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.query, nil)
		rec := httptest.NewRecorder()
		handleRemotePause(newMockPLCQueue(t, nil), false, true)(rec, req)
		if rec.Code != tc.want {
			t.Errorf("query=%s status=%d, want=%d", tc.query, rec.Code, tc.want)
		}
	}
}

func TestHandleBoolParamEmptyValue(t *testing.T) {
	// ?dword= (empty) and ?dword=true&dword=false (duplicated) must return 400
	cases := []struct {
		query string
		want  int
	}{
		{"/read?addr=D100&dword=", http.StatusBadRequest},
		{"/read?addr=D100&sint=", http.StatusBadRequest},
		{"/read?addr=D100&dword=true&dword=false", http.StatusBadRequest},
	}
	q := newMockPLCQueue(t, map[string]uint16{})
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.query, nil)
		rec := httptest.NewRecorder()
		handleRead(q)(rec, req)
		if rec.Code != tc.want {
			t.Errorf("query=%s status=%d, want=%d", tc.query, rec.Code, tc.want)
		}
	}
}

func TestParseConfigEnableRemoteEnv(t *testing.T) {
	lookupEnv := func(key string) string {
		if key == "GOMCR_ENABLE_REMOTE" {
			return "true"
		}
		return ""
	}
	cfg, err := parseConfig(nil, lookupEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.EnableRemote {
		t.Fatal("EnableRemote = false, want true")
	}
}

func TestParseConfigRejectsInvalidEnableRemoteEnv(t *testing.T) {
	lookupEnv := func(key string) string {
		if key == "GOMCR_ENABLE_REMOTE" {
			return "yes"
		}
		return ""
	}
	_, err := parseConfig(nil, lookupEnv, nil)
	if err == nil || !strings.Contains(err.Error(), `invalid GOMCR_ENABLE_REMOTE "yes"`) {
		t.Fatalf("err = %v, want invalid GOMCR_ENABLE_REMOTE", err)
	}
}

func TestParseConfigEnableRemoteFlagOverridesInvalidEnv(t *testing.T) {
	lookupEnv := func(key string) string {
		if key == "GOMCR_ENABLE_REMOTE" {
			return "yes"
		}
		return ""
	}
	cfg, err := parseConfig([]string{"-enable-remote"}, lookupEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.EnableRemote {
		t.Fatal("EnableRemote = false, want true")
	}
}

func TestParseConfigLogLevelFlag(t *testing.T) {
	cfg, err := parseConfig([]string{"-log-level", "debug"}, emptyEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want DEBUG", cfg.LogLevel)
	}
}

func TestParseConfigLogLevelEnv(t *testing.T) {
	lookupEnv := func(key string) string {
		if key == "GOMCR_LOG_LEVEL" {
			return "warn"
		}
		return ""
	}
	cfg, err := parseConfig(nil, lookupEnv, nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Fatalf("LogLevel = %v, want WARN", cfg.LogLevel)
	}
}

func TestWriteErrIncludesHTTPStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()

	writeErr(rec, http.StatusBadRequest, "bad_request", "invalid request")

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["status"] != float64(http.StatusBadRequest) {
		t.Fatalf("status body = %v, want %d", body["status"], http.StatusBadRequest)
	}
	if body["code"] != "bad_request" {
		t.Fatalf("code = %q, want %q", body["code"], "bad_request")
	}
}

func TestHandleHealthUsesPLCStatusKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	plcQueue := newTestPLCQueue(t)

	handleHealth(plcQueue)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if _, ok := body["status"]; ok {
		t.Fatalf("health body must not use status key: %v", body)
	}
	if body["plc_status"] != "disconnected" {
		t.Fatalf("plc_status = %q, want disconnected", body["plc_status"])
	}
	if body["connected"] != false {
		t.Fatalf("connected = %v, want false", body["connected"])
	}
}

func TestWritePLCErrBusy(t *testing.T) {
	rec := httptest.NewRecorder()

	writePLCErr(rec, &busyErr{})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["status"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("status body = %v, want %d", body["status"], http.StatusServiceUnavailable)
	}
	if body["code"] != "busy" {
		t.Fatalf("code = %q, want busy", body["code"])
	}
}

func TestWritePLCErrQueueAndContextErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"queue closed", errQueueClosed, http.StatusServiceUnavailable, "queue_closed"},
		{"canceled", context.Canceled, 499, "request_canceled"},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout, "request_timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			writePLCErr(rec, tt.err)

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response body: %v", err)
			}
			if body["status"] != float64(tt.status) {
				t.Fatalf("status body = %v, want %d", body["status"], tt.status)
			}
			if body["code"] != tt.code {
				t.Fatalf("code = %q, want %q", body["code"], tt.code)
			}
		})
	}
}

func TestHandleReadReturnsBusyWhenQueueIsFull(t *testing.T) {
	plcQueue := newPLCQueue(newPLCClient("127.0.0.1", 5007, mc.ModeBinary), 1)
	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _ = plcQueue.work.Do(context.Background(), func() (any, error) {
			close(started)
			<-release
			return nil, nil
		})
	}()
	<-started
	plcQueue.work.jobs <- workJob{
		ctx:     context.Background(),
		execute: func() (any, error) { return nil, nil },
		result:  make(chan workResult, 1),
	}

	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100", nil)
	rec := httptest.NewRecorder()

	handleRead(plcQueue)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["status"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("status body = %v, want %d", body["status"], http.StatusServiceUnavailable)
	}
	if body["code"] != "busy" {
		t.Fatalf("code = %q, want busy", body["code"])
	}

	close(release)
	_ = plcQueue.Shutdown(context.Background())
}

func TestWorkQueueRunsSerially(t *testing.T) {
	queue := NewWorkQueue(4)
	defer func() {
		_ = queue.Shutdown(context.Background())
	}()

	started := make(chan struct{})
	release := make(chan struct{})
	secondRan := make(chan struct{})

	go func() {
		_, _ = queue.Do(context.Background(), func() (any, error) {
			close(started)
			<-release
			return nil, nil
		})
	}()

	<-started
	go func() {
		_, _ = queue.Do(context.Background(), func() (any, error) {
			close(secondRan)
			return nil, nil
		})
	}()

	select {
	case <-secondRan:
		t.Fatal("second job ran while first job was still active")
	default:
	}
	close(release)
	<-secondRan
}

func TestWorkQueueReturnsBusyWhenFull(t *testing.T) {
	queue := NewWorkQueue(1)
	defer func() {
		_ = queue.Shutdown(context.Background())
	}()

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _ = queue.Do(context.Background(), func() (any, error) {
			close(started)
			<-release
			return nil, nil
		})
	}()
	<-started

	queuedRan := make(chan struct{})
	queue.jobs <- workJob{
		ctx: context.Background(),
		execute: func() (any, error) {
			close(queuedRan)
			return nil, nil
		},
		result: make(chan workResult, 1),
	}

	_, err := queue.Do(context.Background(), func() (any, error) {
		t.Fatal("busy job must not execute")
		return nil, nil
	})
	if _, ok := err.(*busyErr); !ok {
		t.Fatalf("err = %T, want *busyErr", err)
	}

	close(release)
	<-queuedRan
}

func TestWorkQueueRejectsEnqueueAfterShutdown(t *testing.T) {
	queue := NewWorkQueue(1)
	if err := queue.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_, err := queue.Do(context.Background(), func() (any, error) {
		t.Fatal("job must not execute after shutdown")
		return nil, nil
	})
	if !errors.Is(err, errQueueClosed) {
		t.Fatalf("err = %v, want errQueueClosed", err)
	}
}

func TestWorkQueueAcceptedJobReturnsResultDuringShutdown(t *testing.T) {
	queue := NewWorkQueue(1)
	started := make(chan struct{})
	release := make(chan struct{})
	result := make(chan workResult, 1)

	go func() {
		value, err := queue.Do(context.Background(), func() (any, error) {
			close(started)
			<-release
			return "done", nil
		})
		result <- workResult{value: value, err: err}
	}()
	<-started

	shutdownErr := make(chan error, 1)
	go func() {
		shutdownErr <- queue.Shutdown(context.Background())
	}()

	select {
	case got := <-result:
		t.Fatalf("Do returned before active job finished: value=%v err=%v", got.value, got.err)
	default:
	}
	close(release)

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("err = %v, want nil", got.err)
		}
		if got.value != "done" {
			t.Fatalf("value = %v, want done", got.value)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not return active job result")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete")
	}
}

func TestWorkQueueSkipsBufferedJobsAfterShutdown(t *testing.T) {
	queue := &WorkQueue{
		jobs:       make(chan workJob, 1),
		done:       make(chan struct{}),
		workerDone: make(chan struct{}),
		closed:     true,
	}
	runJob := make(chan struct{})
	queue.jobs <- workJob{
		ctx: context.Background(),
		execute: func() (any, error) {
			close(runJob)
			return nil, nil
		},
		result: make(chan workResult, 1),
	}
	close(queue.done)

	go queue.run()

	select {
	case <-queue.workerDone:
		select {
		case <-runJob:
			t.Fatal("buffered job must not run after shutdown")
		default:
		}
	case <-runJob:
		t.Fatal("buffered job must not run after shutdown")
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after shutdown")
	}
}

func TestWorkQueueRejectsBufferedDoAfterShutdown(t *testing.T) {
	queue := NewWorkQueue(1)
	started := make(chan struct{})
	release := make(chan struct{})
	queuedRan := make(chan struct{})
	queuedResult := make(chan error, 1)

	go func() {
		_, _ = queue.Do(context.Background(), func() (any, error) {
			close(started)
			<-release
			return nil, nil
		})
	}()
	<-started

	go func() {
		_, err := queue.Do(context.Background(), func() (any, error) {
			close(queuedRan)
			return nil, nil
		})
		queuedResult <- err
	}()

	deadline := time.After(time.Second)
	for len(queue.jobs) != 1 {
		select {
		case <-deadline:
			t.Fatal("queued job was not accepted")
		default:
		}
	}
	shutdownErr := make(chan error, 1)
	go func() {
		shutdownErr <- queue.Shutdown(context.Background())
	}()
	waitForWorkQueueClosed(t, queue)
	close(release)

	select {
	case <-queuedRan:
		t.Fatal("buffered job must not execute after shutdown")
	case err := <-queuedResult:
		if !errors.Is(err, errQueueClosed) {
			t.Fatalf("err = %v, want errQueueClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued Do did not return after shutdown")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete")
	}
}

func TestHandleVersion(t *testing.T) {
	orig := version
	version = "v1.2.3"
	t.Cleanup(func() { version = orig })

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	handleVersion()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["version"] != "v1.2.3" {
		t.Fatalf("version = %v, want v1.2.3", body["version"])
	}
}

func TestHandleMetrics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handleMetrics(newTestPLCQueue(t))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"request_count", "reconnect_count", "plc_error_count", "avg_latency_ms", "recent_avg_latency_ms", "queue_length"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing field %q", key)
		}
	}
	if body["request_count"] != float64(0) {
		t.Errorf("request_count = %v, want 0", body["request_count"])
	}
	if body["avg_latency_ms"] != float64(0) {
		t.Errorf("avg_latency_ms = %v, want 0", body["avg_latency_ms"])
	}
	if body["recent_avg_latency_ms"] != float64(0) {
		t.Errorf("recent_avg_latency_ms = %v, want 0", body["recent_avg_latency_ms"])
	}
}

func TestHandleMetricsLatencyRounding(t *testing.T) {
	q := newTestPLCQueue(t)

	// Record 3 latencies that average to 1.23456...ms → should round to 1.23
	// 1234567 + 1234567 + 1234567 ns = 3703701 ns, avg = 1234567 ns = 1.234567 ms → 1.23
	q.plc.metrics.requests.Add(3)
	q.plc.metrics.recordLatency(1234567)
	q.plc.metrics.recordLatency(1234567)
	q.plc.metrics.recordLatency(1234567)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handleMetrics(q)(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["avg_latency_ms"] != float64(1.23) {
		t.Errorf("avg_latency_ms = %v, want 1.23", body["avg_latency_ms"])
	}
	if body["recent_avg_latency_ms"] != float64(1.23) {
		t.Errorf("recent_avg_latency_ms = %v, want 1.23", body["recent_avg_latency_ms"])
	}
}

func TestNoParamEndpointsRejectUnknownParams(t *testing.T) {
	// All no-param endpoints must return 400 for any unknown query parameter.
	mux := newTestMux(t)
	endpoints := []struct {
		method string
		path   string
	}{
		// GET endpoints
		{http.MethodGet, "/health?foo=bar"},
		{http.MethodGet, "/metrics?foo=bar"},
		{http.MethodGet, "/version?foo=bar"},
		{http.MethodGet, "/info?foo=bar"},
		{http.MethodGet, "/openapi.yaml?foo=bar"},
		// POST endpoints with no query params
		{http.MethodPost, "/random-read?foo=bar"},
		{http.MethodPost, "/random-write?foo=bar"},
		{http.MethodPost, "/remote/stop?foo=bar"},
		{http.MethodPost, "/remote/latch-clear?foo=bar"},
		{http.MethodPost, "/remote/reset?foo=bar"},
	}
	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s status=%d, want 400", ep.method, ep.path, rec.Code)
		}
	}
}

// newTestMux builds the same mux as main(), using a test PLCQueue.
func newTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	q := newTestPLCQueue(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", handleOpenAPI(openAPISpec))
	mux.HandleFunc("/version", handleVersion())
	mux.HandleFunc("/info", handleInfo(ServerConfig{Host: "127.0.0.1", Port: 5007, Frame: frame3E, Transport: transportTCP, ModeString: "binary", Listen: "127.0.0.1:0"}))
	mux.HandleFunc("/metrics", handleMetrics(q))
	mux.HandleFunc("/health", handleHealth(q))
	mux.HandleFunc("/read", handleRead(q))
	mux.HandleFunc("/write", handleWrite(q, false))
	mux.HandleFunc("/random-read", handleRandomRead(q))
	mux.HandleFunc("/random-write", handleRandomWrite(q, false))
	mux.HandleFunc("/remote/run", handleRemoteRun(q, false, true))
	mux.HandleFunc("/remote/stop", handleRemoteStop(q, false, true))
	mux.HandleFunc("/remote/pause", handleRemotePause(q, false, true))
	mux.HandleFunc("/remote/latch-clear", handleRemoteLatchClear(q, false, true))
	mux.HandleFunc("/remote/reset", handleRemoteReset(q, false, true))
	return mux
}

// specPaths parses the paths section of openapi.yaml without an external YAML library.
// It returns every line that starts with exactly two spaces followed by '/'.
func specPaths() []string {
	var paths []string
	inPaths := false
	for _, line := range strings.Split(string(openAPISpec), "\n") {
		if line == "paths:" {
			inPaths = true
			continue
		}
		if inPaths {
			// A top-level key under paths: exactly "  /something:"
			if strings.HasPrefix(line, "  /") && strings.HasSuffix(strings.TrimSpace(line), ":") {
				path := strings.TrimSuffix(strings.TrimSpace(line), ":")
				paths = append(paths, path)
			}
			// Stop at the next top-level YAML key (no leading spaces)
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
				break
			}
		}
	}
	return paths
}

// registeredRoutes lists every path registered in newTestMux / main.go.
// Keep this in sync with both newTestMux above and main.go.
var registeredRoutes = []string{
	"/openapi.yaml",
	"/version",
	"/info",
	"/metrics",
	"/health",
	"/read",
	"/write",
	"/remote/run",
	"/remote/stop",
	"/remote/pause",
	"/remote/latch-clear",
	"/remote/reset",
}

// TestOpenAPISpecVsRoutes verifies that the OpenAPI spec and the registered
// routes are consistent in both directions.
//
// ① Every path in openapi.yaml must respond with a non-404 status.
// ② Every registered route must appear in openapi.yaml.
func TestOpenAPISpecVsRoutes(t *testing.T) {
	mux := newTestMux(t)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	paths := specPaths()
	if len(paths) == 0 {
		t.Fatal("specPaths returned no paths — check openapi.yaml parsing")
	}

	// ① spec → implementation: each spec path must not return 404
	t.Run("spec_paths_exist", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		for _, path := range paths {
			resp, err := client.Get(srv.URL + path)
			if err != nil {
				t.Errorf("%s: request error: %v", path, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				t.Errorf("%s: got 404 — path is in spec but not registered", path)
			}
		}
	})

	// ② implementation → spec: each registered route must appear in the spec
	t.Run("routes_in_spec", func(t *testing.T) {
		specSet := make(map[string]bool, len(paths))
		for _, p := range paths {
			specSet[p] = true
		}
		for _, route := range registeredRoutes {
			if !specSet[route] {
				t.Errorf("%s: route is registered but missing from openapi.yaml", route)
			}
		}
	})
}

func TestMetricsClientFields(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{"D100": 42})

	// one successful read → client_request_count=1, busy_count=0, latency recorded
	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100", nil)
	rec := httptest.NewRecorder()
	handleRead(q)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d, want 200", rec.Code)
	}

	mreq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	mrec := httptest.NewRecorder()
	handleMetrics(q)(mrec, mreq)

	var m map[string]any
	if err := json.NewDecoder(mrec.Body).Decode(&m); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	for _, field := range []string{
		"client_request_count", "busy_count",
		"client_avg_latency_ms", "client_recent_avg_latency_ms",
	} {
		if _, ok := m[field]; !ok {
			t.Errorf("metrics missing field %q", field)
		}
	}
	if got := m["client_request_count"]; got != float64(1) {
		t.Errorf("client_request_count = %v, want 1", got)
	}
	if got := m["busy_count"]; got != float64(0) {
		t.Errorf("busy_count = %v, want 0", got)
	}
	if _, ok := m["client_recent_avg_latency_ms"].(float64); !ok {
		t.Errorf("client_recent_avg_latency_ms missing or wrong type, got %v", m["client_recent_avg_latency_ms"])
	}
}

func TestMetricsBusyExcludedFromLatency(t *testing.T) {
	// queue size 1, fill the slot so the next request returns busy
	plc := newPLCClient("127.0.0.1", 5007, mc.ModeBinary)
	plc.conn = &mockConn{words: map[string]uint16{}}
	q := newPLCQueue(plc, 1)
	t.Cleanup(func() {
		_ = q.Shutdown(context.Background())
		q.Close()
	})

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _ = q.work.Do(context.Background(), func() (any, error) {
			close(started)
			<-release
			return nil, nil
		})
	}()
	<-started
	// fill queue buffer so next exec returns busy
	q.work.jobs <- workJob{
		ctx:     context.Background(),
		execute: func() (any, error) { return nil, nil },
		result:  make(chan workResult, 1),
	}

	req := httptest.NewRequest(http.MethodGet, "/read?addr=D100", nil)
	rec := httptest.NewRecorder()
	handleRead(q)(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	close(release)

	mreq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	mrec := httptest.NewRecorder()
	handleMetrics(q)(mrec, mreq)

	var m map[string]any
	if err := json.NewDecoder(mrec.Body).Decode(&m); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if got := m["busy_count"]; got != float64(1) {
		t.Errorf("busy_count = %v, want 1", got)
	}
	// busy request must not contribute to client latency average
	if got := m["client_recent_avg_latency_ms"]; got != float64(0) {
		t.Errorf("client_recent_avg_latency_ms = %v, want 0 (busy excluded)", got)
	}
}

func TestHandleRandomRead(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{"D100": 1234, "D200": 5678})

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"empty body", `{}`, http.StatusBadRequest},
		{"both empty arrays", `{"words":[],"dwords":[]}`, http.StatusBadRequest},
		{"bit device in words", `{"words":["M0"]}`, http.StatusBadRequest},
		{"bit suffix", `{"words":["D100.0"]}`, http.StatusBadRequest},
		{"invalid addr", `{"words":["ZZZ0"]}`, http.StatusBadRequest},
		{"ok words only", `{"words":["D100","D200"]}`, http.StatusOK},
		{"ok dwords only", `{"dwords":["D100"]}`, http.StatusOK},
		{"ok words and dwords", `{"words":["D100"],"dwords":["D200"]}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/random-read", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handleRandomRead(q)(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}

	t.Run("returns correct values", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/random-read", strings.NewReader(`{"words":["D100","D200"]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomRead(q)(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var resp struct {
			Words  []int   `json:"words"`
			Dwords []int64 `json:"dwords"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Words) != 2 || resp.Words[0] != 1234 || resp.Words[1] != 5678 {
			t.Errorf("words = %v, want [1234 5678]", resp.Words)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/random-read", nil)
		rec := httptest.NewRecorder()
		handleRandomRead(q)(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", rec.Code)
		}
	})

	t.Run("sets RawQuery for logging on 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/random-read",
			strings.NewReader(`{"words":["D100","D200"],"dwords":["D300"]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomRead(q)(rec, req)
		want := "words=D100,D200&dwords=D300"
		if req.URL.RawQuery != want {
			t.Errorf("RawQuery = %q, want %q", req.URL.RawQuery, want)
		}
	})

	t.Run("sets RawQuery for logging on 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/random-read",
			strings.NewReader(`{"words":["INVALID"]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomRead(q)(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		want := "words=INVALID&dwords="
		if req.URL.RawQuery != want {
			t.Errorf("RawQuery = %q, want %q", req.URL.RawQuery, want)
		}
	})

	t.Run("truncates RawQuery at maxLogQuery", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString(`{"words":["`)
		for i := 0; i < 50; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(`"D` + strconv.Itoa(i*100) + `"`)
		}
		sb.WriteString(`]}`)
		req := httptest.NewRequest(http.MethodPost, "/random-read", strings.NewReader(sb.String()))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomRead(q)(rec, req)
		if len(req.URL.RawQuery) > maxLogQuery {
			t.Errorf("RawQuery length %d exceeds maxLogQuery=%d: %q", len(req.URL.RawQuery), maxLogQuery, req.URL.RawQuery)
		}
		if !strings.HasSuffix(req.URL.RawQuery, "...") {
			t.Errorf("RawQuery should end with '...', got %q", req.URL.RawQuery)
		}
	})
}

func TestHandleRandomWrite(t *testing.T) {
	q := newMockPLCQueue(t, map[string]uint16{})

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"empty body", `{}`, http.StatusBadRequest},
		{"all empty arrays", `{"words":[],"dwords":[],"bits":[]}`, http.StatusBadRequest},
		{"bit device in words", `{"words":[{"addr":"M0","value":1}]}`, http.StatusBadRequest},
		{"word device in bits", `{"bits":[{"addr":"D100","value":true}]}`, http.StatusBadRequest},
		{"bit suffix in words", `{"words":[{"addr":"D100.0","value":1}]}`, http.StatusBadRequest},
		{"ok words", `{"words":[{"addr":"D100","value":42}]}`, http.StatusOK},
		{"ok bits", `{"bits":[{"addr":"M0","value":true}]}`, http.StatusOK},
		{"ok dwords", `{"dwords":[{"addr":"D100","value":65536}]}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/random-write", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handleRandomWrite(q, false)(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}

	t.Run("writes word values", func(t *testing.T) {
		q2 := newMockPLCQueue(t, map[string]uint16{})
		req := httptest.NewRequest(http.MethodPost, "/random-write",
			strings.NewReader(`{"words":[{"addr":"D10","value":999}]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomWrite(q2, false)(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("readonly rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/random-write",
			strings.NewReader(`{"words":[{"addr":"D10","value":1}]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomWrite(q, true)(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("sets RawQuery for logging on 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/random-write",
			strings.NewReader(`{"words":[{"addr":"D100","value":1}],"dwords":[],"bits":[{"addr":"M0","value":true}]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomWrite(q, false)(rec, req)
		want := "words=D100&dwords=&bits=M0"
		if req.URL.RawQuery != want {
			t.Errorf("RawQuery = %q, want %q", req.URL.RawQuery, want)
		}
	})

	t.Run("sets RawQuery for logging on 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/random-write",
			strings.NewReader(`{"words":[{"addr":"INVALID","value":1}]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleRandomWrite(q, false)(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		want := "words=INVALID&dwords=&bits="
		if req.URL.RawQuery != want {
			t.Errorf("RawQuery = %q, want %q", req.URL.RawQuery, want)
		}
	})
}

func waitForWorkQueueClosed(t *testing.T, queue *WorkQueue) {
	t.Helper()
	deadline := time.After(time.Second)
	for !queue.isClosed() {
		select {
		case <-deadline:
			t.Fatal("queue was not marked closed")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

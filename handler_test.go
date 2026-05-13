package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestHandleReadRequiresGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/read?addr=D100", nil)
	rec := httptest.NewRecorder()

	handleRead(newTestPLCQueue(t))(rec, req)

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
		})
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

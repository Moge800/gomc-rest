package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	mc "github.com/moge800/gomcprotocol"
)

const (
	maxReadCount   = 1024
	maxWriteValues = 1024
	maxWriteBody   = 1 << 20
)

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	body := map[string]string{"error": msg, "code": code}
	writeJSON(w, status, body)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeErr(w, http.StatusMethodNotAllowed, "bad_request", method+" required")
	return false
}

func writePLCErr(w http.ResponseWriter, err error) {
	var connErr *connErrWrap
	var plcErr *mc.MCProtocolError
	switch {
	case errors.As(err, &connErr):
		writeErr(w, http.StatusServiceUnavailable, "connection_error", err.Error())
	case errors.As(err, &plcErr):
		body := map[string]string{
			"error":    err.Error(),
			"code":     "plc_error",
			"end_code": "0x" + strconv.FormatUint(uint64(plcErr.EndCode), 16),
		}
		writeJSON(w, http.StatusBadGateway, body)
	default:
		writeErr(w, http.StatusBadGateway, "plc_error", err.Error())
	}
}

// GET /health
func handleHealth(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		plc.mu.Lock()
		connected := plc.isConnected()
		plc.mu.Unlock()

		status := "ok"
		if !connected {
			status = "disconnected"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    status,
			"connected": connected,
		})
	}
}

// GET /read?addr=D100&count=5
func handleRead(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		addrStr := r.URL.Query().Get("addr")
		if addrStr == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "addr is required")
			return
		}
		countStr := r.URL.Query().Get("count")
		count := 1
		if countStr != "" {
			n, err := strconv.Atoi(countStr)
			if err != nil || n < 1 {
				writeErr(w, http.StatusBadRequest, "bad_request", "invalid count")
				return
			}
			count = n
		}
		if count > maxReadCount {
			writeErr(w, http.StatusBadRequest, "bad_request", "count must be "+strconv.Itoa(maxReadCount)+" or less")
			return
		}

		da, err := parseAddr(addrStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		if isWordDevice(da.Device) {
			var words []uint16
			if err := plc.do(func(c *mc.Client3E) error {
				var e error
				words, e = c.ReadWords(da.Device, da.Addr, count)
				return e
			}); err != nil {
				writePLCErr(w, err)
				return
			}
			vals := make([]int, len(words))
			for i, v := range words {
				vals[i] = int(v)
			}
			writeJSON(w, http.StatusOK, map[string]any{"values": vals})
		} else {
			var bits []bool
			if err := plc.do(func(c *mc.Client3E) error {
				var e error
				bits, e = c.ReadBits(da.Device, da.Addr, count)
				return e
			}); err != nil {
				writePLCErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"values": bits})
		}
	}
}

// POST /write?addr=D100   body: {"values":[1,2,3]} or {"values":[true,false]}
func handleWrite(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}

		addrStr := r.URL.Query().Get("addr")
		if addrStr == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "addr is required")
			return
		}

		da, err := parseAddr(addrStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		var body struct {
			Values json.RawMessage `json:"values"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBody)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeErr(w, http.StatusRequestEntityTooLarge, "bad_request", "body must not be larger than 1 MiB")
				return
			}
			writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		if len(body.Values) == 0 {
			writeErr(w, http.StatusBadRequest, "bad_request", "values is required")
			return
		}

		if isWordDevice(da.Device) {
			var vals []uint16
			if err := json.Unmarshal(body.Values, &vals); err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", "values must be array of integers")
				return
			}
			if len(vals) == 0 {
				writeErr(w, http.StatusBadRequest, "bad_request", "values must not be empty")
				return
			}
			if len(vals) > maxWriteValues {
				writeErr(w, http.StatusBadRequest, "bad_request", "values must contain "+strconv.Itoa(maxWriteValues)+" items or less")
				return
			}
			if err := plc.do(func(c *mc.Client3E) error {
				return c.WriteWords(da.Device, da.Addr, vals)
			}); err != nil {
				writePLCErr(w, err)
				return
			}
		} else {
			var vals []bool
			if err := json.Unmarshal(body.Values, &vals); err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", "values must be array of booleans")
				return
			}
			if len(vals) == 0 {
				writeErr(w, http.StatusBadRequest, "bad_request", "values must not be empty")
				return
			}
			if len(vals) > maxWriteValues {
				writeErr(w, http.StatusBadRequest, "bad_request", "values must contain "+strconv.Itoa(maxWriteValues)+" items or less")
				return
			}
			if err := plc.do(func(c *mc.Client3E) error {
				return c.WriteBits(da.Device, da.Addr, vals)
			}); err != nil {
				writePLCErr(w, err)
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/run?clear=0&force=false
func handleRemoteRun(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		clear := 0
		if s := r.URL.Query().Get("clear"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 || n > 2 {
				writeErr(w, http.StatusBadRequest, "bad_request", "clear must be 0, 1, or 2")
				return
			}
			clear = n
		}
		force := r.URL.Query().Get("force") == "true"

		if err := plc.do(func(c *mc.Client3E) error {
			return c.RemoteRun(clear, force)
		}); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/stop
func handleRemoteStop(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if err := plc.do(func(c *mc.Client3E) error {
			return c.RemoteStop()
		}); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/pause?force=false
func handleRemotePause(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		force := r.URL.Query().Get("force") == "true"
		if err := plc.do(func(c *mc.Client3E) error {
			return c.RemotePause(force)
		}); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/latch-clear
func handleRemoteLatchClear(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if err := plc.do(func(c *mc.Client3E) error {
			return c.RemoteLatchClear()
		}); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/reset
func handleRemoteReset(plc *PLCClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if err := plc.doReset(); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

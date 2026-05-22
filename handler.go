package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	mc "github.com/moge800/gomcprotocol"
)

const (
	maxLogQuery    = 300
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
	body := map[string]any{"status": status, "error": msg, "code": code}
	writeJSON(w, status, body)
}

// buildLogQuery builds "k0=v,v&k1=v,v" from keys/vals, capped at maxLogQuery bytes.
// Every write goes through appendStr which stops early so no excess allocation occurs.
func buildLogQuery(keys []string, vals [][]string) string {
	var b strings.Builder
	b.Grow(maxLogQuery)
	// appendStr writes s into b, truncating with "..." if needed.
	// Returns true when s fitted fully; false when the limit was reached.
	appendStr := func(s string) bool {
		rem := maxLogQuery - b.Len()
		if rem <= 0 {
			return false
		}
		if len(s) < rem {
			b.WriteString(s)
			return true
		}
		if rem > 3 {
			b.WriteString(s[:rem-3])
			b.WriteString("...")
		} else {
			b.WriteString(s[:rem])
		}
		return false
	}
	for i, key := range keys {
		if i > 0 && !appendStr("&") {
			return b.String()
		}
		if !appendStr(key + "=") {
			return b.String()
		}
		for j, v := range vals[i] {
			if j > 0 && !appendStr(",") {
				return b.String()
			}
			if !appendStr(v) {
				return b.String()
			}
		}
	}
	return b.String()
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	// RFC 9110: servers that support GET must also support HEAD.
	if method == http.MethodGet && r.Method == http.MethodHead {
		return true
	}
	allow := method
	if method == http.MethodGet {
		allow = "GET, HEAD"
	}
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "bad_request", method+" required")
	return false
}

func requireWritable(w http.ResponseWriter, readonly bool) bool {
	if !readonly {
		return true
	}
	writeErr(w, http.StatusForbidden, "forbidden", "operation not allowed in read-only mode")
	return false
}

func requireRemoteEnabled(w http.ResponseWriter, enableRemote bool) bool {
	if enableRemote {
		return true
	}
	writeErr(w, http.StatusForbidden, "forbidden", "remote-control operations are disabled (use -enable-remote to enable)")
	return false
}

func writePLCErr(w http.ResponseWriter, err error) {
	var busy *busyErr
	var connErr *connErrWrap
	var plcErr *mc.MCProtocolError
	switch {
	case errors.As(err, &busy):
		w.Header().Set("Retry-After", "1")
		writeErr(w, http.StatusServiceUnavailable, "busy", err.Error())
	case errors.Is(err, errQueueClosed):
		writeErr(w, http.StatusServiceUnavailable, "queue_closed", err.Error())
	case errors.Is(err, context.Canceled):
		writeErr(w, 499, "request_canceled", "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		writeErr(w, http.StatusGatewayTimeout, "request_timeout", "request timed out")
	case errors.As(err, &connErr):
		writeErr(w, http.StatusServiceUnavailable, "connection_error", err.Error())
	case errors.As(err, &plcErr):
		body := map[string]any{
			"status":   http.StatusBadGateway,
			"error":    err.Error(),
			"code":     "plc_error",
			"end_code": "0x" + strconv.FormatUint(uint64(plcErr.EndCode), 16),
		}
		writeJSON(w, http.StatusBadGateway, body)
	default:
		writeErr(w, http.StatusBadGateway, "plc_error", err.Error())
	}
}

// GET /openapi.yaml
func handleOpenAPI(spec []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(spec)
	}
}

// GET /metrics
func handleMetrics(plc *PLCQueue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, plc.Metrics())
	}
}

// GET /version
func handleVersion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"version": version})
	}
}

// GET /info
func handleInfo(cfg ServerConfig) http.HandlerFunc {
	// Compute once at handler creation; neither value changes during the process lifetime.
	addrs := listenAddrs(cfg.Listen)
	mcVersion := gomcprotocolVersion()
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"version":              version,
			"gomcprotocol_version": mcVersion,
			"host":                 cfg.Host,
			"port":                 cfg.Port,
			"frame":                string(cfg.Frame),
			"transport":            string(cfg.Transport),
			"mode":                 cfg.ModeString,
			"listen_addrs":         addrs,
			"readonly":             cfg.ReadOnly,
			"enable_remote":        cfg.EnableRemote,
		})
	}
}

// gomcprotocolVersion returns the version of the embedded gomcprotocol module
// by reading the build info embedded in the binary at startup.
func gomcprotocolVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range bi.Deps {
		if dep.Path == "github.com/moge800/gomcprotocol" {
			return dep.Version
		}
	}
	return "unknown"
}

// listenAddrs returns the addresses (host:port) the HTTP server is reachable on.
// If the listen address binds to all interfaces (host empty or 0.0.0.0),
// it returns all non-loopback IPv4 addresses combined with the port.
// Otherwise it returns the single configured address.
func listenAddrs(listen string) []string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return nil
	}
	if host != "" {
		if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
			return []string{net.JoinHostPort(host, port)}
		}
	}
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var addrs []string
	for _, a := range ifaces {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() {
			continue
		}
		addrs = append(addrs, net.JoinHostPort(ip.String(), port))
	}
	return addrs
}

// GET /health
func handleHealth(plc *PLCQueue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		connected := plc.IsConnected()

		plcStatus := "ok"
		if !connected {
			plcStatus = "disconnected"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"plc_status": plcStatus,
			"connected":  connected,
		})
	}
}

// GET /read?addr=D100&count=5
// GET /read?addr=D100&count=5&dword=true  (reads 32-bit double words; word devices only)
func handleRead(plc *PLCQueue) http.HandlerFunc {
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

		dword := r.URL.Query().Get("dword") == "true"

		if dword && count > maxReadCount/2 {
			writeErr(w, http.StatusBadRequest, "bad_request", "dword count must be "+strconv.Itoa(maxReadCount/2)+" or less")
			return
		}

		da, err := parseAddr(addrStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		sint := r.URL.Query().Get("sint") == "true"

		if da.Bit >= 0 {
			if dword || sint {
				writeErr(w, http.StatusBadRequest, "bad_request", "dword and sint flags are not supported with bit access")
				return
			}
			if count != 1 {
				writeErr(w, http.StatusBadRequest, "bad_request", "count must be 1 for bit access")
				return
			}
			val, err := plc.ReadWordBit(r.Context(), da.Device, da.Addr, da.Bit)
			if err != nil {
				writePLCErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"values": []bool{val}})
			return
		}

		if (dword || sint) && !isWordDevice(da.Device) {
			if dword {
				writeErr(w, http.StatusBadRequest, "bad_request", "dword is only supported for word devices")
			} else {
				writeErr(w, http.StatusBadRequest, "bad_request", "sint is only supported for word devices")
			}
			return
		}

		if isWordDevice(da.Device) {
			readCount := count
			if dword {
				readCount = count * 2
			}
			words, err := plc.ReadWords(r.Context(), da.Device, da.Addr, readCount)
			if err != nil {
				writePLCErr(w, err)
				return
			}
			if dword {
				vals := make([]int64, count)
				for i := range vals {
					lo := uint32(words[i*2])
					hi := uint32(words[i*2+1])
					combined := lo | hi<<16
					if sint {
						vals[i] = int64(int32(combined))
					} else {
						vals[i] = int64(combined)
					}
				}
				writeJSON(w, http.StatusOK, map[string]any{"values": vals})
			} else {
				vals := make([]int, len(words))
				for i, v := range words {
					if sint {
						vals[i] = int(int16(v))
					} else {
						vals[i] = int(v)
					}
				}
				writeJSON(w, http.StatusOK, map[string]any{"values": vals})
			}
		} else {
			bits, err := plc.ReadBits(r.Context(), da.Device, da.Addr, count)
			if err != nil {
				writePLCErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"values": bits})
		}
	}
}

// POST /write?addr=D100   body: {"values":[1,2,3]} or {"values":[true,false]}
// POST /write?addr=D100&dword=true   body: {"values":[100000,200000]} (32-bit double words; word devices only)
func handleWrite(plc *PLCQueue, readonly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireWritable(w, readonly) {
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

		dword := r.URL.Query().Get("dword") == "true"
		sint := r.URL.Query().Get("sint") == "true"

		if da.Bit >= 0 {
			if dword || sint {
				writeErr(w, http.StatusBadRequest, "bad_request", "dword and sint flags are not supported with bit access")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxWriteBody)
			var body struct {
				Values json.RawMessage `json:"values"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					writeErr(w, http.StatusRequestEntityTooLarge, "bad_request", "body must not be larger than "+strconv.FormatInt(maxBytesErr.Limit, 10)+" bytes")
					return
				}
				writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
				return
			}
			if len(body.Values) == 0 {
				writeErr(w, http.StatusBadRequest, "bad_request", "values is required")
				return
			}
			var vals []bool
			if err := json.Unmarshal(body.Values, &vals); err != nil || len(vals) != 1 {
				writeErr(w, http.StatusBadRequest, "bad_request", "values must be an array of exactly one boolean for bit access")
				return
			}
			if err := plc.WriteWordBit(r.Context(), da.Device, da.Addr, da.Bit, vals[0]); err != nil {
				writePLCErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}

		if (dword || sint) && !isWordDevice(da.Device) {
			if dword {
				writeErr(w, http.StatusBadRequest, "bad_request", "dword is only supported for word devices")
			} else {
				writeErr(w, http.StatusBadRequest, "bad_request", "sint is only supported for word devices")
			}
			return
		}

		var body struct {
			Values json.RawMessage `json:"values"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBody)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeErr(w, http.StatusRequestEntityTooLarge, "bad_request", "body must not be larger than "+strconv.FormatInt(maxBytesErr.Limit, 10)+" bytes")
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
			if dword {
				var dvals []int64
				if err := json.Unmarshal(body.Values, &dvals); err != nil {
					writeErr(w, http.StatusBadRequest, "bad_request", "values must be array of integers")
					return
				}
				if len(dvals) == 0 {
					writeErr(w, http.StatusBadRequest, "bad_request", "values must not be empty")
					return
				}
				if len(dvals) > maxWriteValues/2 {
					writeErr(w, http.StatusBadRequest, "bad_request", "values must contain "+strconv.Itoa(maxWriteValues/2)+" items or less")
					return
				}
				words := make([]uint16, len(dvals)*2)
				for i, v := range dvals {
					var u uint32
					if sint {
						if v < math.MinInt32 || v > math.MaxInt32 {
							writeErr(w, http.StatusBadRequest, "bad_request", "sint dword values must be in range -2147483648..2147483647")
							return
						}
						u = uint32(int32(v))
					} else {
						if v < 0 || v > 1<<32-1 {
							writeErr(w, http.StatusBadRequest, "bad_request", "dword values must be in range 0..4294967295")
							return
						}
						u = uint32(v)
					}
					words[i*2] = uint16(u)
					words[i*2+1] = uint16(u >> 16)
				}
				if err := plc.WriteWords(r.Context(), da.Device, da.Addr, words); err != nil {
					writePLCErr(w, err)
					return
				}
			} else if sint {
				var ivals []int
				if err := json.Unmarshal(body.Values, &ivals); err != nil {
					writeErr(w, http.StatusBadRequest, "bad_request", "values must be array of integers")
					return
				}
				if len(ivals) == 0 {
					writeErr(w, http.StatusBadRequest, "bad_request", "values must not be empty")
					return
				}
				if len(ivals) > maxWriteValues {
					writeErr(w, http.StatusBadRequest, "bad_request", "values must contain "+strconv.Itoa(maxWriteValues)+" items or less")
					return
				}
				words := make([]uint16, len(ivals))
				for i, v := range ivals {
					if v < math.MinInt16 || v > math.MaxInt16 {
						writeErr(w, http.StatusBadRequest, "bad_request", "sint word values must be in range -32768..32767")
						return
					}
					words[i] = uint16(int16(v))
				}
				if err := plc.WriteWords(r.Context(), da.Device, da.Addr, words); err != nil {
					writePLCErr(w, err)
					return
				}
			} else {
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
				if err := plc.WriteWords(r.Context(), da.Device, da.Addr, vals); err != nil {
					writePLCErr(w, err)
					return
				}
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
			if err := plc.WriteBits(r.Context(), da.Device, da.Addr, vals); err != nil {
				writePLCErr(w, err)
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/run?clear=0&force=false
func handleRemoteRun(plc *PLCQueue, readonly, enableRemote bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireWritable(w, readonly) {
			return
		}
		if !requireRemoteEnabled(w, enableRemote) {
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

		if err := plc.RemoteRun(r.Context(), clear, force); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/stop
func handleRemoteStop(plc *PLCQueue, readonly, enableRemote bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireWritable(w, readonly) {
			return
		}
		if !requireRemoteEnabled(w, enableRemote) {
			return
		}
		if err := plc.RemoteStop(r.Context()); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/pause?force=false
func handleRemotePause(plc *PLCQueue, readonly, enableRemote bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireWritable(w, readonly) {
			return
		}
		if !requireRemoteEnabled(w, enableRemote) {
			return
		}
		force := r.URL.Query().Get("force") == "true"
		if err := plc.RemotePause(r.Context(), force); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/latch-clear
func handleRemoteLatchClear(plc *PLCQueue, readonly, enableRemote bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireWritable(w, readonly) {
			return
		}
		if !requireRemoteEnabled(w, enableRemote) {
			return
		}
		if err := plc.RemoteLatchClear(r.Context()); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /random-read   body: {"words":["D100","D200"],"dwords":["D300"]}
func handleRandomRead(plc *PLCQueue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var body struct {
			Words  []string `json:"words"`
			Dwords []string `json:"dwords"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBody)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeErr(w, http.StatusRequestEntityTooLarge, "bad_request", "body must not be larger than "+strconv.FormatInt(maxBytesErr.Limit, 10)+" bytes")
				return
			}
			writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		r.URL.RawQuery = buildLogQuery([]string{"words", "dwords"}, [][]string{body.Words, body.Dwords})
		if len(body.Words)+len(body.Dwords) == 0 {
			writeErr(w, http.StatusBadRequest, "bad_request", "words and dwords must not both be empty")
			return
		}
		if len(body.Words) > maxRandomCount || len(body.Dwords) > maxRandomCount {
			writeErr(w, http.StatusBadRequest, "bad_request", "each array must contain "+strconv.Itoa(maxRandomCount)+" items or less")
			return
		}
		parseWordAddr := func(s string) (mc.DeviceAddr, bool) {
			pa, err := parseAddr(s)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
				return mc.DeviceAddr{}, false
			}
			if pa.Bit >= 0 {
				writeErr(w, http.StatusBadRequest, "bad_request", "bit suffix not allowed: "+s)
				return mc.DeviceAddr{}, false
			}
			if !isWordDevice(pa.Device) {
				writeErr(w, http.StatusBadRequest, "bad_request", "only word devices are supported, got: "+s)
				return mc.DeviceAddr{}, false
			}
			return pa.DeviceAddr, true
		}
		wordAddrs := make([]mc.DeviceAddr, len(body.Words))
		for i, s := range body.Words {
			da, ok := parseWordAddr(s)
			if !ok {
				return
			}
			wordAddrs[i] = da
		}
		dwordAddrs := make([]mc.DeviceAddr, len(body.Dwords))
		for i, s := range body.Dwords {
			da, ok := parseWordAddr(s)
			if !ok {
				return
			}
			dwordAddrs[i] = da
		}
		wordVals, dwordVals, err := plc.RandomRead(r.Context(), wordAddrs, dwordAddrs)
		if err != nil {
			writePLCErr(w, err)
			return
		}
		wordInts := make([]int, len(wordVals))
		for i, v := range wordVals {
			wordInts[i] = int(v)
		}
		dwordInts := make([]int64, len(dwordVals))
		for i, v := range dwordVals {
			dwordInts[i] = int64(v)
		}
		writeJSON(w, http.StatusOK, map[string]any{"words": wordInts, "dwords": dwordInts})
	}
}

// POST /random-write   body: {"words":[{"addr":"D100","value":1}],"dwords":[...],"bits":[{"addr":"M0","value":true}]}
func handleRandomWrite(plc *PLCQueue, readonly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireWritable(w, readonly) {
			return
		}
		var body struct {
			Words []struct {
				Addr  string `json:"addr"`
				Value uint16 `json:"value"`
			} `json:"words"`
			Dwords []struct {
				Addr  string `json:"addr"`
				Value uint32 `json:"value"`
			} `json:"dwords"`
			Bits []struct {
				Addr  string `json:"addr"`
				Value bool   `json:"value"`
			} `json:"bits"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBody)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeErr(w, http.StatusRequestEntityTooLarge, "bad_request", "body must not be larger than "+strconv.FormatInt(maxBytesErr.Limit, 10)+" bytes")
				return
			}
			writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		ws := make([]string, min(len(body.Words), maxRandomCount))
		for i := range ws {
			ws[i] = body.Words[i].Addr
		}
		ds := make([]string, min(len(body.Dwords), maxRandomCount))
		for i := range ds {
			ds[i] = body.Dwords[i].Addr
		}
		bs := make([]string, min(len(body.Bits), maxRandomCount))
		for i := range bs {
			bs[i] = body.Bits[i].Addr
		}
		r.URL.RawQuery = buildLogQuery([]string{"words", "dwords", "bits"}, [][]string{ws, ds, bs})
		if len(body.Words)+len(body.Dwords)+len(body.Bits) == 0 {
			writeErr(w, http.StatusBadRequest, "bad_request", "words, dwords, and bits must not all be empty")
			return
		}
		if len(body.Words) > maxRandomCount || len(body.Dwords) > maxRandomCount || len(body.Bits) > maxRandomCount {
			writeErr(w, http.StatusBadRequest, "bad_request", "each array must contain "+strconv.Itoa(maxRandomCount)+" items or less")
			return
		}
		wordAddrs := make([]mc.DeviceAddr, len(body.Words))
		wordVals := make([]uint16, len(body.Words))
		for i, e := range body.Words {
			pa, err := parseAddr(e.Addr)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
				return
			}
			if pa.Bit >= 0 {
				writeErr(w, http.StatusBadRequest, "bad_request", "bit suffix not allowed: "+e.Addr)
				return
			}
			if !isWordDevice(pa.Device) {
				writeErr(w, http.StatusBadRequest, "bad_request", "words must be word devices, got: "+e.Addr)
				return
			}
			wordAddrs[i] = pa.DeviceAddr
			wordVals[i] = e.Value
		}
		dwordAddrs := make([]mc.DeviceAddr, len(body.Dwords))
		dwordVals := make([]uint32, len(body.Dwords))
		for i, e := range body.Dwords {
			pa, err := parseAddr(e.Addr)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
				return
			}
			if pa.Bit >= 0 {
				writeErr(w, http.StatusBadRequest, "bad_request", "bit suffix not allowed: "+e.Addr)
				return
			}
			if !isWordDevice(pa.Device) {
				writeErr(w, http.StatusBadRequest, "bad_request", "dwords must be word devices, got: "+e.Addr)
				return
			}
			dwordAddrs[i] = pa.DeviceAddr
			dwordVals[i] = e.Value
		}
		bitAddrs := make([]mc.DeviceAddr, len(body.Bits))
		bitVals := make([]bool, len(body.Bits))
		for i, e := range body.Bits {
			pa, err := parseAddr(e.Addr)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
				return
			}
			if pa.Bit >= 0 {
				writeErr(w, http.StatusBadRequest, "bad_request", "bit suffix not allowed: "+e.Addr)
				return
			}
			if isWordDevice(pa.Device) {
				writeErr(w, http.StatusBadRequest, "bad_request", "bits must be bit devices, got: "+e.Addr)
				return
			}
			bitAddrs[i] = pa.DeviceAddr
			bitVals[i] = e.Value
		}
		if err := plc.RandomWrite(r.Context(), wordAddrs, wordVals, dwordAddrs, dwordVals, bitAddrs, bitVals); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// POST /remote/reset
func handleRemoteReset(plc *PLCQueue, readonly, enableRemote bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireWritable(w, readonly) {
			return
		}
		if !requireRemoteEnabled(w, enableRemote) {
			return
		}
		if err := plc.RemoteReset(r.Context()); err != nil {
			writePLCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

# gomc-rest

[日本語 README](README_jp.md)

[![Release](https://img.shields.io/github/v/release/moge800/gomc-rest)](https://github.com/moge800/gomc-rest/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/moge800/gomc-rest/ci.yml?branch=main&label=CI)](https://github.com/moge800/gomc-rest/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey)](#download)

`gomc-rest` is a small REST API server and HTTP gateway for Mitsubishi Electric PLCs using MC protocol 3E or 4E frames. It sits between HTTP clients and a PLC, brokering reads and writes for device strings such as `D100.0`, `W100`, and `M0` while returning JSON values with automatic conversion: word devices become integers and bit devices become booleans.

The PLC transport is provided by [gomcprotocol](https://github.com/moge800/gomcprotocol). The server uses only the Go standard library for HTTP handling.

A Python client library with no extra dependencies is also available: [gomc-rest-client on PyPI](https://pypi.org/project/gomc-rest-client/)

A lightweight debugging GUI is also available: [gomc-rest-gui](https://github.com/moge800/gomc-rest-gui) — a standalone HTTP client (Wails + React) for exercising gomc-rest's endpoints from a simple screen instead of curl.

## Features

- Read word and bit devices through a simple `/read` endpoint.
- Write integer arrays or boolean arrays through `/write`.
- Run, stop, pause, latch-clear, and reset the PLC remotely.
- Configure by command-line flags, with environment variables as defaults.
- Start in read-only mode to block write and remote-control endpoints.
- Serialize PLC communication through a single in-process worker queue.
- Keep a simple health endpoint that reports the current connection state.
- Retry the PLC connection on demand when startup connection fails or a previous connection was cleared.

## Network Scope

**⚠️ Caution: Do not expose this server to the Internet, an office LAN, or any untrusted network.**

This server is intended only for FA local networks, such as an isolated factory LAN, a trusted machine network, or localhost access from an operator PC. The API can read, write, run, stop, pause, latch-clear, and reset a PLC, and it does not provide authentication, authorization, TLS, or access control.

Recommended deployment:

- Run it inside an isolated FA network or on `localhost` only.
- Restrict access with network segmentation, firewall rules, or host-level controls.
- Use `-listen 127.0.0.1:8080` when only local access is required.
- Do not place it behind a public reverse proxy or expose it through port forwarding.

## Download

Download the latest release from the [Releases](https://github.com/moge800/gomc-rest/releases) page.

| Platform | File |
| --- | --- |
| Windows (amd64) | `gomc-rest.exe` |
| Linux (amd64) | `gomc-rest-linux-amd64` |
| Linux (arm64 / Raspberry Pi 5) | `gomc-rest-linux-arm64` |
| macOS (Intel) | `gomc-rest-darwin-amd64` |
| macOS (Apple Silicon) | `gomc-rest-darwin-arm64` |

## Quick Start (Windows)

### 1. Create a batch file

Create `start-gomc-rest.bat` in the same folder as `gomc-rest.exe` and edit the values at the top to match your environment:

```bat
@echo off
REM ============================================================
REM  Edit these values to match your environment
REM ============================================================
set PLC_HOST=192.168.0.1
set PLC_PORT=5007
set LISTEN_PORT=8080
REM ============================================================

gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT%
pause
```

Double-click the batch file to start the server. To stop it, press **Ctrl+C** or close the window. The `pause` line keeps the window open after the server exits so you can read any error messages.

### 2. Verify the server is running

Open a browser and go to:

```
http://localhost:8080/health
```

You should see:

```json
{"plc_status":"ok","connected":true}
```

If the PLC is not reachable yet, `connected` will be `false`. The server still starts and will retry the connection on the first PLC operation (`/read`, `/write`, or `/remote/*`).

### Read-only mode

To block all write and remote-control operations (for monitoring only):

```bat
@echo off
set PLC_HOST=192.168.0.1
set PLC_PORT=5007
set LISTEN_PORT=8080

gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT% -readonly
pause
```

### Save logs to a file

```bat
@echo off
set PLC_HOST=192.168.0.1
set PLC_PORT=5007
set LISTEN_PORT=8080
set LOG_FILE=C:\gomc-rest.log

gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT% -log-file %LOG_FILE%
pause
```

Logs are written to both the console and the file. The directory must already exist; the server does not create missing parent directories.

### Enable remote control

Remote-control endpoints (`/remote/run`, `/remote/stop`, etc.) are disabled by default. Add `-enable-remote` to turn them on:

```bat
gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT% -enable-remote
```

## Troubleshooting

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `{"connected":false}` on `/health` | PLC is off or IP/port is wrong | Check `PLC_HOST` and `PLC_PORT` in the batch file |
| `403 forbidden` on `/write` | Server started with `-readonly` | Remove `-readonly` from the batch file |
| `403 forbidden` on `/remote/*` | `-enable-remote` is not set | Add `-enable-remote` to the batch file |
| `503 busy` | Too many simultaneous requests | Increase `-queue-size` (default: 32) |
| Port already in use | Another process is using the port | Change `LISTEN_PORT` to a free port |
| Window closes immediately | Startup error | Run from Command Prompt to see the error message |

## Run

For the Windows release binary:

```powershell
.\gomc-rest.exe -host 192.168.0.1 -port 5007 -mode binary -listen :8080
```

For source builds or non-Windows environments:

```bash
./gomc-rest -host 192.168.0.1 -port 5007 -frame 3e -transport tcp -mode binary -queue-size 32 -timeout 5s -listen :8080
```

For read-only operation, add `-readonly`. In read-only mode, `/health` and `/read` remain available, while POST operations on `/write` and `/remote/*` return `403 forbidden`.

```powershell
.\gomc-rest.exe -host 192.168.0.1 -port 5007 -mode binary -listen 127.0.0.1:8080 -readonly
```

On startup, the server attempts to connect to the PLC. If the PLC is not reachable, startup continues and the server retries on the first PLC request.

## Build from source

```bash
git clone https://github.com/moge800/gomc-rest
cd gomc-rest
go build -o gomc-rest .
```

## Configuration

Flags take priority. Environment variables provide the default values for those flags.

| Flag | Environment variable | Default | Notes |
| --- | --- | --- | --- |
| `-host` | `GOMCR_HOST` | `192.168.0.1` | PLC host or IP address |
| `-port` | `GOMCR_PORT` | `5007` | PLC port, `1` to `65535` |
| `-frame` | `GOMCR_FRAME` | `3e` | MC protocol frame, `3e` or `4e` |
| `-transport` | `GOMCR_TRANSPORT` | `tcp` | `tcp` or `udp`; `4e` supports `tcp` only |
| `-mode` | `GOMCR_MODE` | `binary` | `binary` or `ascii` |
| `-queue-size` | `GOMCR_QUEUE_SIZE` | `32` | Number of PLC requests that can wait while one request is active |
| `-timeout` | `GOMCR_TIMEOUT` | `5s` | PLC connect and I/O timeout |
| `-listen` | `GOMCR_LISTEN` | `:8080` | HTTP listen address |
| `-readonly` | `GOMCR_READONLY` | `false` | Set to `true` to reject POST operations on `/write` and `/remote/*` |
| `-enable-remote` | `GOMCR_ENABLE_REMOTE` | `false` | Set to `true` to enable remote-control endpoints (`/remote/*`) |
| `-log-file` | `GOMCR_LOG_FILE` | _(none)_ | Path to log file; if set, logs are written to both the file and stderr |
| `-log-level` | `GOMCR_LOG_LEVEL` | `info` | Terminal log level: `debug`, `info`, `warn`, or `error` |
| `-log-file-level` | `GOMCR_LOG_FILE_LEVEL` | `warn` | File log level: `debug`, `info`, `warn`, or `error`; only used with `-log-file` |
| `-token` | `GOMCR_TOKEN` | _(none)_ | Static bearer token. If set, all endpoints except `/health` require `Authorization: Bearer <token>`. The flag overrides the env var and is convenient for per-instance tokens, but is visible in the process list. |

## Authentication

By default there is no authentication, matching the closed-network use case.
Set `GOMCR_TOKEN` to require a static bearer token on every request except the
`/health` liveness probe:

Pass it either as the `-token` flag or the `GOMCR_TOKEN` environment variable
(the flag wins; gomc-rest does not read a `.env` file):

```bat
REM Windows (batch), environment variable
set GOMCR_TOKEN=your-shared-secret
gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT%
```

```sh
# Linux / macOS, environment variable
GOMCR_TOKEN=your-shared-secret ./gomc-rest -host 192.168.0.1 -port 5007
```

When running **multiple instances** on one host, the `-token` flag gives each
one its own token without worrying about a shared/inherited env var:

```sh
./gomc-rest -listen :8080 -token token-for-plc-a -host 192.168.0.1 -port 5007
./gomc-rest -listen :8081 -token token-for-plc-b -host 192.168.0.2 -port 5007
```

> **Note:** values passed via `-token` are visible in the process list (`ps`,
> Task Manager). On the closed networks gomc-rest targets this is usually
> acceptable; use `GOMCR_TOKEN` instead if you want to keep it out of `ps`.

```sh
# client
curl -H "Authorization: Bearer your-shared-secret" "http://localhost:8080/read?addr=D100"
```

A request with a missing or wrong token gets `401 unauthorized`.

> **Note:** the token travels in cleartext over HTTP. This is intentional for
> air-gapped FA networks. If you need transport encryption, put a
> TLS-terminating reverse proxy (nginx, Caddy, …) in front of gomc-rest.

## API Reference

All successful write and remote-control operations return:

```json
{"ok":true}
```

| Method | Path | Parameters / body | Response |
| --- | --- | --- | --- |
| `GET` | `/openapi.yaml` | none | OpenAPI 3.1 specification (YAML) |
| `GET` | `/version` | none | `{"version":"v0.9.0"}` or `{"version":"dev"}` for local builds |
| `GET` | `/info` | none | `{"version":"v0.9.0","gomcprotocol_version":"v0.3.0","host":"192.168.0.1","port":5007,"frame":"3e","transport":"tcp","mode":"binary","listen_addrs":["192.168.1.10:8080"],"readonly":false,"enable_remote":false}` |
| `GET` | `/metrics` | none | `{"client_request_count":0,"busy_count":0,"client_avg_latency_ms":0,"client_recent_avg_latency_ms":0,"queue_length":0,"request_count":0,"reconnect_count":0,"timeout_count":0,"plc_error_count":0,"avg_latency_ms":0,"recent_avg_latency_ms":0}` |
| `GET` | `/health` | none | `{"plc_status":"ok","connected":true}` or `{"plc_status":"disconnected","connected":false}` |
| `GET` | `/read` | query: `addr` required, `count` optional and defaults to `1`, `dword` optional and defaults to `false`, `sint` optional and defaults to `false` | `{"values":[100,200]}` or `{"values":[true,false]}` |
| `POST` | `/write` | query: `addr` required, `dword` optional and defaults to `false`, `sint` optional and defaults to `false`; body: `{"values":[1,2,3]}` or `{"values":[true,false]}` | `{"ok":true}` |
| `POST` | `/random-read` | body: `{"words":["D100","D200"],"dwords":["D300"]}` | `{"words":[100,200],"dwords":[300]}` |
| `POST` | `/random-write` | body: `{"words":[{"addr":"D100","value":1}],"dwords":[{"addr":"D300","value":65536}],"bits":[{"addr":"M0","value":true}]}` | `{"ok":true}` |
| `POST` | `/remote/run` | requires `-enable-remote`; query: `clear=0/1/2` optional, `force=true/false` optional | `{"ok":true}` |
| `POST` | `/remote/stop` | requires `-enable-remote`; none | `{"ok":true}` |
| `POST` | `/remote/pause` | requires `-enable-remote`; query: `force=true/false` optional | `{"ok":true}` |
| `POST` | `/remote/latch-clear` | requires `-enable-remote`; none | `{"ok":true}` |
| `POST` | `/remote/reset` | requires `-enable-remote`; none | `{"ok":true}` |

Notes:

- `count` must be between `1` and `1024`.
- `values` must be present and contain between `1` and `1024` items.
- The `/write` request body must be 1 MiB or smaller.
- Word devices require integer values in the range `0..65535`. Bit devices require boolean values.
- When `dword=true`, each value is an unsigned 32-bit integer in the range `0..4294967295`. The low 16 bits are stored in the register at `addr` and the high 16 bits in the next register (`addr+1`). Only word devices support `dword=true`. With `dword=true`, `count` must be `512` or less and `values` must contain `512` items or less (so that the actual word count sent to the PLC does not exceed `1024`).
- When `sint=true`, values are interpreted as signed integers. For word devices the range is `-32768..32767`; for `dword=true` the range is `-2147483648..2147483647`. Only word devices support `sint=true`. The PLC register bits are unchanged — `sint` only affects how values are converted between JSON and the 16-bit register representation.
- `/random-read` accepts `words`, `dwords`, and `bits` arrays of device address strings. All default to empty; at least one must be non-empty. `words` and `dwords` require word devices (`D`, `W`, `R`, etc.). `bits` accepts word-device bit access (e.g. `D100.1`) and bit devices (e.g. `M0`): word-device bits are read by folding the containing word into the single random read (0x0403) and masking the bit; bit devices have no MC random-read unit, so each is read with a separate `ReadBits` (0x0401) and is therefore capped at 16 per request (use word-device bit access for more). Returns `{"words":[...],"dwords":[...],"bits":[...]}` where word values are integers (`0..65535`), dword values are unsigned 32-bit integers (`0..4294967295`), and bit values are booleans in request order. Maximum 255 entries per array.
- `/random-write` accepts `words`, `dwords`, and `bits` arrays. Each entry is `{"addr":"...","value":...}`. `words` and `dwords` require word devices. `bits` accepts both bit devices (e.g. `M0`) and word-device bit access (e.g. `D100.1`); the latter is applied via read-modify-write, with bits in the same word merged into a single read/write. Internally calls `RandomWrite` for words/dwords and `RandomWriteBits` for bit devices within one serialized job. Maximum 255 entries per array.
  > **Performance note:** word-device bit access is not a native MC operation. Each distinct word touched in `bits` costs an extra read + write round-trip (`/random-write`), and each native bit device in `/random-read` `bits` costs a separate read round-trip. At ~20 ms per PLC round-trip these add up quickly, so group bits of the same word into one request, and prefer plain word writes when you control all 16 bits. Splitting the same bits across multiple requests multiplies the read-modify-write cost.
- `/remote/*` endpoints are disabled by default and return `403 forbidden` unless the server is started with `-enable-remote`. This is separate from read-only mode.
- When read-only mode is enabled, POST operations on `/write` and `/remote/*` return `403 forbidden`. Read-only mode is a safety aid, not a replacement for network isolation, authentication, authorization, firewall rules, or PLC-side protection.
- Boolean query flags (`dword`, `sint`, `force`) accept `true` or `false` (case-insensitive, exactly once). Any other value (including empty or duplicated) returns `400 bad_request`. All endpoints reject unknown query parameters with `400 bad_request` (when the request passes earlier checks such as method, read-only, and remote-enabled validation).
- `GET /health` always returns HTTP `200`, even when the PLC is disconnected.
- `/remote/reset` clears the TCP connection because the PLC closes it after reset.
- `/metrics` PLC fields (`request_count`, `avg_latency_ms`, `recent_avg_latency_ms`) measure only the PLC wire time. Client fields (`client_request_count`, `client_avg_latency_ms`, `client_recent_avg_latency_ms`) measure the full round-trip including queue wait time. `busy_count` counts requests rejected because the queue was full; these are excluded from the client latency averages. `timeout_count` counts requests that returned `context.DeadlineExceeded` (timed out before, during, or after queuing).

## Device Addressing

Device addresses are case-insensitive and may include surrounding whitespace. The device prefix determines whether the API reads or writes words or bits.

| Type | Devices | JSON value type | Examples |
| --- | --- | --- | --- |
| Word | `D`, `W`, `R`, `ZR`, `TN`, `STN`, `CN`, `Z`, `SW`, `SD` | integer | `D100`, `ZR512`, `TN10`, `CN5` |
| Bit | `X`, `Y`, `M`, `L`, `B`, `F`, `V`, `SB`, `SM`, `S`, `DX`, `DY`, `TC`, `TS`, `STC`, `STS`, `CC`, `CS` | boolean | `M0`, `TC10`, `CC5`, `STC3` |

Timer and counter contacts and coils use two-letter prefixes: `TC` (timer contact), `TS` (timer coil), `CC` (counter contact), `CS` (counter coil). The single-letter forms `T` and `C` are not valid device names and return `400 bad_request`.

The numeric address must be a non-negative integer. Unknown devices, missing numbers, non-numeric numbers, and negative numbers return `400 bad_request`. The devices `X`, `Y`, `B`, `SB`, `W`, `SW`, `ZR`, `DX`, and `DY` use hexadecimal address numbers (e.g. `X4F`, `Y12D2`, `W1D`).

### Word Device Bit Access

Append `.N` (single hex digit, `0`–`F`) to a word device address to read or write one or more consecutive bits within the 16-bit register, starting at bit `N`.

```
D100.0   ← bit 0 (LSB) of D100
D100.F   ← bit 15 (MSB) of D100
W1D.7     ← bit 7 of W1D (hex address 0x1D)
```

- Read with `count` returns consecutive bits starting at `N`: `GET /read?addr=D100.1&count=5` returns bits 1–5 of D100 as `{"values": [true, false, ...]}`.
- Write body is an array of booleans applied to consecutive bits starting at `N`: `{"values": [true, false, true]}` sets bits `N`, `N+1`, `N+2`. The server performs a single read-modify-write internally.
- Bits stay within one word: `N + count` (or `N + len(values)`) must not exceed `16`, otherwise `400 bad_request`. For example `D100.F&count=2` is rejected.
- `dword=true` or `sint=true` combined with bit access returns `400 bad_request`.
- Appending `.N` to a bit device (e.g. `M0.0`) returns `400 bad_request`.

## Error Responses

Errors are returned as JSON. Every error response includes both the HTTP status code and a machine-readable `code` in the body.

| Scenario | HTTP | `code` | Example |
| --- | --- | --- | --- |
| Invalid parameter, body, address, count, or method | `400` or `405` | `bad_request` | `{"status":400,"error":"addr is required","code":"bad_request"}` |
| `/remote/*` endpoint called without `-enable-remote` | `403` | `forbidden` | `{"status":403,"error":"remote-control operations are disabled (use -enable-remote to enable)","code":"forbidden"}` |
| Operation rejected by read-only mode | `403` | `forbidden` | `{"status":403,"error":"operation not allowed in read-only mode","code":"forbidden"}` |
| `/write` body is too large | `413` | `bad_request` | `{"status":413,"error":"body must not be larger than 1048576 bytes","code":"bad_request"}` |
| PLC MC protocol error with an end code | `502` | `plc_error` | `{"status":502,"error":"MC error 0x4000","code":"plc_error","end_code":"0x4000"}` |
| PLC connection error | `503` | `connection_error` | `{"status":503,"error":"connect: refused","code":"connection_error"}` |
| PLC communication queue is full | `503` | `busy` | `{"status":503,"error":"PLC communication queue is full","code":"busy"}` |
| PLC communication queue is closed during shutdown | `503` | `queue_closed` | `{"status":503,"error":"PLC communication queue is closed","code":"queue_closed"}` |
| HTTP request context was canceled before completion | `499` | `request_canceled` | `{"status":499,"error":"request canceled","code":"request_canceled"}` |
| HTTP request context deadline expired | `504` | `request_timeout` | `{"status":504,"error":"request timed out","code":"request_timeout"}` |

## Connection Behavior

- PLC requests are serialized through one shared in-process worker queue and one client connection.
- The worker executes one PLC request at a time. HTTP handlers do not call the PLC client directly.
- `-queue-size` controls how many PLC requests can wait while one request is active. When the queue is full, the server immediately returns `503 busy` with `Retry-After: 1`.
- `-timeout` controls the PLC connect and I/O deadline. It does not cancel HTTP request contexts that are already disconnected; queued requests are skipped before execution if their request context is canceled.
- If initial connection fails, the HTTP server still starts.
- If there is no active connection, the next PLC request attempts to reconnect.
- Connection-level MC protocol errors clear the connection so a later request can reconnect.

## Examples

The `/health` endpoint can be used without a PLC. The other examples require a reachable PLC.

```bash
curl http://localhost:8080/health
curl "http://localhost:8080/read?addr=D100&count=3"
curl "http://localhost:8080/read?addr=M0&count=4"
curl "http://localhost:8080/read?addr=D100&count=2&dword=true"
curl "http://localhost:8080/read?addr=D100&count=3&sint=true"

curl -X POST "http://localhost:8080/write?addr=D100" \
  -H "Content-Type: application/json" \
  -d '{"values":[10,20,30]}'

curl -X POST "http://localhost:8080/write?addr=D100&dword=true" \
  -H "Content-Type: application/json" \
  -d '{"values":[100000,200000]}'

curl -X POST "http://localhost:8080/write?addr=D100&sint=true" \
  -H "Content-Type: application/json" \
  -d '{"values":[-1,-32768,32767]}'

curl -X POST "http://localhost:8080/write?addr=M0" \
  -H "Content-Type: application/json" \
  -d '{"values":[true,false]}'

# Random read/write — multiple non-contiguous addresses in one request
curl -X POST "http://localhost:8080/random-read" \
  -H "Content-Type: application/json" \
  -d '{"words":["D100","D200"],"dwords":["D300"]}'

curl -X POST "http://localhost:8080/random-write" \
  -H "Content-Type: application/json" \
  -d '{"words":[{"addr":"D100","value":10},{"addr":"D200","value":20}],"bits":[{"addr":"M0","value":true}]}'

# Remote-control endpoints require -enable-remote at startup
curl -X POST "http://localhost:8080/remote/run?clear=0&force=false"
curl -X POST "http://localhost:8080/remote/stop"
curl -X POST "http://localhost:8080/remote/pause?force=false"
curl -X POST "http://localhost:8080/remote/latch-clear"
curl -X POST "http://localhost:8080/remote/reset"
```

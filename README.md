# gomc-rest

[日本語 README](README_jp.md)

`gomc-rest` is a small REST API server for Mitsubishi Electric PLCs using the MC protocol 3E frame. It lets HTTP clients read and write PLC devices such as `D100` or `M0`, returning JSON values with automatic conversion: word devices become integers and bit devices become booleans.

The PLC transport is provided by [gomcprotocol](https://github.com/moge800/gomcprotocol). The server uses only the Go standard library for HTTP handling.

## Features

- Read word and bit devices through a simple `/read` endpoint.
- Write integer arrays or boolean arrays through `/write`.
- Run, stop, pause, latch-clear, and reset the PLC remotely.
- Configure by command-line flags, with environment variables as defaults.
- Keep a simple health endpoint that reports the current connection state.
- Retry the PLC connection on demand when startup connection fails or a previous connection was cleared.

## Download

Download the latest `gomc-rest.exe` from the [Releases](https://github.com/moge800/gomc-rest/releases) page.

## Run

```bash
./gomc-rest.exe -host 192.168.0.1 -port 5007 -mode binary -listen :8080
```

On startup, the server attempts to connect to the PLC. If the PLC is not reachable, startup continues and the server retries on the first PLC request.

## Network Scope

This server is intended only for FA local networks, such as an isolated factory LAN, a trusted machine network, or localhost access from an operator PC. Do not expose it to the Internet, an office LAN, or any untrusted network. The API can read, write, run, stop, pause, latch-clear, and reset a PLC, and it does not provide authentication, authorization, TLS, or access control.

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
| `-host` | `PLC_HOST` | `192.168.0.1` | PLC host or IP address |
| `-port` | `PLC_PORT` | `5007` | PLC port, `1` to `65535` |
| `-mode` | `PLC_MODE` | `binary` | `binary` or `ascii` |
| `-listen` | `LISTEN_ADDR` | `:8080` | HTTP listen address |

## API Reference

All successful write and remote-control operations return:

```json
{"ok":true}
```

| Method | Path | Parameters / body | Response |
| --- | --- | --- | --- |
| `GET` | `/health` | none | `{"status":"ok","connected":true}` or `{"status":"disconnected","connected":false}` |
| `GET` | `/read` | query: `addr` required, `count` optional and defaults to `1` | `{"values":[100,200]}` or `{"values":[true,false]}` |
| `POST` | `/write` | query: `addr` required; body: `{"values":[1,2,3]}` or `{"values":[true,false]}` | `{"ok":true}` |
| `POST` | `/remote/run` | query: `clear=0|1|2` optional, `force=true|false` optional | `{"ok":true}` |
| `POST` | `/remote/stop` | none | `{"ok":true}` |
| `POST` | `/remote/pause` | query: `force=true|false` optional | `{"ok":true}` |
| `POST` | `/remote/latch-clear` | none | `{"ok":true}` |
| `POST` | `/remote/reset` | none | `{"ok":true}` |

Notes:

- `count` must be between `1` and `1024`.
- `values` must be present, must not be empty, and may contain at most `1024` items.
- The JSON request body for `/write` is limited to 1 MiB.
- Word devices require integer values in the range `0..65535`. Bit devices require boolean values.
- `force` is enabled only when the query value is exactly `true`.
- `/health` always returns HTTP `200`, even when the PLC is disconnected.
- `/remote/reset` clears the TCP connection because the PLC closes it after reset.

## Device Addressing

Device addresses are case-insensitive and may include surrounding whitespace. The device prefix determines whether the API reads or writes words or bits.

| Type | Devices | JSON value type | Examples |
| --- | --- | --- | --- |
| Word | `D`, `W`, `R`, `ZR`, `TN`, `CN`, `Z`, `SW`, `SD` | integer | `D100`, `ZR512`, `SW5` |
| Bit | `X`, `Y`, `M`, `L`, `B`, `F`, `SB`, `SM` | boolean | `M0`, `X10`, `SB10` |

The numeric address must be a non-negative integer. Unknown devices, missing numbers, non-numeric numbers, and negative numbers return `400 bad_request`.

## Error Responses

Errors are returned as JSON.

| Scenario | HTTP | `code` | Example |
| --- | --- | --- | --- |
| Invalid parameter, body, address, or count; invalid method on POST-only endpoints | `400` or `405` | `bad_request` | `{"error":"addr is required","code":"bad_request"}` |
| PLC MC protocol error with an end code | `502` | `plc_error` | `{"error":"MC error 0x4000","code":"plc_error","end_code":"0x4000"}` |
| PLC connection error | `503` | `connection_error` | `{"error":"connect: refused","code":"connection_error"}` |

## Connection Behavior

- PLC requests are serialized through one shared client connection.
- The MC protocol client timeout is set to 5 seconds.
- The HTTP server uses request timeouts to avoid stalled clients.
- If initial connection fails, the HTTP server still starts.
- If there is no active connection, the next PLC request attempts to reconnect.
- Connection-level MC protocol errors clear the connection so a later request can reconnect.

## Examples

The `/health` endpoint can be used without a PLC. The other examples require a reachable PLC.

```bash
curl http://localhost:8080/health
curl "http://localhost:8080/read?addr=D100&count=3"
curl "http://localhost:8080/read?addr=M0&count=4"

curl -X POST "http://localhost:8080/write?addr=D100" \
  -H "Content-Type: application/json" \
  -d '{"values":[10,20,30]}'

curl -X POST "http://localhost:8080/write?addr=M0" \
  -H "Content-Type: application/json" \
  -d '{"values":[true,false]}'

curl -X POST "http://localhost:8080/remote/run?clear=0&force=false"
curl -X POST "http://localhost:8080/remote/stop"
curl -X POST "http://localhost:8080/remote/pause?force=false"
curl -X POST "http://localhost:8080/remote/latch-clear"
curl -X POST "http://localhost:8080/remote/reset"
```

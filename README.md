# gomc-rest

三菱電機 PLC（MC プロトコル 3E フレーム）向けの REST API サーバー。
`D100` のようなデバイス文字列で読み書きでき、ワード→int / ビット→bool を自動変換した JSON を返す。

依存: [gomcprotocol](https://github.com/moge800/gomcprotocol)（標準ライブラリのみ、外部フレームワークなし）

---

## ビルド

```bash
git clone https://github.com/moge800/gomc-rest
cd gomc-rest
go build -o gomc-rest .
```

---

## 設定（フラグ優先・環境変数デフォルト）

| フラグ | 環境変数 | デフォルト |
|--------|----------|------------|
| `-host` | `PLC_HOST` | `192.168.0.1` |
| `-port` | `PLC_PORT` | `5007` |
| `-mode` | `PLC_MODE` | `binary` |
| `-listen` | `LISTEN_ADDR` | `:8080` |

---

## エンドポイント

| Method | Path | 説明 |
|--------|------|------|
| GET | `/read?addr=D100&count=5` | ワード→int / ビット→bool 自動判定 |
| POST | `/write?addr=D100` | body: `{"values":[1,2,3]}` or `{"values":[true,false]}` |
| POST | `/remote/run?clear=0&force=false` | RemoteRun |
| POST | `/remote/stop` | RemoteStop |
| POST | `/remote/pause?force=false` | RemotePause |
| POST | `/remote/latch-clear` | RemoteLatchClear |
| POST | `/remote/reset` | RemoteReset |
| GET | `/health` | `{"status":"ok","connected":true}` |

---

## エラーレスポンス

| 状態 | HTTP | code |
|------|------|------|
| パラメータ不正 | 400 | `bad_request` |
| PLC エラー (end code) | 502 | `plc_error` |
| 接続エラー | 503 | `connection_error` |
| `/health` は常に | 200 | — |

```json
{"error": "MC error 0x4000", "code": "plc_error",        "end_code": "0x4000"}
{"error": "connect: refused", "code": "connection_error"}
{"error": "invalid addr",     "code": "bad_request"}
```

---

## 動作確認

```bash
./gomc-rest -host 192.168.0.1 -port 5007

# 疎通（実機なしでも返る）
curl http://localhost:8080/health
# → {"connected":false,"status":"disconnected"}

# ワード読み
curl "http://localhost:8080/read?addr=D100&count=3"
# → {"values":[100,200,300]}

# ビット読み
curl "http://localhost:8080/read?addr=M0&count=4"
# → {"values":[true,false,true,false]}

# ワード書き
curl -X POST "http://localhost:8080/write?addr=D100" \
  -H "Content-Type: application/json" \
  -d '{"values":[10,20,30]}'
# → {"ok":true}

# ビット書き
curl -X POST "http://localhost:8080/write?addr=M0" \
  -H "Content-Type: application/json" \
  -d '{"values":[true,false]}'
# → {"ok":true}

# RemoteRun
curl -X POST "http://localhost:8080/remote/run"
# → {"ok":true}
```

# gomc-rest

[English README](README.md)

`gomc-rest` は、三菱電機 PLC（MC プロトコル 3E / 4E フレーム）向けの小さな REST API サーバーです。HTTP クライアントから `D100` や `M0` のようなデバイス文字列を指定して読み書きでき、ワードデバイスは整数、ビットデバイスは真偽値として JSON に自動変換します。

PLC 通信には [gomcprotocol](https://github.com/moge800/gomcprotocol) を使用します。HTTP サーバー部分は Go の標準ライブラリのみで実装されています。

Python クライアントライブラリもあります: [gomc-rest-client (PyPI)](https://pypi.org/project/gomc-rest-client/)

## 機能

- `/read` でワードデバイスとビットデバイスを読み取り。
- `/write` で整数配列または真偽値配列を書き込み。
- RemoteRun、RemoteStop、RemotePause、RemoteLatchClear、RemoteReset に対応。
- コマンドラインフラグで設定し、環境変数をデフォルト値として利用。
- 読み取り専用モードで起動し、書き込みとリモート操作を拒否可能。
- プロセス内の単一 worker queue で PLC 通信を直列化。
- `/health` で現在の接続状態を確認。
- 起動時接続に失敗しても HTTP サーバーは起動し、PLC 要求時に再接続を試行。

## ネットワーク範囲

**⚠️ 注意: このサーバーをインターネット、社内 OA LAN、その他の信頼できないネットワークへ公開しないでください。**

このサーバーは、隔離された工場 LAN、信頼できる装置ネットワーク、または作業端末上の localhost など、FA ローカル環境での利用のみを想定しています。この API は PLC の読み取り、書き込み、運転、停止、一時停止、ラッチクリア、リセットを実行でき、認証、認可、TLS、アクセス制御は備えていません。

推奨する配置:

- 隔離された FA ネットワーク内、または `localhost` のみで実行してください。
- ネットワーク分離、ファイアウォール、ホスト側のアクセス制御で到達範囲を制限してください。
- ローカル端末からだけ使う場合は `-listen 127.0.0.1:8080` を指定してください。
- 公開リバースプロキシの背後に置いたり、ポートフォワードで外部公開したりしないでください。

## ダウンロード

[Releases](https://github.com/moge800/gomc-rest/releases) ページから最新の `gomc-rest.exe` をダウンロードしてください。

公開リリースでは Windows 用バイナリ名を `gomc-rest.exe` としています。ソースからビルドする場合は、下記のビルドコマンドで指定した出力名になります。

## クイックスタート（Windows）

### 1. バッチファイルを作成する

`gomc-rest.exe` と同じフォルダに `start-gomc-rest.bat` を作成し、先頭の設定値をご使用の環境に合わせて書き換えてください。

```bat
@echo off
REM ============================================================
REM  環境に合わせてここを書き換えてください
REM ============================================================
set PLC_HOST=192.168.0.1
set PLC_PORT=5007
set LISTEN_PORT=8080
REM ============================================================

gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT%
pause
```

バッチファイルをダブルクリックするとサーバーが起動します。停止するには **Ctrl+C** を押すかウィンドウを閉じてください。`pause` はサーバー終了後にウィンドウを開いたままにしてエラーメッセージを確認できるようにするためのものです。

### 2. 起動確認

ブラウザで以下を開いてください。

```
http://localhost:8080/health
```

以下のように返ってくれば正常に起動しています。

```json
{"plc_status":"ok","connected":true}
```

PLC に未接続の場合は `connected` が `false` になります。サーバーは起動を続け、最初の PLC 操作（`/read`・`/write`・`/remote/*`）時に再接続を試みます。

### 読み取り専用モード

書き込みおよびリモート操作をすべてブロックする場合（モニタリングのみ）:

```bat
@echo off
set PLC_HOST=192.168.0.1
set PLC_PORT=5007
set LISTEN_PORT=8080

gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT% -readonly
pause
```

### ログをファイルに保存する

```bat
@echo off
set PLC_HOST=192.168.0.1
set PLC_PORT=5007
set LISTEN_PORT=8080
set LOG_FILE=C:\gomc-rest.log

gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT% -log-file %LOG_FILE%
pause
```

ログはコンソールとファイルの両方に出力されます。ディレクトリが存在しない場合はサーバーが起動できないため、事前に作成しておく必要があります。

### リモート操作を有効にする

リモート操作エンドポイント（`/remote/run`・`/remote/stop` など）はデフォルトで無効です。`-enable-remote` を追加して有効にします。

```bat
gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT% -listen %LISTEN_PORT% -enable-remote
```

## トラブルシューティング

| 症状 | 考えられる原因 | 対処 |
| --- | --- | --- |
| `/health` で `{"connected":false}` | PLC が停止しているか IP/ポートが違う | バッチファイルの `PLC_HOST` と `PLC_PORT` を確認 |
| `/write` で `403 forbidden` | `-readonly` で起動している | バッチファイルから `-readonly` を削除 |
| `/remote/*` で `403 forbidden` | `-enable-remote` が設定されていない | バッチファイルに `-enable-remote` を追加 |
| `503 busy` | 同時リクエストが多すぎる | `-queue-size` を増やす（デフォルト: 32） |
| ポートが使用中 | 他のプロセスがポートを使用している | `LISTEN_PORT` を空いているポート番号に変更 |
| ウィンドウがすぐ閉じる | 起動エラー | コマンドプロンプトから実行してエラーを確認 |

## 実行

Windows 用リリースバイナリの場合:

```powershell
.\gomc-rest.exe -host 192.168.0.1 -port 5007 -mode binary -listen :8080
```

ソースからビルドした場合、または Windows 以外の環境の場合:

```bash
./gomc-rest -host 192.168.0.1 -port 5007 -frame 3e -transport tcp -mode binary -queue-size 32 -timeout 5s -listen :8080
```

読み取り専用で運用する場合は `-readonly` を追加します。読み取り専用モードでは `/health` と `/read` は利用でき、`/write` と `/remote/*` の POST 操作は `403 forbidden` になります。

```powershell
.\gomc-rest.exe -host 192.168.0.1 -port 5007 -mode binary -listen 127.0.0.1:8080 -readonly
```

起動時に PLC への接続を試行します。PLC に到達できない場合でも起動は継続し、最初の PLC 要求時に再接続します。

## ソースからビルド

```bash
git clone https://github.com/moge800/gomc-rest
cd gomc-rest
go build -o gomc-rest .
```

## 設定

フラグが優先されます。環境変数は各フラグのデフォルト値として使われます。

| フラグ | 環境変数 | デフォルト | 説明 |
| --- | --- | --- | --- |
| `-host` | `GOMCR_HOST` | `192.168.0.1` | PLC のホスト名または IP アドレス |
| `-port` | `GOMCR_PORT` | `5007` | PLC ポート、`1` から `65535` |
| `-frame` | `GOMCR_FRAME` | `3e` | MC プロトコルフレーム、`3e` または `4e` |
| `-transport` | `GOMCR_TRANSPORT` | `tcp` | `tcp` または `udp`。`4e` は `tcp` のみ対応 |
| `-mode` | `GOMCR_MODE` | `binary` | `binary` または `ascii` |
| `-queue-size` | `GOMCR_QUEUE_SIZE` | `32` | 1 件の実行中要求とは別に待機できる PLC 要求数 |
| `-timeout` | `GOMCR_TIMEOUT` | `5s` | PLC 接続および I/O timeout |
| `-listen` | `GOMCR_LISTEN` | `:8080` | HTTP 待ち受けアドレス |
| `-readonly` | `GOMCR_READONLY` | `false` | `true` のとき `/write` と `/remote/*` の POST 操作を拒否 |
| `-enable-remote` | `GOMCR_ENABLE_REMOTE` | `false` | `true` のとき `/remote/*` エンドポイントを有効化 |
| `-log-file` | `GOMCR_LOG_FILE` | _(なし)_ | ログファイルのパス。指定するとファイルと stderr の両方に出力 |
| `-verbose` | `GOMCR_VERBOSE` | `false` | `true` のとき PLC 操作ごとのデバッグログ（`plc_op` エントリ）を出力 |

## API リファレンス

書き込みとリモート操作が成功すると、次の JSON を返します。

```json
{"ok":true}
```

| Method | Path | パラメータ / body | レスポンス |
| --- | --- | --- | --- |
| `GET` | `/openapi.yaml` | なし | OpenAPI 3.1 仕様書（YAML） |
| `GET` | `/version` | なし | `{"version":"v0.5.0"}` またはローカルビルドでは `{"version":"dev"}` |
| `GET` | `/metrics` | なし | `{"request_count":0,"reconnect_count":0,"plc_error_count":0,"avg_latency_ms":0,"queue_length":0}` |
| `GET` | `/health` | なし | `{"plc_status":"ok","connected":true}` または `{"plc_status":"disconnected","connected":false}` |
| `GET` | `/read` | query: `addr` 必須、`count` は任意でデフォルト `1`、`dword` は任意でデフォルト `false`、`sint` は任意でデフォルト `false` | `{"values":[100,200]}` または `{"values":[true,false]}` |
| `POST` | `/write` | query: `addr` 必須、`dword` は任意でデフォルト `false`、`sint` は任意でデフォルト `false`、body: `{"values":[1,2,3]}` または `{"values":[true,false]}` | `{"ok":true}` |
| `POST` | `/remote/run` | `-enable-remote` が必要。query: `clear=0/1/2` 任意、`force=true/false` 任意 | `{"ok":true}` |
| `POST` | `/remote/stop` | `-enable-remote` が必要。なし | `{"ok":true}` |
| `POST` | `/remote/pause` | `-enable-remote` が必要。query: `force=true/false` 任意 | `{"ok":true}` |
| `POST` | `/remote/latch-clear` | `-enable-remote` が必要。なし | `{"ok":true}` |
| `POST` | `/remote/reset` | `-enable-remote` が必要。なし | `{"ok":true}` |

補足:

- `count` は `1` 以上 `1024` 以下である必要があります。
- `values` は必須で、要素数は `1` 以上 `1024` 以下です。
- `/write` のリクエスト body は 1 MiB 以下である必要があります。
- ワードデバイスには `0..65535` の整数配列、ビットデバイスには真偽値配列を指定します。
- `dword=true` を指定すると、各値は符号なし 32 ビット整数（`0..4294967295`）として扱われます。下位 16 ビットは `addr` のレジスタに、上位 16 ビットは次のレジスタ（`addr+1`）に格納されます。`dword=true` はワードデバイスのみ対応しています。`dword=true` の場合、`count` は `512` 以下、`values` の要素数も `512` 以下にしてください（PLC へ送る語数が `1024` を超えないようにするため）。
- `sint=true` を指定すると、値を符号付き整数として扱います。ワードデバイスは `-32768..32767`、`dword=true` との組み合わせでは `-2147483648..2147483647` の範囲になります。ビットデバイスには使用できません。PLC レジスタのビット列は変わらず、JSON との変換方式のみが変わります。
- `/remote/*` エンドポイントはデフォルトで無効です。`-enable-remote` なしで呼び出すと `403 forbidden` になります。読み取り専用モードとは独立した設定です。
- 読み取り専用モードでは `/write` と `/remote/*` の POST 操作は `403 forbidden` になります。読み取り専用モードは安全補助であり、ネットワーク分離、認証、認可、ファイアウォール、PLC 側保護の代替ではありません。
- ブール型 query フラグ（`dword`、`sint`、`force`）は query 文字列の値が厳密に `true` のときだけ有効です。
- `GET /health` は PLC 未接続時でも常に HTTP `200` を返します。
- `/remote/reset` は PLC 側で TCP 接続が閉じられるため、実行後に接続をクリアします。

## デバイスアドレス

デバイスアドレスは大文字小文字を区別せず、前後の空白も許容します。デバイスプレフィックスによって、ワード読み書きかビット読み書きかを自動判定します。

| 種別 | デバイス | JSON の値 | 例 |
| --- | --- | --- | --- |
| ワード | `D`, `W`, `R`, `ZR`, `TN`, `CN`, `Z`, `SW`, `SD` | 整数 | `D100`, `ZR512`, `SW5` |
| ビット | `X`, `Y`, `M`, `L`, `B`, `F`, `SB`, `SM`, `T`, `C` | 真偽値 | `M0`, `X10`, `T10`, `C5` |

アドレス番号は 0 以上の整数である必要があります。不明なデバイス、番号なし、数値でない番号、負数は `400 bad_request` になります。

## エラーレスポンス

エラーは JSON で返します。すべてのエラー応答は HTTP status code と機械判定用の `code` を body に含めます。

| 状態 | HTTP | `code` | 例 |
| --- | --- | --- | --- |
| パラメータ、body、アドレス、count が不正、または POST 専用エンドポイントで method が不正 | `400` または `405` | `bad_request` | `{"status":400,"error":"addr is required","code":"bad_request"}` |
| `-enable-remote` なしで `/remote/*` を呼び出した | `403` | `forbidden` | `{"status":403,"error":"remote-control operations are disabled (use -enable-remote to enable)","code":"forbidden"}` |
| 読み取り専用モードで拒否された操作 | `403` | `forbidden` | `{"status":403,"error":"operation not allowed in read-only mode","code":"forbidden"}` |
| `/write` の body サイズ超過 | `413` | `bad_request` | `{"status":413,"error":"body must not be larger than 1048576 bytes","code":"bad_request"}` |
| PLC の MC プロトコルエラー、end code あり | `502` | `plc_error` | `{"status":502,"error":"MC error 0x4000","code":"plc_error","end_code":"0x4000"}` |
| PLC 接続エラー | `503` | `connection_error` | `{"status":503,"error":"connect: refused","code":"connection_error"}` |
| PLC 通信キューが満杯 | `503` | `busy` | `{"status":503,"error":"PLC communication queue is full","code":"busy"}` |
| shutdown 中に PLC 通信キューが閉じられた | `503` | `queue_closed` | `{"status":503,"error":"PLC communication queue is closed","code":"queue_closed"}` |
| HTTP request context が完了前にキャンセルされた | `499` | `request_canceled` | `{"status":499,"error":"request canceled","code":"request_canceled"}` |
| HTTP request context の deadline が切れた | `504` | `request_timeout` | `{"status":504,"error":"request timed out","code":"request_timeout"}` |

## 接続の挙動

- PLC 要求は、プロセス内の単一 worker queue と 1 つの共有クライアント接続で直列化されます。
- worker は PLC 要求を常に 1 件ずつ実行します。HTTP handler は PLC client を直接呼びません。
- `-queue-size` は、1 件の実行中要求とは別に待機できる PLC 要求数です。キューが満杯の場合、server は待たせず `Retry-After: 1` 付きの `503 busy` を返します。
- `-timeout` は PLC 接続と I/O deadline です。HTTP request context の timeout とは別物です。キュー待機中に request context がキャンセルされた要求は、通信開始前に実行されません。
- 起動時接続に失敗しても HTTP サーバーは起動します。
- 有効な接続がない場合、次の PLC 要求で再接続を試行します。
- 接続レベルの MC プロトコルエラーが発生した場合、接続をクリアして次回要求で再接続できるようにします。

## 動作確認

`/health` は実機 PLC がなくても利用できます。その他の例は到達可能な PLC が必要です。

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

# リモート操作は起動時に -enable-remote が必要
curl -X POST "http://localhost:8080/remote/run?clear=0&force=false"
curl -X POST "http://localhost:8080/remote/stop"
curl -X POST "http://localhost:8080/remote/pause?force=false"
curl -X POST "http://localhost:8080/remote/latch-clear"
curl -X POST "http://localhost:8080/remote/reset"
```

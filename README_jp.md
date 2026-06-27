# gomc-rest

[English README](README.md)

[![Release](https://img.shields.io/github/v/release/moge800/gomc-rest)](https://github.com/moge800/gomc-rest/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/moge800/gomc-rest/ci.yml?branch=main&label=CI)](https://github.com/moge800/gomc-rest/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux-lightgrey)](#ダウンロード)

`gomc-rest` は、三菱電機 PLC（MC プロトコル 3E / 4E フレーム）向けの小さな REST API サーバー属 HTTP ゲートウェイです。HTTP クライアントと PLC の間に入り、`D100.0`、`W100`、`M0` のようなデバイス文字列を指定した読み書きを仓介しつつ、ワードデバイスは整数、ビットデバイスは真偽値として JSON に自動変換します。

PLC 通信には [gomcprotocol](https://github.com/moge800/gomcprotocol) を使用します。HTTP サーバー部分は Go の標準ライブラリのみで実装されています。

追加依存なしの Python クライアントライブラリもあります: [gomc-rest-client (PyPI)](https://pypi.org/project/gomc-rest-client/)

軽量なデバッグ用 GUI もあります: [gomc-rest-gui](https://github.com/moge800/gomc-rest-gui) — curl の代わりにシンプルな画面から gomc-rest のエンドポイントを操作できる、単体動作の HTTP クライアント（Wails + React）です。

解説記事: [Qiita](https://qiita.com/Moge800/items/7e97a5cfbd76cb111bef)

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

[Releases](https://github.com/moge800/gomc-rest/releases) ページから最新リリースをダウンロードしてください。

| プラットフォーム | ファイル |
| --- | --- |
| Windows (amd64) | `gomc-rest.exe` |
| Linux (amd64) | `gomc-rest-linux-amd64` |
| Linux (arm64 / Raspberry Pi 5) | `gomc-rest-linux-arm64` |

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

以下のように返ってくれれば正常に起動しています。

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

起動時に PLC への接続を試行します。PLC に到達できない場合でも起動は続行し、最初の PLC 要求時に再接続します。

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
| `-log-level` | `GOMCR_LOG_LEVEL` | `info` | ターミナルのログレベル: `debug`、`info`、`warn`、または `error` |
| `-log-file-level` | `GOMCR_LOG_FILE_LEVEL` | `warn` | ファイルへのログレベル: `debug`、`info`、`warn`、または `error`。`-log-file` 指定時のみ有効 |
| _(なし)_ | `GOMCR_TOKEN` | _(なし)_ | 静的ベアラートークン。設定すると `/health` を除く全エンドポイントで `Authorization: Bearer <token>` が必須になります。環境変数のみ（プロセス一覧に出さないため）。 |

## 認証

デフォルトでは認証はありません（閉域ネットワーク用途に合わせた挙動）。
`GOMCR_TOKEN` を設定すると、`/health`（死活監視用）を除く全リクエストで
静的ベアラートークンが必須になります。

サーバ起動時に環境変数として渡します（gomc-rest は `.env` ファイルを読み込みません）:

```bat
REM Windows（バッチ）: gomc-rest.exe を起動する前に設定
set GOMCR_TOKEN=your-shared-secret
gomc-rest.exe -host %PLC_HOST% -port %PLC_PORT%
```

```sh
# Linux / macOS
GOMCR_TOKEN=your-shared-secret ./gomc-rest -host 192.168.0.1 -port 5007
```

```sh
# クライアント側
curl -H "Authorization: Bearer your-shared-secret" "http://localhost:8080/read?addr=D100"
```

トークンが未指定または不一致のリクエストは `401 unauthorized` を返します。

> **注意:** トークンは HTTP 上を平文で流れます。これは閉域 FA ネットワークを
> 前提とした意図的な設計です。通信の暗号化が必要な場合は、gomc-rest の前段に
> TLS 終端のリバースプロキシ（nginx、Caddy など）を置いてください。

## API リファレンス

書き込みとリモート操作が成功すると、次の JSON を返します。

```json
{"ok":true}
```

| Method | Path | パラメータ / body | レスポンス |
| --- | --- | --- | --- |
| `GET` | `/openapi.yaml` | なし | OpenAPI 3.1 仕様書（YAML） |
| `GET` | `/version` | なし | `{"version":"v0.9.0"}` またはローカルビルドでは `{"version":"dev"}` |
| `GET` | `/info` | なし | `{"version":"v0.9.0","gomcprotocol_version":"v0.3.0","host":"192.168.0.1","port":5007,"frame":"3e","transport":"tcp","mode":"binary","listen_addrs":["192.168.1.10:8080"],"readonly":false,"enable_remote":false}` |
| `GET` | `/metrics` | なし | `{"request_count":0,"reconnect_count":0,"plc_error_count":0,"avg_latency_ms":0,"recent_avg_latency_ms":0,"queue_length":0,"client_request_count":0,"busy_count":0,"client_avg_latency_ms":0,"client_recent_avg_latency_ms":0}` |
| `GET` | `/health` | なし | `{"plc_status":"ok","connected":true}` または `{"plc_status":"disconnected","connected":false}` |
| `GET` | `/read` | query: `addr` 必須、`count` は任意でデフォルト `1`、`dword` は任意でデフォルト `false`、`sint` は任意でデフォルト `false` | `{"values":[100,200]}` または `{"values":[true,false]}` |
| `POST` | `/write` | query: `addr` 必須、`dword` は任意でデフォルト `false`、`sint` は任意でデフォルト `false`、body: `{"values":[1,2,3]}` または `{"values":[true,false]}` | `{"ok":true}` |
| `POST` | `/random-read` | body: `{"words":["D100","D200"],"dwords":["D300"]}` | `{"words":[100,200],"dwords":[300]}` |
| `POST` | `/random-write` | body: `{"words":[{"addr":"D100","value":1}],"dwords":[{"addr":"D300","value":65536}],"bits":[{"addr":"M0","value":true}]}` | `{"ok":true}` |
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
- `/random-read` は `words`・`dwords`・`bits`（いずれもアドレス文字列配列）を受け付けます。いずれか非空であれば可。`words`・`dwords` はワードデバイス（`D`、`W`、`R` など）のみ。`bits` はワードデバイスのビットアクセス（例: `D100.1`）とビットデバイス（例: `M0`）に対応します。ワードビットは下位ワードを単一のランダム読み出し（0x0403）に畳み込んでビット抽出、ビットデバイスは MC のランダム読み出しにビット単位が無いため `ReadBits`（0x0401）で1点ずつ読み、1リクエストあたり 16 点までに制限されます（それ以上はワードビットアクセスを利用）。レスポンスは `{"words":[...],"dwords":[...],"bits":[...]}` で、word は整数（`0..65535`）、dword は符号なし 32 ビット整数（`0..4294967295`）、bit はリクエスト順の真偽値です。各配列の上限は 255 件。
- `/random-write` は `words`、`dwords`、`bits` の配列を受け付けます。各要素は `{"addr":"...","value":...}` 形式。`words`・`dwords` にはワードデバイスを指定。`bits` にはビットデバイス（例: `M0`）に加えて、ワードデバイスのビットアクセス（例: `D100.1`）も指定可能で、後者は read-modify-write で適用され、同一ワード内のビットは1回の読み書きにまとめられます。内部で `RandomWrite`（words/dwords）と `RandomWriteBits`（ビットデバイス）を1つの直列ジョブとして実行。各配列の上限は 255 件。
  > **性能上の注意:** ワードデバイスのビットアクセスは MC ネイティブの操作ではありません。`/random-write` の `bits` で触れる**ワードごとに read + write の往復が1回ずつ余分に**かかり、`/random-read` の `bits` のネイティブビットデバイスは**1点ごとに read 往復**がかかります。PLC 往復が約 20ms とすると積み重なって無視できない遅延になるため、**同一ワードのビットは1リクエストにまとめ**、16ビット全てを制御できる場合は通常のワード書き込みを優先してください。同じビット群を複数リクエストに分割すると read-modify-write のコストが倍増します。
- `/remote/*` エンドポイントはデフォルトで無効です。`-enable-remote` なしで呼び出すと `403 forbidden` になります。読み取り専用モードとは独立した設定です。
- 読み取り専用モードでは `/write` と `/remote/*` の POST 操作は `403 forbidden` になります。読み取り専用モードは安全補助であり、ネットワーク分離、認証、認可、ファイアウォール、PLC 側保護の代替ではありません。
- ブール型 query フラグ（`dword`、`sint`、`force`）は query 文字列の値が厳密に `true` のときだけ有効です。
- `GET /health` は PLC 未接続時でも常に HTTP `200` を返します。
- `/remote/reset` は PLC 側で TCP 接続が閉じられるため、実行後に接続をクリアします。
- `/metrics` の PLC 系フィールド（`request_count`、`avg_latency_ms`、`recent_avg_latency_ms`）は PLC との通信時間のみを計測します。クライアント系フィールド（`client_request_count`、`client_avg_latency_ms`、`client_recent_avg_latency_ms`）はキュー待ち時間を含むクライアント視点の往復時間を計測します。`busy_count` はキュー満杯で弾かれたリクエスト数で、クライアントレイテンシーの平均には含まない。

## デバイスアドレス

デバイスアドレスは大文字小文字を区別せず、前後の空白も許容します。デバイスプレフィックスによって、ワード読み書きかビット読み書きかを自動判定します。

| 種別 | デバイス | JSON の値 | 例 |
| --- | --- | --- | --- |
| ワード | `D`, `W`, `R`, `ZR`, `TN`, `STN`, `CN`, `Z`, `SW`, `SD` | 整数 | `D100`, `ZR512`, `TN10`, `CN5` |
| ビット | `X`, `Y`, `M`, `L`, `B`, `F`, `V`, `SB`, `SM`, `S`, `DX`, `DY`, `TC`, `TS`, `STC`, `STS`, `CC`, `CS` | 真偽値 | `M0`, `TC10`, `CC5`, `STC3` |

タイマ・カウンタの接点・コイルは 2 文字プレフィックスで指定します: `TC`（タイマ接点）、`TS`（タイマコイル）、`CC`（カウンタ接点）、`CS`（カウンタコイル）。単一文字の `T` や `C` は有効なデバイス名ではなく `400 bad_request` になります。

アドレス番号は 0 以上の整数である必要があります。不明なデバイス、番号なし、数値でない番号、負数は `400 bad_request` になります。`X`・`Y`・`B`・`SB`・`W`・`SW`・`ZR`・`DX`・`DY` のアドレス番号は 16 進数で指定します（例: `X4F`, `Y12D2`, `W1D`）。

### ワードデバイスのビット単位アクセス

ワードデバイスのアドレスに `.N`（16進1桁、`0`〜`F`）を付けると、第 `N` ビットから連続する 1 ビット以上を読み書きできます。

```
D100.0   ← D100 の第 0 ビット（最下位）
D100.F   ← D100 の第 15 ビット（最上位）
W1D.7     ← W1D（16進アドレス 0x1D）の第 7 ビット
```

- 読み取り: `count` を付けると第 `N` ビットから連続するビットを返します。`GET /read?addr=D100.1&count=5` は D100 の第 1〜5 ビットを `{"values": [true, false, ...]}` として返します。
- 書き込み: body は真偽値の配列で、第 `N` ビットから順に適用します。`{"values": [true, false, true]}` は第 `N`・`N+1`・`N+2` ビットを設定します。内部で 1 回の read-modify-write を実行します。
- ビットは同一ワード内に収める必要があります。`N + count`（書き込みは `N + 要素数`）が `16` を超えると `400 bad_request` になります。例えば `D100.F&count=2` は拒否されます。
- `dword=true` または `sint=true` との組み合わせは `400 bad_request` になります。
- ビットデバイス（`X`、`M` など）に `.N` を付けると `400 bad_request` になります。

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

# ランダム読み書き — 非連続アドレスを1リクエストで処理
curl -X POST "http://localhost:8080/random-read" \
  -H "Content-Type: application/json" \
  -d '{"words":["D100","D200"],"dwords":["D300"]}'

curl -X POST "http://localhost:8080/random-write" \
  -H "Content-Type: application/json" \
  -d '{"words":[{"addr":"D100","value":10},{"addr":"D200","value":20}],"bits":[{"addr":"M0","value":true}]}'

# リモート操作は起動時に -enable-remote が必要
curl -X POST "http://localhost:8080/remote/run?clear=0&force=false"
curl -X POST "http://localhost:8080/remote/stop"
curl -X POST "http://localhost:8080/remote/pause?force=false"
curl -X POST "http://localhost:8080/remote/latch-clear"
curl -X POST "http://localhost:8080/remote/reset"
```

# gomc-rest

[English README](README.md)

`gomc-rest` は、三菱電機 PLC（MC プロトコル 3E フレーム）向けの小さな REST API サーバーです。HTTP クライアントから `D100` や `M0` のようなデバイス文字列を指定して読み書きでき、ワードデバイスは整数、ビットデバイスは真偽値として JSON に自動変換します。

PLC 通信には [gomcprotocol](https://github.com/moge800/gomcprotocol) を使用します。HTTP サーバー部分は Go の標準ライブラリのみで実装されています。

## 機能

- `/read` でワードデバイスとビットデバイスを読み取り。
- `/write` で整数配列または真偽値配列を書き込み。
- RemoteRun、RemoteStop、RemotePause、RemoteLatchClear、RemoteReset に対応。
- コマンドラインフラグで設定し、環境変数をデフォルト値として利用。
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

## 実行

Windows 用リリースバイナリの場合:

```powershell
.\gomc-rest.exe -host 192.168.0.1 -port 5007 -mode binary -listen :8080
```

ソースからビルドした場合、または Windows 以外の環境の場合:

```bash
./gomc-rest -host 192.168.0.1 -port 5007 -mode binary -listen :8080
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
| `-host` | `PLC_HOST` | `192.168.0.1` | PLC のホスト名または IP アドレス |
| `-port` | `PLC_PORT` | `5007` | PLC ポート、`1` から `65535` |
| `-mode` | `PLC_MODE` | `binary` | `binary` または `ascii` |
| `-listen` | `LISTEN_ADDR` | `:8080` | HTTP 待ち受けアドレス |

## API リファレンス

書き込みとリモート操作が成功すると、次の JSON を返します。

```json
{"ok":true}
```

| Method | Path | パラメータ / body | レスポンス |
| --- | --- | --- | --- |
| `GET` 推奨、未強制 | `/health` | なし | `{"status":"ok","connected":true}` または `{"status":"disconnected","connected":false}` |
| `GET` | `/read` | query: `addr` 必須、`count` は任意でデフォルト `1`、`dword` は任意でデフォルト `false` | `{"values":[100,200]}` または `{"values":[true,false]}` |
| `POST` | `/write` | query: `addr` 必須、`dword` は任意でデフォルト `false`、body: `{"values":[1,2,3]}` または `{"values":[true,false]}` | `{"ok":true}` |
| `POST` | `/remote/run` | query: `clear=0/1/2` 任意、`force=true/false` 任意 | `{"ok":true}` |
| `POST` | `/remote/stop` | なし | `{"ok":true}` |
| `POST` | `/remote/pause` | query: `force=true/false` 任意 | `{"ok":true}` |
| `POST` | `/remote/latch-clear` | なし | `{"ok":true}` |
| `POST` | `/remote/reset` | なし | `{"ok":true}` |

補足:

- `count` は `1` 以上 `1024` 以下である必要があります。
- `values` は必須で、要素数は `1` 以上 `1024` 以下です。
- `/write` のリクエスト body は 1 MiB 以下である必要があります。
- ワードデバイスには `0..65535` の整数配列、ビットデバイスには真偽値配列を指定します。
- `dword=true` を指定すると、各値は 32 ビット整数として扱われます。下位 16 ビットは `addr` のレジスタに、上位 16 ビットは次のレジスタ（`addr+1`）に格納されます。`dword=true` はワードデバイスのみ対応しています。`dword=true` の場合、`count` は `512` 以下、`values` の要素数も `512` 以下にしてください（PLC へ送る語数が `1024` を超えないようにするため）。
- `force` は query の値が厳密に `true` のときだけ有効です。
- `/health` は PLC 未接続時でも常に HTTP `200` を返します。
- `/remote/reset` は PLC 側で TCP 接続が閉じられるため、実行後に接続をクリアします。

## デバイスアドレス

デバイスアドレスは大文字小文字を区別せず、前後の空白も許容します。デバイスプレフィックスによって、ワード読み書きかビット読み書きかを自動判定します。

| 種別 | デバイス | JSON の値 | 例 |
| --- | --- | --- | --- |
| ワード | `D`, `W`, `R`, `ZR`, `TN`, `CN`, `Z`, `SW`, `SD` | 整数 | `D100`, `ZR512`, `SW5` |
| ビット | `X`, `Y`, `M`, `L`, `B`, `F`, `SB`, `SM` | 真偽値 | `M0`, `X10`, `SB10` |

アドレス番号は 0 以上の整数である必要があります。不明なデバイス、番号なし、数値でない番号、負数は `400 bad_request` になります。

## エラーレスポンス

エラーは JSON で返します。

| 状態 | HTTP | `code` | 例 |
| --- | --- | --- | --- |
| パラメータ、body、アドレス、count が不正、または POST 専用エンドポイントで method が不正 | `400` または `405` | `bad_request` | `{"error":"addr is required","code":"bad_request"}` |
| `/write` の body サイズ超過 | `413` | `bad_request` | `{"error":"body must not be larger than 1048576 bytes","code":"bad_request"}` |
| PLC の MC プロトコルエラー、end code あり | `502` | `plc_error` | `{"error":"MC error 0x4000","code":"plc_error","end_code":"0x4000"}` |
| PLC 接続エラー | `503` | `connection_error` | `{"error":"connect: refused","code":"connection_error"}` |

## 接続の挙動

- PLC 要求は 1 つの共有クライアント接続で直列化されます。
- MC プロトコルクライアントのタイムアウトは 5 秒です。
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

curl -X POST "http://localhost:8080/write?addr=D100" \
  -H "Content-Type: application/json" \
  -d '{"values":[10,20,30]}'

curl -X POST "http://localhost:8080/write?addr=D100&dword=true" \
  -H "Content-Type: application/json" \
  -d '{"values":[100000,200000]}'

curl -X POST "http://localhost:8080/write?addr=M0" \
  -H "Content-Type: application/json" \
  -d '{"values":[true,false]}'

curl -X POST "http://localhost:8080/remote/run?clear=0&force=false"
curl -X POST "http://localhost:8080/remote/stop"
curl -X POST "http://localhost:8080/remote/pause?force=false"
curl -X POST "http://localhost:8080/remote/latch-clear"
curl -X POST "http://localhost:8080/remote/reset"
```

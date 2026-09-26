---
name: golang-lint-governance
description: "Go (Golang) における golangci-lint（errcheck, gofmt, gosec, govet, paralleltest, staticcheck, testifylint, unused 等）の完全準拠、自動修正、およびコード変更時に常に lint エラー 0 を維持・遵守するための静的解析・品質ガバナンスプラクティス。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, Antigravity, OpenCode, OpenClaw, and other AI coding agents.
metadata:
  author: [YOUR_NAME]
  version: "1.0.0"
  openclaw:
    emoji: "🔍"
    requires:
      bins:
        - go
        - golangci-lint
        - make
    install: []
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(make:*) Bash(golangci-lint:*) Agent
---

> [!IMPORTANT]
> **ゼロ容認ガバナンス (Zero-Lint-Tolerance Principle):**
> コードの新規作成、機能追加、バグ修正、リファクタリングを行った後は、タスク完了前に**必ず `make lint` を実行し、全 linter の指摘事項が 0 件（`0 issues`）であることを確認しなければなりません**。
> `nolint` ディレクティブは安易に乱用せず、仕様上の正当な理由（プロトコル固有の型キャストやテスト用エンドポイント等）がある場合にのみ、明確な理由コメントを添えて付与します。

**Persona:** あなたは **Go 静的解析およびコード品質ガバナンス（Lint Governance）エンジニア** です。すべての Go コードに対して厳格な静的解析基準を適用し、リソースリーク、データレース、ロック値コピー、セキュリティ脆弱性、並行テスト不備をゼロに抑え込む責任を持ちます。

---

# Go Lint Governance & Compliance Guide

## 1. 主要 Linter ルールと具体的対応指針

### 1.1 `errcheck`（エラーおよびリソース解放漏れの防止）
関数の戻り値である `error` を未検証のまま放置したり、リソースクローズ処理の戻り値を無視してはなりません。

- **`defer Close()` パターン**:
  `defer x.Close()` は戻り値チェック漏れ（errcheck）の典型例です。必ず無名関数でラップして明示的に処理します:
  ```go
  // ❌ NG (errcheck 警告)
  defer file.Close()
  defer resp.Body.Close()
  defer svc.Close()

  // ✅ OK
  defer func() { _ = file.Close() }()
  defer func() { _ = resp.Body.Close() }()
  defer func() { _ = svc.Close() }()
  ```
- **インラインクローズ**:
  ループ内や終了処理で直接 `Close()` を呼ぶ場合も、戻り値をログ出力するか明示的に無視します:
  ```go
  // ✅ OK
  if err := conn.Close(); err != nil {
      slog.Warn("Failed to close connection", "error", err)
  }
  // または破棄が前提の場合
  _ = rtpSession.Close()
  ```

---

### 1.2 `govet`（Go 標準静的解析: ロックコピー & シャドウイング）

- **`copylocks`（ロック値のコピー防止）**:
  `sync.Mutex` や `sync.RWMutex` をフィールドに持つ構造体を、値渡し（戻り値、関数の値引数、`json.Marshal`、`template.Execute` 等）してはなりません。ロックの状態がコピーされ、デッドロックやレースの原因になります。
  ```go
  // ❌ NG: StreamStats が sync.RWMutex を内包している場合、*s のコピーは copylocks 違反
  func (s *StreamStats) GetStats() StreamStats {
      s.mu.RLock()
      defer s.mu.RUnlock()
      return *s
  }

  // ✅ OK: 読み取り専用スナップショット構造体（Mutex を持たない）を分離定義する
  type StreamStatsSnapshot struct {
      PacketsReceived uint64  `json:"packets_received"`
      LossRate        float64 `json:"loss_rate"`
  }

  func (s *StreamStats) Snapshot() StreamStatsSnapshot {
      s.mu.RLock()
      defer s.mu.RUnlock()
      return StreamStatsSnapshot{
          PacketsReceived: s.packetsReceived,
          LossRate:        s.lossRate,
      }
  }
  ```

- **`shadow`（変数のシャドウイング防止）**:
  外側のスコープで宣言された変数（特に `err` や `resp`）を、内側のブロックで `:=` を用いて再宣言してはなりません:
  ```go
  // ❌ NG: 外側の err をシャドウイング
  func Handle(ctx context.Context) error {
      data, err := readData()
      if err != nil { return err }
      if condition {
          result, err := process(data) // shadow 警告
          _ = result
      }
      return err
  }

  // ✅ OK: 固有の名前（procErr, mErr 等）を使用するか代入（=）にする
  func Handle(ctx context.Context) error {
      data, err := readData()
      if err != nil { return err }
      if condition {
          result, procErr := process(data)
          if procErr != nil { return procErr }
          _ = result
      }
      return nil
  }
  ```

---

### 1.3 `gosec`（セキュリティ脆弱性チェック）

- **G112 (Slowloris 対策)**:
  `http.Server` には必ず `ReadHeaderTimeout` を設定します:
  ```go
  // ✅ OK
  server := &http.Server{
      Addr:              addr,
      Handler:           mux,
      ReadHeaderTimeout: 5 * time.Second, // 必須
  }
  ```

- **G120 (無制限マルチパートフォーム解析の防止)**:
  `r.ParseMultipartForm` を呼び出す前に、必ず `http.MaxBytesReader` でリクエストボディの上限を制限します:
  ```go
  // ✅ OK
  r.Body = http.MaxBytesReader(rw, r.Body, 32<<20)
  //nolint:gosec // G120: request body bounded by http.MaxBytesReader
  if err := r.ParseMultipartForm(32 << 20); err != nil {
      http.Error(rw, err.Error(), http.StatusBadRequest)
      return
  }
  ```

- **G301 / G306 (ファイル・ディレクトリ権限)**:
  ディレクトリ作成パーミッションは `0o750` 以下、ファイル作成パーミッションは `0o600` 以下に設定します:
  ```go
  // ❌ NG
  os.MkdirAll(dir, 0o755) // G301
  os.WriteFile(path, data, 0o644) // G306

  // ✅ OK
  os.MkdirAll(dir, 0o750)
  os.WriteFile(path, data, 0o600)
  ```

- **G304 / G705 (パストラバーサル & XSS/MIME インジェクション)**:
  動的パスには `filepath.Clean(path)` を適用し、バイナリデータ（WAV 音声等）を直接 HTTP 応答に書き込む場合は `X-Content-Type-Options: nosniff` を付与します:
  ```go
  // ✅ OK
  cleanPath := filepath.Clean(filePath)
  //nolint:gosec // G304: validated file path
  data, err := os.ReadFile(cleanPath)

  rw.Header().Set("Content-Type", "audio/wav")
  rw.Header().Set("X-Content-Type-Options", "nosniff")
  //nolint:gosec // G705: raw binary WAV data
  _, _ = rw.Write(data)
  ```

- **G115 (整数型キャストのオーバーフロー防止)**:
  プロトコル仕様（RTP ヘッダ、G.711 PCM サンプル、WAV ヘッダ等）に基づく意図的な型変換（`int -> uint32`, `uint16 -> int16`, `int -> uint8`）を行う場合は、安全な範囲チェックを行うか、仕様根拠を明記して nolint を付与します:
  ```go
  // ✅ OK: 境界チェック + 根拠コメント
  if pt, err := strconv.Atoi(str); err == nil && pt >= 0 && pt <= 127 {
      //nolint:gosec // G115: standard RTP payload type (0-127)
      info.PayloadType = uint8(pt)
  }

  // ✅ OK: RTP シーケンス番号の wrap-around 差分計算（RFC 3550）
  //nolint:gosec // G115: standard RTP sequence number modular comparison
  diff := int16(h[i].SequenceNumber - h[j].SequenceNumber)
  ```

- **G107 (変数 URL による HTTP リクエスト)**:
  テストコード等で動的に生成したポートや URL にリクエストを送る場合は、テスト用途であることを明記します:
  ```go
  //nolint:gosec // G107: test endpoint URL
  resp, err := http.Get(testURL)
  ```

---

### 1.4 `paralleltest`（テストの並行実行と独立性）

- **テスト関数での `t.Parallel()` 呼び出し**:
  すべての単体テスト関数の先頭で `t.Parallel()` を呼び出します。
- **テーブル駆動テストでの変数コピー**:
  サブテスト（`t.Run`）を実行する前に、ループ変数 `tt := tt` を必ずコピーしてからサブテスト内で `t.Parallel()` を呼び出します:
  ```go
  func TestSomething(t *testing.T) {
      t.Parallel()
      tests := []struct{ name string }{ ... }

      for _, tt := range tests {
          tt := tt // ループ変数のキャプチャ
          t.Run(tt.name, func(t *testing.T) {
              t.Parallel()
              // 検証処理
          })
      }
  }
  ```
- **逐次実行が必要な E2E シナリオテストの扱い**:
  共通のサーバインスタンスに対して接続→送話→切断のライフサイクルを順番に検証するテストでは、サブテストの並行化が不可能な場合があります。その場合は親テスト関数に明確な理由を添えて nolint を付記します:
  ```go
  //nolint:paralleltest // Sequential E2E lifecycle test scenarios against shared server instance
  func TestE2E_FullStack(t *testing.T) { ... }
  ```

---

### 1.5 `gofmt` & `staticcheck` & `testifylint` & `unused`

- **`gofmt`**:
  コードを保存・変更した後は、常に `go fmt ./...` を実行してインデントやタグ位置を揃えます。
- **`staticcheck` (QF1003)**:
  同一変数に対する連続した `if-else` 分岐は、タグ付き `switch` 文へリファクタリングします:
  ```go
  // ❌ NG
  if ptt == "priority" { ... } else if ptt == "emergency" { ... }
  // ✅ OK
  switch ptt {
  case "priority": ...
  case "emergency": ...
  default: ...
  }
  ```
- **`testifylint`**:
  - エラーの検証: `assert.ErrorIs` ではなく、テストを即時停止できる `require.ErrorIs(t, err, expectedErr)` を使用。
  - 浮動小数点数の比較: 丸め誤差を防ぐため `assert.InDelta(t, expected, actual, delta)` を使用。
  - ゼロ超過の比較: `assert.Greater(t, v, 0)` ではなく `assert.Positive(t, v)` を使用。
- **`unused`**:
  未使用の構造体フィールド、関数引数、未参照の定数・プライベート関数は残さず削除します。

---

## 2. 陷りがちな罠（Traps）とチェックリスト

| 罠 (Trap) | 発生する Linter 警告 | 回避策 |
| :--- | :--- | :--- |
| `defer resp.Body.Close()` と 1 行で書く | `errcheck` | `defer func() { _ = resp.Body.Close() }()` と書く |
| Mutex を持つ構造体を戻り値 `return *s` で返す | `govet (copylocks)` | Mutex を含まない Snapshot 構造体を定義して返す |
| `http.Server` にタイムアウトを書かない | `gosec (G112)` | `ReadHeaderTimeout: 5 * time.Second` を設定する |
| ディレクトリを `0o755`、ファイルを `0o644` で作る | `gosec (G301, G306)` | ディレクトリは `0o750`、ファイルは `0o600` で作成する |
| `for _, tt := range tests` で `tt := tt` を忘れる | `paralleltest` | ループ先頭で `tt := tt` を行い `t.Run` 内で `t.Parallel()` を呼ぶ |
| `assert.ErrorIs(t, err, target)` を使う | `testifylint` | `require.ErrorIs(t, err, target)` に置き換える |
| 浮動小数点を `assert.Equal(t, 1.0, val)` で比較する | `testifylint` | `assert.InDelta(t, 1.0, val, 0.001)` を使用する |

---

## 3. エージェント標準自己検証ワークフロー (Self-Verification Workflow)

コード変更・機能追加・リファクタリングを完了とする前に、以下の順序でコマンドを実行し、全チェックが PASS することを確認します:

```bash
# 1. コードフォーマットの統一
go fmt ./...

# 2. 静的解析チェック（エラーゼロを確認）
make lint

# 3. 単体テスト・データレース検証（カバレッジ 80% 以上を維持）
make test

# 4. ライセンスヘッダー整合性検証
make license-check

# 5. スキル定義フロントマター検証
make check
```

# QA エビデンスレポート: 外部オンプレミスOSS（Musubi）デプロイ & ダッシュボード稼働確認 E2E シナリオテスト

- **実施日時**: 2026-10-19 10:33 JST
- **対象プロダクト**: `ytagarasu (ytg)` v0.1.0
- **検証対象外部OSS**: `Musubi` (Air-Gapped Standalone Network Orchestration & Telemetry Gateway)
  - リポジトリ: https://github.com/sh0jitmy/Musubi
  - 対象コンポーネント: `musubi-server` (Core API & SNMP Collector), `musubi-web` (Docker-Free HTMX Dashboard)
- **テスト自動化コード**: [test/e2e/musubi_scenario_test.go](file:///Users/shjtmy/gravity/ytagarasu/test/e2e/musubi_scenario_test.go)
- **マニフェスト定義**: [examples/musubi/manifest.yaml](file:///Users/shjtmy/gravity/ytagarasu/examples/musubi/manifest.yaml)
- **CI ワークフロー**: [.github/workflows/musubi-scenario-test.yml](file:///Users/shjtmy/gravity/ytagarasu/.github/workflows/musubi-scenario-test.yml)
- **合否判定**: **合格 (ALL PASSED)**

---

## 1. テスト概要と目的

オンプレミス・閉域網（エアギャップ）環境における実践的ユースケースとして、外部OSSである「Musubi」を ytagarasu のマニフェスト定義（`manifest.yaml`）によってデプロイし、以下を一気通貫で検証する：
1. **マニフェスト真正性の暗号署名・検証 (Ed25519)**
2. **`ytg supervisor` による複数プロセスの協調看取り起動** (`musubi-server` + `musubi-web`)
3. **プロセス健全性ポーリング (`health_endpoint`) の自動待機**
4. **Musubi Web 統合監視ダッシュボード (`http://localhost:13001/`) の描画確認（HTTP 200 OK & 主要UIコンポーネントの存在確認）**
5. **Supervisor による安全なプロセスの Graceful Shutdown**

---

## 2. デプロイメントマニフェスト仕様

```yaml
version: "1.0"
metadata:
  name: "musubi-autonomous-network"
  description: "Musubi Standalone Network Orchestration & Telemetry Dashboard"
  environment: "air-gapped-onpremise"

processes:
  - name: musubi-server
    binary: ./bin/musubi-server
    work_dir: .
    env:
      PORT: "18080"
      SNMP_TRAP_PORT: "10162"
      DATABASE_DRIVER: "sqlite3"
      DATABASE_DSN: "musubi.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
      BACKUP_ENABLED: "false"
    auto_restart: true
    max_retries: 5
    listen_port: 18080
    health_endpoint: "http://127.0.0.1:18080/v1/system/healthz"
    rollback_on_error: true
    escape_hatch_mode: true
    log_path: "./logs/musubi-server.log"
    log_max_size_mb: 50
    log_max_backups: 5
    log_compress: true

  - name: musubi-web
    binary: ./bin/musubi-web
    work_dir: .
    env:
      PORT: "13001"
      MUSUBI_API_URL: "http://127.0.0.1:18080"
    auto_restart: true
    max_retries: 5
    listen_port: 13001
    health_endpoint: "http://127.0.0.1:13001/healthz"
    rollback_on_error: true
    escape_hatch_mode: true
    log_path: "./logs/musubi-web.log"
    log_max_size_mb: 50
    log_max_backups: 5
    log_compress: true
```

---

## 3. 実機テスト実行結果ログ

```text
=== RUN   TestMusubi_DeploymentAndDashboardScenario
=== PAUSE TestMusubi_DeploymentAndDashboardScenario
=== CONT  TestMusubi_DeploymentAndDashboardScenario
    musubi_scenario_test.go:276: Musubi Dashboard successfully verified and HTML snapshot saved to /var/folders/zy/jng03j6n2tx0s5s_q1xwy7fc0000gn/T/TestMusubi_DeploymentAndDashboardScenario2704111044/001/musubi-dashboard-snapshot.html (size: 11344 bytes)
--- PASS: TestMusubi_DeploymentAndDashboardScenario (0.18s)
PASS
ok  	github.com/sh0jitmy/ytagarasu/test/e2e	0.446s
```

---

## 4. Musubi ダッシュボード描画確認結果 (HTTP レスポンス)

- **リクエスト**: `GET http://127.0.0.1:13001/`
- **ステータス**: `HTTP/1.1 200 OK`
- **Content-Type**: `text/html; charset=utf-8`
- **HTML サイズ**: 11,344 bytes
- **検出された必須 UI コンポーネント**:
  - `Musubi Dashboard` (ブランドヘッダー)
  - `Air-Gapped & Standalone Network Orchestration` (サブタイトル)
  - `🖥️ 統合監視ダッシュボード` (メインセクション)
  - `⚙️ システムリソース & SNMP帯域 (Process CPU / Memory / Throughput / Goroutines)` (リアルタイムメトリクス)
  - `🎯 ターゲットインベントリ & 死活ステータス` (監視対象ノード一覧)
  - `📡 最新MIBテレメトリ (Trap / Inform / BulkGet)` (テレメトリテーブル)
  - `⚡ シナリオジョブ実行履歴 & 実行判定` (自動化シナリオ実行状況)
  - `🛡️ API & 管理者操作監査ログ (Audit Trail)` (改ざん不能監査ログ)
  - `hx-get="/ui/components/system-metrics"` (HTMX による 5 秒ごとの自動ポーリングディレクティブ)

---

## 5. 結論

ytagarasu の宣言的マニフェストによって、外部オンプレミス OSS（Musubi）のマルチプロセス構成が確実に起動・看取りされ、暗号署名による改ざん検証、ポートバインド、ヘルスチェック連携、および Web ダッシュボードの完全な描画が実機上で確認された。
本検証はユニットモックではなく、実バイナリを用いた完全な E2E シナリオテストとしてパスした。

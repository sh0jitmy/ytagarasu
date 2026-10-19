# エアギャップ（完全閉域網）USB可搬媒体書き出し・検証・デプロイ仕様書 & ユースケース集
**Document ID**: SPEC-AIRGAP-USB-2026-001  
**Target Product**: ytagarasu (v0.8.0+) / Musubi Integration  
**Classification**: On-Premise SRE / QA Audit Standard / Security Architecture  
**Status**: APPROVED / IN-PRODUCTION  

---

## 1. エグゼクティブサマリー & 背景 (Executive Summary & Background)

### 1.1 オンプレミス閉域網における物理的分離の現実
金融勘定系、防衛・セキュリティ基盤、医療機器ネットワーク、工場OT（Operational Technology）制御盤などの現場では、**外部インターネットへの物理的・論理的接続が完全に遮断（Air-Gapped）** されています。
一般的なクラウドネイティブツール（Terraform、Helm、Docker Compose、Ansible等）は、パブリックなリポジトリ（Docker Hub、GitHub、APT/DNFミラー、npm等）からの動的ダウンロードを暗黙の前提としており、完全閉域網ではそのまま動作しません。

### 1.2 「1度媒体用に書き出す動作」の必然性
閉域網へのソフトウェア導入では、以下の2つの隔離環境を行き来する二段階運用が不可避です：
1. **準備端末（オンライン / 準オンライン環境）**:
   - 外部からバイナリ、コンテナイメージ、OSパッケージを取得可能な端末。
   - ここで必要な全アセットを収集し、暗号署名を付与して **USBメモリやポータブルSSDなどの可搬物理媒体へ書き出す（エクスポート）**。
2. **本番ホスト（完全エアギャップ環境）**:
   - 外部接続が一切禁止された現場サーバー。
   - 持ち込まれたUSB媒体を接続し、**媒体上のデータのみを用いて暗号完全性を検証し、自律デプロイと看取り起動を完結** させる。

ytagarasuは、この「媒体書き出し ➔ 物理搬送 ➔ 現場での検証・オフラインデプロイ・プロセス看取り」の一連のライフサイクルを単一のマニフェスト（`manifest.yaml`）とCLI（`ytg`）で完全自動化します。

---

## 2. システムアーキテクチャ & 媒体データ構造 (Architecture & Media Layout)

### 2.1 全体アーキテクチャ概念図

```text
[ ステージング環境 (オンライン) ]
  │
  ├─► Musubi Core & Web ソース/バイナリのビルド
  ├─► docker pull & docker save (postgres:16-alpine, grafana:10.4.0)
  ├─► ytagarasu manifest 生成 & Ed25519 暗号署名 (manifest.sig)
  └─► export-airgap-bundle.sh による可搬媒体出力
            │
            ▼ 【物理搬送: USBメモリ / 暗号化ポータブルSSD】
            │
[ 本番現場ホスト (完全エアギャップ環境: インターネット遮断) ]
  │
  ├─► 1. ytg bundle verify /mnt/usb/bundle-*.tar.gz (Ed25519 & SHA-256 改ざん検知)
  ├─► 2. docker load -i /mnt/usb/images/*.tar (外部レジストリ通信ゼロでオフライン登録)
  ├─► 3. ytg supervisor run -c /mnt/usb/manifest.yaml (看取りデーモン起動)
  │        │
  │        ├─► postgres-container  (PostgreSQL 16 :5432)
  │        ├─► grafana-container   (Grafana :3000)
  │        ├─► musubi-server       (Core API :18080)
  │        └─► musubi-web          (HTMX Dashboard :13001)
  │
  └─► 4. 現場運用端末からブラウザアクセス (Musubi Web & Grafana)
```

### 2.2 USB可搬媒体内のディレクトリ構成

USB媒体（マウント先: `/mnt/usb/musubi-bundle` 等）には、外部アクセスなしで完全自立起動するための全成果物が自己完結形式で格納されます。

```text
/Volumes/USB_STICK/musubi-bundle/
├── manifest.yaml                                # オフラインDockerハイブリッド看取りマニフェスト
├── bundle-musubi-hybrid-airgap-1.0.tar.gz       # 配布用単一アーカイブ (暗号検証対象)
├── checksums                                    # 全アセットの SHA-256 チェックサムリスト
├── signatures/
│   ├── manifest.sig                             # Ed25519 デジタル署名
│   └── public.key                               # 署名検証用 Ed25519 公開鍵 (hex)
├── artifacts/
│   ├── musubi-server                            # Musubi Core Server バイナリ (ELF x86_64 / Mach-O)
│   └── musubi-web                               # Musubi HTMX Web Dashboard バイナリ
├── images/
│   ├── postgres.tar                             # PostgreSQL 16 Alpine コンテナイメージ (docker save)
│   └── grafana.tar                              # Grafana 10.4.0 コンテナイメージ (docker save)
└── configs/
    └── musubi.yaml                              # エアギャップ環境用ランタイム設定ファイル
```

---

## 3. 実践運用ユースケース仕様 (Use Cases Specification)

### UC-01: 準備端末でのUSB媒体書き出し (Staging & Media Export Pipeline)
- **アクター**: リリースエンジニア / SRE担当者
- **事前条件**: 準備端末がインターネットに接続可能であり、Docker CLI および Go コンパイラが利用可能であること。
- **実行手順**:
  ```bash
  # USBマウント先を指定してエクスポートスクリプトを実行
  ./examples/musubi/export-airgap-bundle.sh /Volumes/USB_STICK/musubi-bundle
  ```
- **内部処理**:
  1. `artifacts/` に `musubi-server` および `musubi-web` の最新バイナリを配置。
  2. `docker save postgres:16-alpine -o images/postgres.tar` および `docker save grafana/grafana:10.4.0 -o images/grafana.tar` を実行。
  3. `manifest-airgap-docker.yaml` を `manifest.yaml` として媒体ルートに配置。
  4. Ed25519 署名鍵を用いて `signatures/manifest.sig` を生成。
  5. 各ファイルの SHA-256 ハッシュを走査し `checksums` を出力。
  6. 一時領域で `.tar.gz` を生成後、媒体直下へ `bundle-musubi-hybrid-airgap-1.0.tar.gz` として配置（GNU tarの走査中更新エラーを防止）。
- **事後条件**: 外部接続が一切不要な自己完結型USB媒体が確定する。

---

### UC-02: 物理搬送 & サプライチェーンセキュリティ検査
- **アクター**: セキュリティ管理者 / 搬送担当者
- **概要**: 媒体搬送途上での悪意ある差し替えやファイル改ざんを防止。
- **検証基準**:
  - 媒体書き出し時に生成された公開鍵（`signatures/public.key`）のフィンガープリント、または事前共有された信頼済み公開鍵と照合。
  - 1ビットでも改ざんや破損が生じた場合、現場ホストでのインポートが拒否される。

---

### UC-03: 完全閉域網ホストでの暗号検証 (Integrity Verification)
- **アクター**: 現場オンプレミス保守運用者
- **事前条件**: 本番ホストが外部ネットワークから完全に遮断されていること。USB媒体が `/mnt/usb` にマウントされていること。
- **実行手順**:
  ```bash
  ytg bundle verify /mnt/usb/musubi-bundle/bundle-musubi-hybrid-airgap-1.0.tar.gz
  ```
- **期待結果**:
  - `Signature: VALID (Ed25519 verified)`
  - `Checksums: VALID (All files SHA-256 match)`
  - `Status: VERIFIED`
- **改ざん検知テスト**:
  - アーカイブ内の1バイトを改変した不正アーカイブを与えた場合、`tampered archive rejected` として処理が中断されること。

---

### UC-04: オフラインコンテナ登録 (`docker load -i`) & ネイティブ配置
- **アクター**: ytagarasu エージェント (`ytagarasu-agent`) または手動コマンド
- **実行手順**:
  ```bash
  docker load -i /mnt/usb/musubi-bundle/images/postgres.tar
  docker load -i /mnt/usb/musubi-bundle/images/grafana.tar
  ```
- **期待結果**:
  - Docker Hub や外部レジストリへの `docker pull` 通信が **0件** であること。
  - ローカルの Docker イメージキャッシュに `postgres:16-alpine` および `grafana/grafana:10.4.0` が即座に登録されること。

---

### UC-05: ytagarasu supervisor による看取り起動 & 死活監視
- **アクター**: 現場オンプレミス保守運用者
- **実行手順**:
  ```bash
  ytg supervisor run -c /mnt/usb/musubi-bundle/manifest.yaml
  ```
- **看取り対象プロセス**:
  1. `postgres-container`: `docker run ... postgres:16-alpine`（ポート 5432）
  2. `grafana-container`: `docker run ... grafana/grafana:10.4.0`（ポート 3000）
  3. `musubi-server`: `/mnt/usb/musubi-bundle/artifacts/musubi-server`（ポート 18080）
  4. `musubi-web`: `/mnt/usb/musubi-bundle/artifacts/musubi-web`（ポート 13001）
- **動作保証**:
  - 全4プロセスが `RUNNING` 状態へ遷移し、正の PID を獲得。
  - 万が一プロセスが強制終了された場合、スーパーバイザーが即座に自動再起動を実施。

---

### UC-06: 統合運用ダッシュボードでの可視化
- **アクター**: インフラ運用オペレータ
- **アクセス先**:
  - **Musubi 統合監視ダッシュボード**: `http://<HOST_IP>:13001/`
  - **Grafana メトリクスダッシュボード**: `http://<HOST_IP>:3000/`
- **検証基準**:
  - ブラウザから HTTP 200 OK が返り、HTMX によるリアルタイム死活ステータス・SNMP帯域・リソースメトリクスが完全オフラインで描画されること。

---

## 4. E2E テストシナリオ仕様 (Test Scenario Specification)

本ユースケースを自動検証するため、`test/e2e/musubi_airgap_usb_scenario_test.go` に以下の7フェーズから成るE2Eシナリオテストを実装しています。

| フェーズ | テスト項目 | 入力・操作 | 期待される判定基準 (Assertion) |
|:---|:---|:---|:---|
| **PHASE 1** | **USB媒体エクスポート** | `usbMountDir := t.TempDir()` にバイナリ、コンテナtar、マニフェスト、Ed25519鍵、チェックサム、単一アーカイブを出力 | 全ファイルが存在し、実行権限・チェックサム整合性が成立すること。 |
| **PHASE 2** | **エアギャップ環境隔離** | `t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")`, `GOPROXY=off` | 外部インターネットへの通信が一切発生しないこと（万一通信した場合は即座に接続拒否）。 |
| **PHASE 3** | **暗号完全性 & 改ざん検知** | `bundle.VerifyBundle(usbBundleArchive, pubKey)` および 1バイトビット反転アーカイブの検証 | 正常バンドルは `Valid=true`, `SignatureValid=true`。<br>改ざんバンドルは検証拒否されること。 |
| **PHASE 4** | **オフラインコンテナ登録** | `agent.NewDockerPackageManager` で `docker load -i` を実行 | 外部レジストリ通信 0 件で両コンテナアーカイブがロードされること。 |
| **PHASE 5** | **看取り起動 (Supervisor)** | `supervisor.NewManifestSupervisor` で `StartAll` を実行 | 4プロセス全てが `StateRunning` かつ正のPIDを保持すること。 |
| **PHASE 6** | **Web ダッシュボード描画** | HTTP クライアントで `:13001` および `:3000` に GET リクエスト | HTTP 200 OK かつ HTML 本文に「Musubi Dashboard」「Grafana Container」が含まれること。 |
| **PHASE 7** | **グレースフルシャットダウン** | `ms.StopAll(3 * time.Second)` | 全プロセスが `StateStopped` に遷移し、ゾンビプロセスが残存しないこと。 |

---

## 5. CI/CD 自動化 & 回帰テスト統制 (CI/CD Pipeline)

GitHub Actions ワークフロー（`.github/workflows/musubi-scenario-test.yml`）において、本エアギャップシナリオおよびUSBエクスポートスクリプトが毎コミット・毎PRで自動実行されます。

```yaml
- name: Run Go E2E Musubi Scenario Tests (Standalone, Docker Hybrid & Airgap USB)
  env:
    MUSUBI_SERVER_BIN: /tmp/Musubi/bin/musubi-server
    MUSUBI_WEB_BIN: /tmp/Musubi/bin/musubi-web
  run: |
    echo "==> Executing Go E2E Scenario Tests for Musubi..."
    go test -v -run "TestMusubi_" ./test/e2e

- name: Verify Musubi Air-Gapped USB Media Export Pipeline
  run: |
    echo "==> Testing USB Media Export Pipeline Script..."
    ./examples/musubi/export-airgap-bundle.sh /tmp/usb-media-bundle
    test -f /tmp/usb-media-bundle/manifest.yaml
    test -f /tmp/usb-media-bundle/artifacts/musubi-server
    test -f /tmp/usb-media-bundle/artifacts/musubi-web
    test -f /tmp/usb-media-bundle/images/postgres.tar
    test -f /tmp/usb-media-bundle/images/grafana.tar
    test -f /tmp/usb-media-bundle/checksums
    test -f /tmp/usb-media-bundle/bundle-musubi-hybrid-airgap-1.0.tar.gz
```

---

## 6. トラブルシューティング & 運用チェックリスト (Operation & Troubleshooting)

### 6.1 GNU tar「file changed as we read it」エラーの防止
- **現象**: Linux 環境で `tar -czf ${OUTPUT_DIR}/bundle.tar.gz -C ${OUTPUT_DIR} .` を実行すると、tar自身が出力中のアーカイブファイルを読み込もうとして exit code 1 で異常終了する。
- **恒久対策**: アーカイブは一時ディレクトリ（`TMP_ARCHIVE=$(mktemp ...)`）で作成し、完了後に `mv` で USB 媒体へ移動する。

### 6.2 USB メディア容量のサイジング基準
- **PostgreSQL 16 Alpine tar**: 約 100MB〜150MB
- **Grafana 10.4.0 tar**: 約 300MB〜400MB
- **Musubi Server & Web バイナリ**: 約 40MB〜60MB
- **推奨 USB 容量**: **最低 2GB 以上**（差分更新や複数世代バックアップを考慮すると 8GB〜16GB 推奨）。

### 6.3 ポート競合の回避
ホスト上に既存の PostgreSQL（5432）や Grafana（3000）が稼働している場合、`manifest.yaml` の `listen_port` およびコンテナポートバインディング（`-p 15432:5432`, `-p 13000:3000` 等）を変更して衝突を回避します。

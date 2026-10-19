# Dockerハイブリッド構成におけるUSB可搬媒体書き出しと完全エアギャップ環境デプロイ仕様書・運用ユースケース集

## 1. 背景と設計思想 (Background & Architecture Philosophy)

### 1.1 オンプレミス閉域網における「コンテナ運用の緩さ」の徹底排除
オンプレミスや閉域網環境（重要インフラ、医療、防衛、金融、製造プラント等）では、セキュリティ上の理由から外部インターネット（Docker Hubやパブリッククラウド）との通信が完全に遮断（エアギャップ）されています。
従来のオンプレミス運用において、コンテナを導入すると以下のような「運用の緩み・属人化」が発生しがちでした：
- 準備端末で手動の `docker save` を打ち忘れ、現場でイメージが存在せずデプロイが中断する。
- 現場ホストで運用者が手動で `docker load -i` や `docker run -p ... -v ...` などの生コマンドを打ち、引数の渡し忘れやポート競合、権限不備などの人為的ミスが発生する。
- コンテナイメージの改ざん・破損検知がコンテナレジストリ依存になっており、オフライン媒体上の tar アーカイブが改変されても検知できない。
- `docker-compose` コマンドに依存し、エアギャップ現場に `docker-compose` 自体をインストールする手間や、バイナリプロセス（Go等）との統合監視ができない分断が生じる。

### 1.2 ytagarasu によるファーストクラス支援原則
ytagarasu は、「**コンテナになった瞬間に運用が緩くなることを一切許容しない**」設計思想を掲げています：
1. **ユーザーに手動コマンドを一切打たせない（Zero Manual Container Operations）**:
   - `docker save` によるイメージアーカイブ作成から、バンドル暗号チェーンへの自動包含、現場での `docker load -i`、コンテナ起動（ポート・ボリューム・ネットワーク設定）まで、すべて ytagarasu がファーストクラスでデプロイとしてサポート。
2. **コンテナイメージまで含めた完全暗号バンドル保証**:
   - コンテナイメージ tar もバイナリやマニフェストと同等に SHA-256 チェックサムと Ed25519 電子署名で防護。現場での 1 ビットの改ざん・破損も許しません。
3. **マニフェスト & スーパーバイザーのファーストクラス Docker サポート**:
   - マニフェストに `containers:` を宣言的に定義。Docker が期待するポートフォワード（`ports`）、ボリュームマウント（`volumes`）、環境変数（`environment`）、ネットワーク（`network`）、再起動ポリシー（`restartPolicy`）を完全提供。
4. **docker-compose ユーザー向けの移行・変換支援**:
   - `ytg manifest convert -f docker-compose.yml -o manifest.yaml` により、既存の compose 定義を ytagarasu マニフェストへワンショット変換。
   - 変換後は `docker-compose` ツール自体が不要となり、ytagarasu 単一バイナリのみでコンテナとネイティブプロセスの統合看取り・死活監視・自動再起動・暗号署名保証を享受可能。

---

## 2. アーキテクチャとマニフェスト仕様 (Architecture & First-Class Container Spec)

### 2.1 全体アーキテクチャ概要

```
[オンライン準備端末]
   │
   ├─ 既存資産: docker-compose.yml / Dockerfile / Goバイナリ
   │     │
   │     ▼ (ytg manifest convert -f docker-compose.yml)
   │  ytagarasu manifest.yaml (containers: & processes:)
   │     │
   │     ▼ (ytg bundle export / BuildBundle)
   │  【自動実行】未存在イメージの自動 docker save
   │  【自動実行】Ed25519署名 + SHA-256チェックサム生成
   │     │
   ▼ 出力
[USB可搬媒体 / 移動媒体]
   ├── manifest.yaml                    # コンテナ & プロセス統合マニフェスト
   ├── artifacts/                       # ネイティブバイナリ (musubi-server, web)
   ├── images/                          # コンテナアーカイブ (postgres.tar, grafana.tar)
   ├── signatures/manifest.sig          # Ed25519 デジタル署名
   ├── signatures/public.key            # 検証用公開鍵
   ├── checksums                        # 全ファイル SHA-256 チェックサム
   └── bundle-musubi-hybrid-1.0.tar.gz  # 自己完結型バンドルアーカイブ
   │
   ▼ 物理搬送 (サプライチェーンセキュリティ検査)
   │
[完全エアギャップ本番ホスト (外部インターネット完全遮断)]
   │
   ├─ ytg bundle verify (暗号完全性 & 改ざん検知)
   │
   ▼ ytg supervisor run -c manifest.yaml
   │
   ├─ 【完全自動】未ロードの images/*.tar を docker load -i
   ├─ 【完全自動】ports, volumes, network, env を反映して docker run
   └─ 【完全自動】ネイティブプロセスと混在して PID看取り・死活監視・自動再起動
```

### 2.2 ファーストクラス `containers:` マニフェスト仕様

ytagarasu のマニフェスト（`manifest.yaml`）は、Docker コンテナをネイティブプロセスと同等のファーストクラス市民として宣言できます：

```yaml
version: "1.0"
bundleVersion: "1.0.0"
release: "1.0.0"
metadata:
  name: musubi-airgap-docker-hybrid
  environment: airgap-usb

# 1. 宣言的コンテナ定義 (ytagarasu supervisor が自動看取り)
containers:
  - name: postgres-container
    image: postgres:16-alpine
    imageArchive: images/postgres.tar            # オフラインtarの相対パス (未指定時は images/<name>.tar を自動補完)
    ports:
      - "5432:5432"                             # ホストポート:コンテナポート
    volumes:
      - "./data/postgres:/var/lib/postgresql/data" # ホストパス:コンテナパス(:ro 等も可)
    environment:
      POSTGRES_DB: musubi
      POSTGRES_USER: musubi
      POSTGRES_PASSWORD: secretpassword
    network: bridge                             # bridge, host, またはカスタムネットワーク
    restartPolicy: always                       # always, on-failure, unless-stopped
    maxRetries: 5
    logPath: /var/log/ytagarasu/postgres-docker.log

  - name: grafana-container
    image: grafana/grafana:10.4.0
    imageArchive: images/grafana.tar
    ports:
      - "3000:3000"
    network: bridge
    restartPolicy: always
    healthEndpoint: http://127.0.0.1:3000/api/health # スーパーバイザーによるHTTPヘルスチェック
    logPath: /var/log/ytagarasu/grafana-docker.log

# 2. ネイティブプロセス定義 (Goバイナリ等)
processes:
  - name: musubi-server
    binary: ./artifacts/musubi-server
    env:
      PORT: "18080"
      DATABASE_DRIVER: postgres
      DATABASE_DSN: "postgres://musubi:secretpassword@127.0.0.1:5432/musubi?sslmode=disable"
    auto_restart: true
    max_retries: 5
    listen_port: 18080
    health_endpoint: http://127.0.0.1:18080/v1/system/healthz
    log_path: /var/log/ytagarasu/musubi-server.log

  - name: musubi-web
    binary: ./artifacts/musubi-web
    env:
      PORT: "13001"
      MUSUBI_API_URL: "http://127.0.0.1:18080"
    auto_restart: true
    max_retries: 5
    listen_port: 13001
    health_endpoint: http://127.0.0.1:13001/healthz
    log_path: /var/log/ytagarasu/musubi-web.log
```

---

## 3. 実践運用ユースケース (Operational Use Cases)

### UC-01: 準備端末での完全自動バンドル書き出し (Zero Manual Docker Save)
- **アクター**: リリースエンジニア / 準備担当者
- **概要**: ユーザーが個別に `docker save` を実行することなく、マニフェストからバンドルを 1 コマンドでエクスポート。
- **実行手順**:
  ```bash
  # マニフェスト内の containers: に定義されたイメージが未保存の場合、
  # ytagarasu が自動的に docker save を実行して images/ に格納
  ytg bundle export -c manifest.yaml -o /Volumes/USB/musubi-bundle
  ```
- **自動化パイプラインの動作**:
  1. `manifest.yaml` の `containers:` を走査。
  2. ローカルの `images/<name>.tar` が存在しない場合、ホストの Docker CLI を検出し自動で `docker save <image> -o images/<name>.tar` を実行。
  3. 全アセット（バイナリ、コンテナtar、設定ファイル）の SHA-256 チェックサムを算出。
  4. 秘密鍵で `manifest.yaml` の Ed25519 デジタル署名を生成（`signatures/manifest.sig`）。
  5. 媒体直下に自己完結型バンドルアーカイブ `bundle-musubi-hybrid-1.0.tar.gz` を生成。
- **期待結果**: ユーザーは `docker save` コマンドを 1 度も打つことなく、完全検証済みの媒体成果物が手に入る。

---

### UC-02: 既存 docker-compose.yml からのワンショット自動移行
- **アクター**: アプリケーション開発者 / インフラエンジニア
- **概要**: 既存の `docker-compose.yml` を再利用したい場合、ytagarasu がマニフェストへ自動変換。
- **実行手順**:
  ```bash
  ytg manifest convert -f docker-compose.yml -o manifest.yaml
  ```
- **変換対応項目**:
  - `image` ➔ `containers[].image`
  - `ports` ➔ `containers[].ports`
  - `volumes` ➔ `containers[].volumes`
  - `environment` ➔ `containers[].environment`
  - `restart` ➔ `containers[].restartPolicy`
  - `command` ➔ `containers[].command`
- **導入メリット**:
  - `docker-compose` コマンドや Python/Docker ランタイムの追加ツールが一切不要になる。
  - ytagarasu 単一バイナリのみで、Docker コンテナと Go 等のネイティブバイナリを統一されたアーキテクチャで看取り可能。

---

### UC-03: 物理搬送 & サプライチェーンセキュリティ検査
- **アクター**: セキュリティ管理者 / 搬送担当者
- **概要**: 媒体搬送途上での悪意ある差し替えやファイル改ざんを防止。
- **検証基準**:
  - コンテナイメージアーカイブ（`images/postgres.tar` 等）も含め、全ファイルがチェックサムとデジタル署名の対象。
  - 事前共有された信頼済み公開鍵と照合し、1 ビットでも改変・欠損があれば現場ホストでの起動をブロック。

---

### UC-04: 完全閉域網ホストでのワンストップ暗号検証
- **アクター**: 現場オンプレミス保守運用者
- **事前条件**: 本番ホストが外部ネットワークから完全に遮断されていること。USB媒体がマウントされていること。
- **実行手順**:
  ```bash
  ytg bundle verify /mnt/usb/musubi-bundle/bundle-musubi-hybrid-1.0.tar.gz
  ```
- **期待結果**:
  ```
  Signature: VALID (Ed25519 verified)
  Checksums: VALID (All 6 files SHA-256 match, including container archives)
  Status: VERIFIED
  ```

---

### UC-05: ytagarasu supervisor による完全自動コンテナロード & 看取り起動
- **アクター**: 現場オンプレミス保守運用者
- **概要**: ユーザーが `docker load -i` や `docker run` を打つ必要はゼロ。マニフェストを指定してスーパーバイザーを起動するだけ。
- **実行手順**:
  ```bash
  ytg supervisor run -c /mnt/usb/musubi-bundle/manifest.yaml
  ```
- **スーパーバイザーの自動看取りフロー**:
  1. **コンテナイメージ自動ロード**:
     - `containers:` 内のアーカイブ（`images/postgres.tar`, `images/grafana.tar`）を検出し、未ロードであれば自動で `docker load -i` を実行。外部レジストリ通信は 0 件。
  2. **引数自動組み立て & コンテナ起動**:
     - ポート（`-p 5432:5432`）、ボリューム（`-v ...`）、環境変数（`-e ...`）、ネットワーク（`--network bridge`）を自動付与して `docker run` を起動。
  3. **ネイティブプロセスの並行起動**:
     - `musubi-server`、`musubi-web` を指定された環境変数・作業ディレクトリで起動。
  4. **自律看取り & 自動リカバリ**:
     - 全4プロセスの PID・死活状態を常時監視。プロセスやコンテナが予期せず終了した場合は即座に再起動。

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
| **PHASE 4** | **docker-compose 移行 & コンテナ自動ロード** | `manifest.ConvertComposeToManifest` および `agent.NewDockerPackageManager` の検証 | `docker-compose.yml` から `containers:` が自動生成され、外部通信 0 件でイメージがロードされること。 |
| **PHASE 5** | **看取り起動 (Supervisor)** | `supervisor.NewManifestSupervisor` で `StartAll` を実行 | 自動で `docker load -i` が走り、ポート・マウント・ネットワーク付きでコンテナとネイティブ全4プロセスが `StateRunning` になること。 |
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
ホスト上に既存の PostgreSQL（5432）や Grafana（3000）が稼働している場合、`manifest.yaml` の `containers[].ports`（例: `"15432:5432"`, `"13000:3000"` 等）を変更するだけで簡単にポート衝突を回避できます。

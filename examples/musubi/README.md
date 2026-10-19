# Musubi × ytagarasu エアギャップ・オンプレミスデプロイメント実践ガイド (Full-Stack: PostgreSQL & Grafana 対応)

> 📘 **公式仕様書・ユースケース集**: 本構成の USB 可搬媒体エクスポートおよび完全エアギャップ環境デプロイに関する包括的な技術仕様・テストシナリオ・トラブルシューティングは、[docs/airgap-usb-docker-hybrid-test-scenarios-and-usecases.md](../../docs/airgap-usb-docker-hybrid-test-scenarios-and-usecases.md) をご参照ください。

本ディレクトリは、外部OSSである **Musubi**（Air-Gapped 自律ネットワークオーケストレーター & テレメトリゲートウェイ）およびその本番環境構成である **PostgreSQL 16** と **Grafana** を、**ytagarasu (ytg)** の宣言的マニフェストによってパッケージインストール・プロセス看取り・監視する実践ユースケースのサンプル構成です。

---

## 🎯 構成アーキテクチャ

Docker や Kubernetes、クラウドサービスに一切依存せず、Pure Go および OS ネイティブパッケージによって完全オンプレミス自律動作します。

```text
[ytg supervisor (プロセス看取り・死活監視・自動再起動・暗号完全性保証)]
       │
       ├─► postgres (PostgreSQL 16 RDBMS / SQLステート永続化 :5432)
       │        ▲
       │        │ DB接続 (pgx Cgo-Free)
       │        ▼
       ├─► musubi-server (Core API / SNMP Trap Listener / Prometheusメトリクス :18080)
       │        ▲                              ▲
       │        │ Upstream API 通信             │ /metrics スクレイピング
       │        ▼                              ▼
       ├─► musubi-web (HTMX 統合監視UI :13001)  └─► grafana (時系列可視化ダッシュボード :3000)
       │        ▲                                      ▲
       │        │ ブラウザ閲覧                         │ ブラウザ閲覧
       ▼        ▼                                      ▼
        [ネットワーク管理者・運用オペレータ端末]
```

---

## 📦 パッケージインストール定義 (ytagarasu manifest)

マニフェスト内の `packages:` 定義により、対象環境のパッケージマネージャー（APT / DNF / YUM）経由で PostgreSQL 16 および Grafana をオフラインキャッシュまたは閉域リポジトリから一元導入します。

```yaml
packages:
  apt:
    manager: "apt"
    order: 1
    items:
      - name: "postgresql-16"
        version: "16*"
      - name: "grafana"
        version: "10*"
  dnf:
    manager: "dnf"
    order: 1
    items:
      - name: "postgresql-server"
      - name: "grafana"
```

---

## 🚀 クイックスタート手順

### 1. Musubi バイナリの準備
Musubi リポジトリから実行バイナリをビルドし、`bin/` 配下に配置します。

```bash
# Musubi リポジトリでビルド
cd /path/to/Musubi
go build -o bin/musubi-server ./cmd/musubi-server
go build -o bin/musubi-web ./cmd/musubi-web

# バイナリを本サンプルディレクトリへリンクまたはコピー
mkdir -p examples/musubi/bin
cp /path/to/Musubi/bin/musubi-* examples/musubi/bin/
```

### 2. マニフェストの真正性署名 (Ed25519)
改ざん検知およびサプライチェーンセキュリティを担保するため、Ed25519 鍵ペアを生成し、マニフェストに暗号署名を付与します。

```bash
# 鍵ペア生成 (未作成の場合: deploy.key と deploy.pub が生成されます)
ytg manifest keygen -o deploy

# マニフェスト署名 (manifest.yaml.sig が生成されます)
ytg manifest sign examples/musubi/manifest.yaml --key deploy.key -o examples/musubi/manifest.yaml.sig

# 署名検証
ytg manifest verify examples/musubi/manifest.yaml --key deploy.pub
```

### 3. ytg supervisor による看取り起動
```bash
ytg supervisor run -c examples/musubi/manifest.yaml
```

### 4. プロセス状態と暗号整合性の確認
別ターミナルからステータスを確認します。

```bash
ytg status
```

出力例:
```text
  __   __ _
  \ \ / /| |_  __ _
   \ V / | __|/ _` |
    | |  | |_| (_| |  _
    |_|   \__|\\__, | (_)
              |___/

Manifest File:     examples/musubi/manifest.yaml
Manifest Integrity: VERIFIED (Signed with Ed25519)
Active Processes:  4

NAME           PID     STATUS    RESTARTS  UPTIME     ENDPOINT
postgres       41233   RUNNING   0         1m23s      http://127.0.0.1:5432
musubi-server  41234   RUNNING   0         1m23s      http://127.0.0.1:18080/v1/system/healthz
musubi-web     41235   RUNNING   0         1m23s      http://127.0.0.1:13001/healthz
grafana        41236   RUNNING   0         1m23s      http://127.0.0.1:3000/api/health
```

### 5. 各ダッシュボードへのアクセス
- **Musubi 統合運用ダッシュボード**: `http://localhost:13001/`
- **Grafana メトリクスダッシュボード**: `http://localhost:3000/` (初期管理者: admin / admin)

---

## 🛡️ セルフヒーリングと自動復旧テスト
万が一 `postgres`, `musubi-server`, `musubi-web`, `grafana` のいずれかのプロセスが強制終了（`kill -9` や Kernel OOM）された場合でも、`ytg supervisor` が瞬時に検知し、自動再起動およびヘルスチェックによる正常性復帰を実行します。

---

## 🐳 Docker ハイブリッド構成 (manifest-docker.yaml)

ホストOS上の Docker を利用して PostgreSQL および Grafana をコンテナとして起動・看取りしつつ、Musubi Core Server と Web UI をネイティブプロセスとして連携させる構成です。

```bash
# 署名と検証
ytg manifest sign examples/musubi/manifest-docker.yaml --key deploy.key -o examples/musubi/manifest-docker.yaml.sig
ytg manifest verify examples/musubi/manifest-docker.yaml --key deploy.pub

# supervisor 看取り起動
ytg supervisor run -c examples/musubi/manifest-docker.yaml
```

### APT / DNF リポジトリ動的追加
マニフェスト内で Docker 公式リポジトリの URL を宣言することにより、エージェントが自動的に `/etc/apt/sources.list.d/docker.list` を構成し、最新のコンテナランタイムを非対話的に導入します。

---

## 💾 USB可搬媒体への書き出しと完全エアギャップ環境デプロイ手順

外部ネットワークとの通信が一切遮断された「完全エアギャップ（オフライン）環境」にデプロイする場合、オンライン/準備環境において **USB メモリやポータブル SSD などの可搬媒体へアセット一式をエクスポート** してから現場へ持ち込みます。

### 1. 準備端末での媒体書き出し (USB Export Pipeline)
インターネット接続可能な準備端末で `export-airgap-bundle.sh` を実行し、USB マウント先（例: `/Volumes/USB_STICK/musubi-bundle`）へ書き出します。

```bash
# USBマウント先を指定してエクスポート
./examples/musubi/export-airgap-bundle.sh /Volumes/USB_STICK/musubi-bundle
```

エクスポートされる媒体データ構成：
```text
/Volumes/USB_STICK/musubi-bundle/
├── manifest.yaml                    # オフラインコンテナ・バイナリ看取りマニフェスト
├── bundle-musubi-hybrid-airgap-1.0.tar.gz  # 配布用単一アーカイブ
├── signatures/
│   ├── manifest.sig                 # Ed25519 暗号署名
│   └── public.key                   # 署名検証用公開鍵
├── checksums                        # 全アセットの SHA-256 チェックサム
├── artifacts/
│   ├── musubi-server                # Musubi バックエンドバイナリ
│   └── musubi-web                   # Musubi Web ダッシュボードバイナリ
├── images/
│   ├── postgres.tar                 # PostgreSQL 16 コンテナイメージアーカイブ (docker save)
│   └── grafana.tar                  # Grafana 10.4.0 コンテナイメージアーカイブ (docker save)
└── configs/
    └── musubi.yaml                  # エアギャップ用設定ファイル
```

### 2. エアギャップ現場での媒体検証と自律デプロイ (Zero Manual Operations)
USB メモリを完全オフラインの対象ホストに接続し、以下のコマンドで整合性検証と看取り起動を行います。
**ユーザーが生の `docker load` や `docker run` コマンドを手動で叩く必要は一切ありません。**

```bash
# 1. 媒体およびバンドルアーカイブの暗号完全性を検証（改ざん・破損検知）
#    コンテナイメージアーカイブ（images/*.tar）も含めて全ファイルが Ed25519 / SHA-256 検証されます
ytg bundle verify /mnt/usb/musubi-bundle/bundle-musubi-hybrid-airgap-1.0.tar.gz

# 2. ytg supervisor による完全自動コンテナロード & 看取り起動
#    マニフェスト内の containers: を検出し、未ロードのオフライン tar を自動で docker load -i して起動
ytg supervisor run -c /mnt/usb/musubi-bundle/manifest.yaml
```

スーパーバイザーが自律実行するデプロイパイプライン：
1. **自動コンテナイメージロード**: `images/postgres.tar` および `images/grafana.tar` を自動で `docker load -i`（外部レジストリ通信 0 件）。
2. **ポート・マウント・ネットワーク自動構成**: ポート（`-p 5432:5432`, `-p 3000:3000`）、ボリュームマウント（`-v ./data/postgres:...`）、環境変数、ネットワーク（`--network bridge`）を自動付与してコンテナを作成・起動。
3. **ネイティブプロセス並行起動**: `musubi-server`, `musubi-web` を起動し、全4プロセスを一括で PID 看取り・死活監視・自動再起動。

---

## 🔄 docker-compose からの自動移行支援 (ytg manifest convert)

すでに `docker-compose.yml` で PostgreSQL や Grafana を運用している場合、ytagarasu の自動変換コマンドで 1 発でマニフェスト化できます：

```bash
# docker-compose.yml を ytagarasu の型安全な manifest.yaml に変換
ytg manifest convert -f docker-compose.yml -o manifest.yaml
```

**変換後のメリット**:
- 現場のエアギャップ環境に `docker-compose` コマンドや追加ランタイムをインストールする必要がなくなります。
- `ytagarasu` 単一バイナリのみで、Docker コンテナと Go 等のネイティブバイナリの混在看取り・自動復旧・暗号署名保証を享受できます。

### 3. ダッシュボードへのアクセス
- **Musubi 統合監視ダッシュボード**: `http://<HOST_IP>:13001/`
- **Grafana テレメトリダッシュボード**: `http://<HOST_IP>:3000/`

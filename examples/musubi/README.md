# Musubi × ytagarasu エアギャップ・オンプレミスデプロイメント実践ガイド (Full-Stack: PostgreSQL & Grafana 対応)

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

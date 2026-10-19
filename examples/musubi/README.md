# Musubi × ytagarasu エアギャップ・オンプレミスデプロイメント実践ガイド

本ディレクトリは、外部OSSである **Musubi**（Air-Gapped 自律ネットワークオーケストレーター & テレメトリゲートウェイ）を、**ytagarasu (ytg)** の宣言的マニフェストによってデプロイ・プロセス看取り・監視する実践ユースケースのサンプル構成です。

---

## 🎯 構成アーキテクチャ

Docker や Kubernetes、外部データベース（PostgreSQL等）に一切依存せず、Pure Go と SQLite WAL モードによって完全自律動作します。

```text
[ytg supervisor (プロセス看取り・死活監視・自動再起動・暗号完全性保証)]
       │
       ├─► musubi-server (Core API / SQLite / SNMP Trap Listener :18080)
       │        ▲
       │        │ Upstream API 通信
       │        ▼
       └─► musubi-web (Docker-Free HTMX 統合監視ダッシュボード :13001)
                ▲
                │ HTTP GET / (ブラウザ閲覧)
                ▼
        [管理者・運用オペレータ端末]
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
# 鍵ペア生成 (未作成の場合)
ytg manifest keygen --privkey key.priv --pubkey key.pub

# マニフェスト署名
ytg manifest sign -m examples/musubi/manifest.yaml --privkey key.priv
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
Active Processes:  2

NAME           PID     STATUS    RESTARTS  UPTIME     ENDPOINT
musubi-server  41234   RUNNING   0         1m23s      http://127.0.0.1:18080/v1/system/healthz
musubi-web     41235   RUNNING   0         1m23s      http://127.0.0.1:13001/healthz
```

### 5. Musubi ダッシュボードの確認
ブラウザまたは curl で Musubi のリアルタイムダッシュボードにアクセスします。

```bash
open http://localhost:13001/
# または
curl -s http://localhost:13001/ | grep -i "Musubi Dashboard"
```

---

## 🛡️ セルフヒーリングと自動復旧テスト
万が一 `musubi-server` や `musubi-web` のプロセスが強制終了（`kill -9` や Kernel OOM）された場合でも、`ytg supervisor` が瞬時に検知し、自動再起動およびヘルスチェックによる正常性復帰を実行します。

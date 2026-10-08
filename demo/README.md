# 🦅 ytagarasu & ytg 統合オンプレミス実行監視デモ環境

Docker を用いて **「外部インターネットから隔離されたオンプレミス閉域網 Linux サーバー」** を手元で再現し、
**マニフェスト駆動プロセス看取り・ヘルスチェック・クラッシュ自己修復・現場手動ロールバック（エスケープハッチ）** をリアルに体感できるインタラクティブデモです。

---

## 🚀 クイックスタート (ワンコマンド起動)

ターミナルで以下のコマンドを実行するだけで、デモ環境が起動します：

```bash
# 対話型メニューを起動
./demo/run.sh
```

または直接サブコマンドを実行：

```bash
# 1. デモ環境の起動 (Docker コンテナ立ち上げ)
./demo/run.sh start

# 2. プロセス看取りステータス確認
./demo/run.sh status

# 3. 障害注入実験 (kill -9 による強制クラッシュと自己修復)
./demo/run.sh chaos

# 4. 手動ロールバック実験 (ytg supervisor rollback --all)
./demo/run.sh rollback

# 5. コンテナ停止・クリーンアップ
./demo/run.sh stop
```

---

## 🌐 ブラウザでの視覚的体験

デモ環境起動中、ブラウザで以下の URL にアクセスできます：

👉 **[http://localhost:8080](http://localhost:8080)**

- **リアルタイム表示**: 現在の PID、稼働時間（Uptime）、リクエスト数を表示。
- **💥 クラッシュボタン**: 画面上の「⚡ プロセスをクラッシュさせる」ボタンをクリックすると、Web API プロセスが即座に強制終了し、2秒後に自動再起動されて **新しい PID で即座に復帰する様子** をブラウザ上で体感できます。

---

## 🏗️ アーキテクチャとマニフェスト (`demo/manifest.yaml`)

単一バイナリ `ytg`（2.7MB〜3.9MB）が、設定マニフェストに基づいて複数プロセス（API サーバー、バックグラウンドワーカー）を親プロセスとして常駐看取りします。

```yaml
version: "1.0"
processes:
  - name: demo-api
    binary: /app/bin/demo-api
    args: ["-port", "8080"]
    work_dir: /app
    auto_restart: true
    max_restarts: 5
    listen_port: 8080
    health_endpoint: "http://localhost:8080/healthz"
    rollback_on_error: true
    escape_hatch_mode: true

  - name: demo-worker
    binary: /app/bin/demo-worker
    work_dir: /app
    auto_restart: true
    max_restarts: 5
    rollback_on_error: true
    escape_hatch_mode: true
```

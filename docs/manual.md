# ytagarasu (八咫烏) ユーザーマニュアル

本書は、エアギャップ・完全オフライン環境向けデプロイ基盤 **ytagarasu（八咫烏）** の導入・運用・トラブルシューティングに関する公式ユーザーマニュアルです。

---

## 目次

1. [システム概要と設計思想](#1-システム概要と設計思想)
2. [コンポーネント構成](#2-コンポーネント構成)
3. [エンドツーエンド運用手順 (Workflows)](#3-エンドツーエンド運用手順-workflows)
   - [Step 1: オンライン環境でのデプロイマニフェスト定義](#step-1-オンライン環境でのデプロイマニフェスト定義)
   - [Step 2: パッケージ依存関係の解決と署名付きバンドル生成](#step-2-パッケージ依存関係の解決と署名付きバンドル生成)
   - [Step 3: 差分バンドル (Delta Bundle) の作成と更新](#step-3-差分バンドル-delta-bundle-の作成と更新)
   - [Step 4: 物理メディア（USB等）での搬送](#step-4-物理メディアusb等での搬送)
   - [Step 5: オフライン成果物サーバーへのインポート](#step-5-オフライン成果物サーバーへのインポート)
   - [Step 6: Ansible によるエージェントの初期導入](#step-6-ansible-によるエージェントの初期導入)
   - [Step 7: エージェントによる自律同期・適用とヘルスチェック](#step-7-エージェントによる自律同期適用とヘルスチェック)
   - [Step 8: 自動ロールバックの挙動と復元確認](#step-8-自動ロールバックの挙動と復元確認)
   - [Step 9: 暗号学的改ざん検証 (監査ハッシュチェーン)](#step-9-暗号学的改ざん検証-監査ハッシュチェーン)
4. [HTMX Web ダッシュボード操作ガイド](#4-htmx-web-ダッシュボード操作ガイド)
5. [CLI コマンドリファレンス](#5-cli-コマンドリファレンス)
6. [設定ファイルリファレンス](#6-設定ファイルリファレンス)
   - [manifest.yaml 仕様](#manifestyaml-仕様)
   - [agent.yaml 仕様](#agentyaml-仕様)
7. [トラブルシューティング (FAQ)](#7-トラブルシューティング-faq)

---

## 1. システム概要と設計思想

現代のエンタープライズインフラにおいて、金融決済システム、医療機器制御網、工場IoT、防衛・公共インフラなどは、情報漏洩やサイバー攻撃を防ぐため、外部インターネットから物理的・論理的に完全隔離（エアギャップ）されています。

しかし、これらの環境では以下のような深刻な課題が存在します：
- **推移的パッケージ依存関係の欠落**: `apt-get` や `dnf` で単一のパッケージをインストールしようとしても、数十個の依存ライブラリが不足して失敗する。
- **設定ミスの致命的影響**: 設定ファイルの反映ミスや構文エラーが発生した際、外部からリモート修復ができず現場の長時間のサービス停止につながる。
- **改ざんや取り違えのリスク**: 物理メディアで持ち込まれるアーカイブが正規のものか、誰がいつ持ち込み適用したかの監査証跡が残らない。

**ytagarasu** は、これらの課題を「**オンラインでの完全解決 & 署名**」「**オフラインサーバーでの CAS 重複排除配信 & 改ざん耐性監査**」「**エージェントでのアトミック適用 & 即座ロールバック**」の3本柱で解決します。

---

## 2. コンポーネント構成

| コンポーネント | 種別 | 主な責務 |
| :--- | :--- | :--- |
| **`ytagarasu`** | CLI | マニフェスト作成、依存パッケージ解決、Ed25519 署名、フル/差分バンドルエクスポート、監査検証 |
| **`ytagarasu-server`** | サーバー | 署名・有効期限検証、CAS ストレージ、SQLite WAL リリース管理、仮想リポジトリ配信、組み込み HTMX ダッシュボード |
| **`ytagarasu-agent`** | デーモン | ノード常駐、Desired State ポーリング、flock ガード、非対話パッケージ導入、設定レンダリング、アトミックロールバック |
| **`roles/deploy_agent`** | Ansible | エージェントのバイナリ配置、設定展開、systemd ユニット管理（初期ブートストラップ） |

---

## 3. エンドツーエンド運用手順 (Workflows)

### Step 1: オンライン環境でのデプロイマニフェスト定義

インターネットに接続された開発・ステージング環境で、サービスのデプロイ定義ファイル（`manifest.yaml`）を作成します。
対話型ウィザードを使用するか、直接ファイルを編集します：

```bash
ytagarasu manifest init
```

設定ファイル例（`manifest.yaml`）：
```yaml
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "v1.0.0"
createdAt: "2026-09-27T00:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "payment-gw"
    type: "golang"
    artifact: "artifacts/payment-gw"
    destination: "/usr/local/bin/payment-gw"
    permissions: "0755"
    selector:
      roles: ["payment"]
configs:
  - template: "configs/payment.conf.tmpl"
    destination: "/etc/payment-gw/payment.conf"
    permissions: "0644"
    validateCommand: "cat {{.TempFile}}"
healthChecks:
  - type: "command"
    command: "test -f /etc/payment-gw/payment.conf"
    intervalSeconds: 1
    maxRetries: 3
rollback:
  autoOnFailure: true
  strategy: "immediate"
```

### Step 2: パッケージ依存関係の解決と署名付きバンドル生成

1. 署名用 Ed25519 鍵ペアを生成します（初回のみ）：
   ```bash
   ytagarasu keygen -d ./keys
   ```
2. バンドルを評価・署名・出力します：
   ```bash
   ytagarasu bundle export \
       --manifest manifest.yaml \
       --source . \
       --output release-v1.0.0.tar.gz \
       --key ./keys/private.key
   ```
   出力される `release-v1.0.0.tar.gz` には、成果物・設定テンプレート・マニフェスト・デジタル署名・公開鍵・SHA-256 チェックサムが内包されます。

### Step 3: 差分バンドル (Delta Bundle) の作成と更新

すでにベースバージョン（例: `v1.0.0`）がオフラインサーバーに展開されている場合、`v1.0.1` への更新時は差分バンドルを使用することで、搬送メディアのサイズを大幅に削減できます：

```bash
# 2つのバージョンの差分を確認
ytagarasu bundle diff release-v1.0.0.tar.gz release-v1.0.1.tar.gz

# 差分のみを抽出した軽量バンドルを出力 (80〜99% 容量削減)
ytagarasu bundle export \
    --manifest manifest.yaml \
    --source . \
    --output release-v1.0.1-delta.tar.gz \
    --key ./keys/private.key \
    --delta-from release-v1.0.0.tar.gz
```

### Step 4: 物理メディア（USB等）での搬送

生成した `.tar.gz` ファイルを、組織のセキュリティ規定に準拠した物理メディア（暗号化 USB メモリ、追記型光ディスク等）にコピーし、オフライン環境へ搬送します。

### Step 5: オフライン成果物サーバーへのインポート

オフライン環境内の `ytagarasu-server` へバンドルを取り込みます。

- **Web ブラウザから操作**:
  `http://<server-ip>:8080/ui` を開き、「バンドル手動インポート」領域にファイルをドラッグ＆ドロップして「バンドルをインポート」をクリックします。
- **curl コマンドから操作**:
  ```bash
  curl -X POST http://<server-ip>:8080/api/v1/bundles/import \
      -F "bundle=@release-v1.0.0.tar.gz"
  ```
インポートパイプラインが自動的に以下を実行します：
1. Ed25519 署名と有効期限（`ExpiresAt`）の厳格な検証（不正な場合は即時拒絶）。
2. CAS（Content-Addressable Storage）への SHA-256 重複排除ブロブ格納。
3. 仮想リポジトリ（`/repos/<service_id>/...`）へのハードリンク投影。
4. 改ざん耐性監査ログ（ハッシュチェーン）への記録。

### Step 6: Ansible によるエージェントの初期導入

対象ノード群への `ytagarasu-agent` の初回セットアップは、同梱の Ansible Role を使用して一括実行します：

```yaml
# deploy/ansible/site.yml
- hosts: targets
  become: true
  roles:
    - role: deploy_agent
      vars:
        agent_server_url: "http://192.168.10.10:8080"
        agent_service_id: "payment-gw"
        agent_roles:
          - "payment"
```

実行コマンド：
```bash
ansible-playbook -i deploy/ansible/hosts deploy/ansible/site.yml
```
これにより、バイナリ配置（`/usr/local/bin/ytagarasu-agent`）、設定ファイル配置（`/etc/ytagarasu/agent.yaml`）、systemd サービス登録（`ytagarasu-agent.service`）が自動完了します。

### Step 7: エージェントによる自律同期・適用とヘルスチェック

`ytagarasu-agent` は定期ポーリングにより Desired State（`GET /api/v1/services/{service_id}/desired`）を監視します。
新しいリリースが検知されると：
1. `flock` によりプロセス排他ロックを取得（並行多重実行の完全防止）。
2. 設定ファイルのスナップショットをバックアップ。
3. 設定ファイルを一時ファイルへレンダリングし、`validateCommand` を実行。
4. バリデーション成功時、`renameat` によりアトミックにファイルを本番パスへ置換。
5. ヘルスチェックプローブ（コマンドまたは HTTP）を実行。
6. 成功時、ステータスをコミットしサーバーへ監査レポートを送信。

### Step 8: 自動ロールバックの挙動と復元確認

もし `validateCommand` または `HealthCheck` が失敗した場合：
1. エージェントは即座にエラーを検知し、適用を中断します。
2. 事前に保存されたスナップショットから直前の正常な設定ファイルへミリ秒単位でアトミック復元します。
3. サーバーへ `agent.rollback` 監査イベントを記録し、異常状態のままサービスが稼働し続けることを防ぎます。

### Step 9: 暗号学的改ざん検証 (監査ハッシュチェーン)

すべてのデプロイ履歴は、直前レコードのハッシュと連鎖した SHA-256 ハッシュチェーンに格納されています。
端末または Web UI から、監査ログが何者かによって書き換えられていないかを検証できます：

```bash
ytagarasu audit verify --server http://<server-ip>:8080
```
成功時の出力：
```
✅ Audit trail is VALID and tamper-free!
  • Total records:    12
  • Last sequence:    12
  • Last record hash: 27771feb51cb08873d5bd19d49989e1a58f68445a20096f822a7d01ec49642d9
```

---

## 4. HTMX Web ダッシュボード操作ガイド

Node.js や npm、外部 CDN を一切使用しない組み込み HTMX ダッシュボードが提供されます。

- **URL**: `http://<server-ip>:8080/ui`
- **主要ビュー**:
  1. **📊 ダッシュボード (`/ui`)**:
     - 登録サービス一覧、各サービスのアクティブリリースバージョン、マニフェストチェックサム。
     - リアルタイムシステム稼働状態（メモリ使用量、Goroutine数、総リリース数）。
     - バンドル手動インポート用アップロードフォーム。
  2. **🛡️ 監査ログ & 改ざん検証 (`/ui/audit`)**:
     - シーケンス番号、UTC タイムスタンプ、イベント種別（`release.import`, `agent.deploy.success`, `agent.rollback` 等）。
     - `prev_record_hash` と `record_hash` の完全な連鎖表示。
     - **「🔍 即時チェーン検証実行」ボタン**: クリックするとサーバー内で数学的チェーン検証が即時実行され、`✅ チェーン整合性確認済み` バッジが表示されます。

---

## 5. CLI コマンドリファレンス

### `ytagarasu`
- `manifest init`: 対話型ウィザードによる `manifest.yaml` 作成
- `manifest eval -m <file>`: マニフェストの事前検証とリスク評価
- `keygen -d <dir>`: Ed25519 署名・検証鍵ペア（`private.key`, `public.key`）の生成
- `bundle export [options]`: 署名付きオフラインデプロイバンドルの生成
  - `--manifest, -m`: マニフェストパス
  - `--source, -s`: 成果物ルートディレクトリ
  - `--output, -o`: 出力先アーカイブパス（`.tar.gz`）
  - `--key`: 秘密鍵パス
  - `--force`: 警告があっても強制ビルド
  - `--delta-from`: 差分抽出元のベースバンドルパス
- `bundle verify -b <archive>`: バンドルの署名・チェックサム検証
- `bundle diff <base.tar.gz> <target.tar.gz>`: 2つのバンドル間のファイル差分可視化
- `audit list --server <url>`: 記録された監査ログの一覧表示
- `audit verify --server <url>`: 監査ハッシュチェーンの完全性検証

### `ytagarasu-server`
- `--listen, -l`: HTTP 待ち受けアドレス（デフォルト: `:8080`）
- `--data-dir, -d`: CAS ストレージ・SQLite DB・名前空間のルートディレクトリ
- `--db`: SQLite データベースパス（省略時は `<data-dir>/ytagarasu.db`）

### `ytagarasu-agent`
- `--config, -c`: 設定ファイルパス（`/etc/ytagarasu/agent.yaml` 等）
- `--server, -s`: 成果物サーバー URL
- `--service`: デプロイ対象のサービス ID
- `--interval, -i`: ポーリング間隔（例: `10s`）
- `--roles`: ホストロール（カンマ区切り、例: `api,worker`）
- `--once`: 1サイクルのみデプロイを実行して終了

---

## 6. 設定ファイルリファレンス

### `manifest.yaml` 仕様

```yaml
version: "1.0"                   # マニフェストスキーマバージョン
bundleVersion: "2026.09.27.1"    # バンドルバージョン
release: "v1.0.0"                # ソフトウェアリリースバージョン
expiresAt: "2027-01-01T00:00:00Z"# 有効期限 (省略可)
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "app-server"
    type: "golang"               # golang, python, htmx
    artifact: "artifacts/app"    # バンドル内の成果物相対パス
    destination: "/usr/bin/app"  # 配置先絶対パス
    permissions: "0755"
    selector:
      roles: ["api"]
configs:
  - template: "configs/app.tmpl" # テンプレート相対パス
    destination: "/etc/app.conf" # 配置先パス
    permissions: "0644"
    validateCommand: "cat {{.TempFile}}" # 反映前検証コマンド
healthChecks:
  - type: "command"              # command, http
    command: "test -f /etc/app.conf"
    intervalSeconds: 2
    maxRetries: 3
rollback:
  autoOnFailure: true            # 失敗時に自動復元
  strategy: "immediate"          # 即座ロールバック
```

### `agent.yaml` 仕様

```yaml
server_url: "http://192.168.10.10:8080" # ytagarasu-server のベース URL
service_id: "payment-gw"                # 担当するサービス識別子
interval: "10s"                         # ポーリング周期
state_dir: "/var/lib/ytagarasu-agent"   # 状態・ロック保存ディレクトリ
install_dir: "/"                        # 成果物インストール基準パス
roles:
  - "payment"                           # ホストの担当ロール
hostname: "node-01"                     # ホスト名識別子
```

---

## 7. トラブルシューティング (FAQ)

### Q1. バンドルインポート時に `400 Bad Request` または `invalid signature` となる
- **原因**: バンドル生成時に使用した秘密鍵と、検証用の公開鍵が一致していないか、搬送中にアーカイブが破損しています。
- **対処**: `ytagarasu bundle verify -b <bundle.tar.gz>` をローカルで実行し、署名エラーの詳細を確認してください。

### Q2. 差分バンドルインポート時に `base release not found` となる
- **原因**: 差分バンドルが前提としているベースリリースが、オフラインサーバーにまだインポートされていません。
- **対処**: 先にベースとなったフルバンドル（例: `v1.0.0`）をインポートした後に、差分バンドル（`v1.0.1-delta`）をインポートしてください。

### Q3. エージェントが `agent lock held by another process` と出力してスキップする
- **原因**: 前回のデプロイ処理が実行中であるか、何らかの理由で `agent.lock` が残留しています。
- **対処**: 前回の処理が終了するのを待つか、プロセスが存在しないことを確認した上で `<state_dir>/agent.lock` を安全に削除してください。

### Q4. 設定変更後に `syntax validation rejected file` となりロールバックされた
- **原因**: マニフェストに定義された `validateCommand`（例: `nginx -t`）がゼロ以外の終了コードを返しました。
- **対処**: テンプレート内の変数指定や構文に誤りがないか確認し、スナップショットから安全に復元された状態を維持したまま、修正した新バージョンのバンドルを作成・再配布してください。

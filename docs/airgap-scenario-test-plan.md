# エアギャップ（完全閉域網）シナリオテスト計画書・標準手順書
**Document ID**: ATP-AIRGAP-2026-001  
**Target Product**: ytagarasu (v0.8.0+)  
**Target Environment**: Linux (Ubuntu 22.04/24.04, RHEL/Rocky 9) 完全閉域・エアギャップ環境  
**Classification**: Quality Assurance / SRE Security Standard  

---

## 1. テスト概要と目的

### 1.1 背景と目的
オンプレミスの基幹系インフラ、金融・防衛・医療・社会インフラ等の極秘ネットワークでは、**「外部インターネットへの通信が一切禁止された完全閉域網（Air-Gapped Environment）」** でのシステム運用が絶対要件となります。
本シナリオテストは、外部インターネット（パブリックDNS、GitHub、Docker Hub、外部CDN、パブリックAPT/DNFミラー等）への接続が100%遮断された物理・仮想隔離環境において、`ytagarasu`（サーバー、エージェント、3文字CLI `ytg`）が**外部依存ゼロ（Zero-Dependency, Zero-CDN, Pure Go）で自律的にデプロイ、プロセス看取り、ヘルスチェック、自動ロールバックを完結できること**を実証・検証することを目的とします。

### 1.2 合格基準 (Definition of Done)
1. **外部通信完全遮断の証明**: 外部IP（例: `8.8.8.8`）および外部ホスト名への通信が確実に失敗すること（Network Unreachable または 接続拒否）。
2. **ローカル完結型デプロイの成功**: 外部リポジトリや外部キーサーバーを介さず、ローカルで生成された暗号署名バンドルがエージェントにより正常に取得・配置・実行されること。
3. **プロセス看取りとヘルスチェックの自律稼働**: 閉域内でスーパーバイザーがプロセスを監視し、1秒周期の死活監視と状態判定が機能すること。
4. **自動ロールバックの確実性**: 異常バイナリ投入時にヘルスチェック失敗を検知し、外部介入なしで直前正常バージョンへ自動復帰すること。
5. **改ざん耐性監査チェーンの整合性**: 全イベントがSHA-256ハッシュチェーンに改ざんなく記録され、`ytg audit verify` でOKとなること。
6. **ターミナルスナップショットと全ログの保存**: 各工程のCLI画面出力と全システムログが監査証跡として保存されること。

---

## 2. 閉域網テスト環境アーキテクチャ

```
+---------------------------------------------------------------------------------------+
|  AIR-GAPPED ISOLATED ENVIRONMENT (Network Policy: All Egress Blocked except Loopback) |
|                                                                                       |
|   +-------------------------------------------------------------------------------+   |
|   | 1. Isolation Guard: iptables / network isolation                              |   |
|   |    - DROP all packets to 0.0.0.0/0 (Internet)                                 |   |
|   |    - ALLOW only 127.0.0.1 / internal container network                        |   |
|   +-------------------------------------------------------------------------------+   |
|                                                                                       |
|   +--------------------------+                 +----------------------------------+   |
|   |    ytagarasu-server      |                 |         ytagarasu-agent          |   |
|   |  (CAS Store / SQLite)    | <--- (Pull) --- |  - Local APT/DNF Provisioning    |   |
|   |  - Manifest API          |                 |  - Autonomous Deployment Loop    |   |
|   |  - Artifact Distribution |                 |  - Atomic Switchover             |   |
|   |  - Audit Hash Chain      |                 |  - Fail-Safe Rollback Engine     |   |
|   +--------------------------+                 +----------------------------------+   |
|                 ^                                                |                    |
|                 |                                                v                    |
|   +--------------------------+                 +----------------------------------+   |
|   |  Operator / Release CLI  |                 |      Supervised Applications     |   |
|   |   (ytg / ytagarasu)      |                 |  - billing-api (State: Running)  |   |
|   |  - keygen / export       |                 |  - 1.0s Health Check Probe       |   |
|   |  - audit verify          |                 |  - Log Rotation & Disk Guard     |   |
|   +--------------------------+                 +----------------------------------+   |
+---------------------------------------------------------------------------------------+
```

---

## 3. テストシナリオ詳細

| シナリオ ID | シナリオ名称 | 検証内容 | 期待結果 |
|---|---|---|---|
| **SCN-01** | **外部ネットワーク遮断検証 (Air-Gap Isolation Check)** | 外部IP（`8.8.8.8`）、外部DNS、パブリックHTTPへの通信を試行 | すべて接続不能（Network unreachable / Timeout）で即時遮断される |
| **SCN-02** | **ローカル暗号署名鍵生成とバンドル作成** | `ytagarasu keygen` で Ed25519 鍵を生成し、マニフェストとアプリを署名・アーカイブ | 外部証明機関不要で暗号署名済み `.tar.gz` が出力される |
| **SCN-03** | **閉域配信サーバー起動とバンドルインポート** | サーバーをローカルポートで起動し、署名バンドルをCASストレージへ登録 | HTTP 201 Created でバンドルがインポートされ、SQLiteに記録される |
| **SCN-04** | **エージェント自律デプロイとOSパッケージ構成** | エージェントがポーリングし、ローカルリポジトリからアプリを展開 | アプリケーションが配置され、設定ファイルが決定論的にレンダリングされる |
| **SCN-05** | **プロセス看取りとリアルタイム死活監視** | `ytg status` および `ytg inspect` による稼働状況の確認 | プロセスがPIDを保持してRUNNING状態となり、ヘルスチェックが合格する |
| **SCN-06** | **異常リリース注入と自動ロールバック** | ヘルスチェックが失敗する意図的欠陥リリース（v1.1.0-bad）を投入 | エージェントが異常を検知し、直前安定版（v1.0.0）へ自動ロールバックする |
| **SCN-07** | **SHA-256 監査チェーン完全性検証** | `ytg audit verify` による改ざん検知チェック | 全リリースおよびロールバック履歴のハッシュチェーンが100%整合（0 Tampering） |

---

## 4. 標準作業手順書 (Standard Operating Procedure: SOP)

### ステップ 1: テストワークスペースの初期化とログディレクトリ準備
```bash
export WORK_DIR="/tmp/ytagarasu-airgap-test"
mkdir -p "${WORK_DIR}/logs" "${WORK_DIR}/snapshots" "${WORK_DIR}/server_data" "${WORK_DIR}/agent_data"
```

### ステップ 2: 外部ネットワークの完全遮断
```bash
# Linux 実機環境の場合（sudo権限）
sudo iptables -F OUTPUT 2>/dev/null || true
sudo iptables -A OUTPUT -o lo -j ACCEPT
sudo iptables -A OUTPUT -d 127.0.0.1/32 -j ACCEPT
sudo iptables -A OUTPUT -j DROP

# 遮断確認
curl --connect-timeout 2 http://8.8.8.8 >/dev/null 2>&1 && echo "FAIL: Internet accessible" || echo "PASS: Air-Gap Isolated"
```

### ステップ 3: 閉域CASサーバーの起動
```bash
./bin/ytagarasu-server --listen ":18090" --data-dir "${WORK_DIR}/server_data" > "${WORK_DIR}/logs/server.log" 2>&1 &
SERVER_PID=$!
```

### ステップ 4: Ed25519署名鍵生成と正常リリース（v1.0.0）バンドルの作成
```bash
./bin/ytagarasu keygen -d "${WORK_DIR}/keys"
./bin/ytagarasu bundle export \
    --manifest "${WORK_DIR}/bundle_src/manifest.yaml" \
    --source "${WORK_DIR}/bundle_src" \
    --output "${WORK_DIR}/billing-v1.0.0.tar.gz" \
    --key "${WORK_DIR}/keys/private.key" \
    --force
```

### ステップ 5: バンドルインポートとエージェント自律デプロイ
```bash
curl -sf -X POST "http://127.0.0.1:18090/api/v1/bundles/import" -F "bundle=@${WORK_DIR}/billing-v1.0.0.tar.gz"
./bin/ytagarasu-agent --config "${WORK_DIR}/agent_data/agent.yaml" --once
```

### ステップ 6: 意図的障害リリース（v1.1.0-bad）の投入と自動ロールバック検証
```bash
# 起動時クラッシュする欠陥バンドルをインポート
curl -sf -X POST "http://127.0.0.1:18090/api/v1/bundles/import" -F "bundle=@${WORK_DIR}/billing-v1.1.0-bad.tar.gz"
./bin/ytagarasu-agent --config "${WORK_DIR}/agent_data/agent.yaml" --once
```

### ステップ 7: 監査チェーン検証と全証跡のアーカイブ
```bash
./bin/ytagarasu audit verify --server "http://127.0.0.1:18090"
tar -czf "${WORK_DIR}/airgap_evidence_bundle.tar.gz" -C "${WORK_DIR}" logs snapshots
```

---

## 5. ターミナルスナップショットとログ保存規約

1. **ターミナルスナップショット (`snapshots/*.txt`)**:
   - 各ステップの実行直後に、ANSIカラーコードまたは標準テキスト形式でターミナル画面出力（ヘッダー、コマンド、レスポンス）をファイル保存する。
2. **全システムログ (`logs/*.log`)**:
   - `server.log`: ytagarasu-server の全HTTPアクセスログ、CASストレージコミットログ。
   - `agent.log`: ytagarasu-agent のポーリング、バンドルダウンロード、パッケージ適用、プロセス起動、ヘルスチェックログ。
   - `audit.log`: SQLite / JSONLines に記録された暗号署名監査チェーン。
   - `scenario_execution.log`: シナリオテスト全体のタイムスタンプ付き実行ログ。

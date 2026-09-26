// Copyright 2026 [Copyright Holder]
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: [YOUR_NAME]

# ADR 0005: オンプレミス向けオフライン対応デプロイ基盤のアーキテクチャ設計

## ステータス
承認済み (Accepted)

## コンテキスト
オンプレミスおよび重要インフラ（エアギャップ閉域環境）へのアプリケーション配備において、従来のSSH手動運用やAnsibleの過剰適用による運用負荷、Python依存関係の衝突、成果物の散乱、事後検証の困難さが課題となっていました。
これらを解消するため、通常運用でのSSH接続を完全排除し、単一の検証済みバンドルによるプル型エージェント自律デプロイと決定論的ロールバックを保証する、オフライン対応デプロイ基盤の設計・開発が求められています。

## 意思決定

以下の主要な技術スタックおよびアーキテクチャ設計を採用します。

1. **ytagarasu-server の自律適応型ローカルファースト設計（モード概念撤廃）とマルチサービス仮想名前空間**
   - **モード二元論の完全撤廃 (Zero Mode Awareness)**: `--mode=online/offline` フラグを廃止。単一の `ytagarasu-server` バイナリが、リクエストに応じてローカル照会（Local-First）と上流自律適応（Probe）を自動実行。ローカルにあれば即答、オンライン時は自動透過プロキシ・キャッシュ、オフライン時は即座にローカル完結エラーを返し、運用者やエージェントに相手サーバーのモードを意識させない。
   - **マルチサービス仮想名前空間 & CAS（Content-Addressable Storage）**: 1台のサーバーで複数サービス（Core API, ML Worker, Web Portal 等）を完全同居・分離。サービス単位の仮想リポジトリ（`/repos/<service>/...`）と CAS（SHA-256重複排除）により、ライブラリのバージョン競合やインデックス破損を原理的に排除。サービスごとにサーバーを立てる非効率を完全に根絶。
   - **100% サーバー経由のインストール (Zero Direct Outbound)**: オンライン・オフラインを問わず、対象ホストからの外部直接通信を遮断し、すべての apt / dnf / pip / バイナリ取得を 100% `ytagarasu-server` 経由に統一（Unified UX）。

2. **Ansibleの責務の限定化 (Bootstrap Only)**
   - Ansible は対象ホストへの初回エージェント導入（バイナリ配置、systemd登録、最小sudoers設定、公開鍵配置）の1回のみに限定し、定常デプロイからは完全に切り離す。

3. **OCI 1.1 Artifacts & ORAS 準拠のデプロイバンドル仕様**
   - 独自アーカイブ形式の車輪の再発明を避け、CNCF標準の OCI Image / Artifact 仕様（ORAS / Zot準拠）を採用。
   - バイナリ、deb/rpmパッケージ、設定テンプレート、マイグレーション、SBOM、署名を OCI Layout アーカイブ（`.tar.zst`）に封入。
   - 秘密情報はバンドルに平文で含めず、CNCF SOPS + age により暗号化。

4. **アトミック設定反映と決定論的ロールバック**
   - 設定反映時は一時ファイル生成・スキーマ検証・パーミッション設定・POSIX `rename()` による不可分置換を実行。
   - ヘルスチェック失敗時は、直前世代の設定・バイナリ・サービス状態へ即時自動ロールバック。

5. **4大配備対象スタック (Golang, Python, React, HTMX) の完全サポート**
   - **Golang**: CGOフリー静的バイナリの配置と systemd 再起動。
   - **Python**: オンラインでの Wheel (`.whl`) パッケージ完全収集と、オフライン対象ホストでの完全隔離 venv 自動構築（PyPI非接続）。
   - **React**: オンラインでの事前静的ビルド（`dist/`）と、Node.js不要な Nginx 公開ディレクトリへのアトミック展開。
   - **HTMX**: Go 内包型（`//go:embed`）または Python 内包型による、外部 CDN 完全非依存のローカルUI配信。

6. **ホストセレクタによるデプロイ先指定と Smallstep `step-ca` 連携 PKI**
   - **配備先指定**: `manifest.yaml` 内に `selector.roles`（例: `["api"]`, `["worker"]`, `["web"]`）とホスト上の絶対配置先（`destination`）を明記し、エージェントが自ホスト属性とマッチングして自律適用。
   - **2階層 TLS 暗号化**:
     - *第1層 (プラットフォーム)*: オフライン環境を支えるプライベート CA（ルート CA を Ansible で全ホストのシステム証明書ストアに事前配置）を採用し、agent・apt・pip の透過通信を保証。
     - *第2層 (業務アプリ)*: Nginx, Go API, gRPC 等のアプリケーションサーバー証明書・秘密鍵を Ytagarasu の管理下に置き、**Smallstep `step-ca` のローカル ACME サーバー** と連携した自動発行・自動更新（30日更新）、または SOPS+age 暗号化秘密鍵の配置をサポート。

7. **Ansible との機能境界および明示的制約条件 (Non-Goals)**
   - **Ansible の担当**: OS・ハードウェア初期セットアップ、ネットワーク設定、ディスクマウント、CAルート証明書初期配布、エージェント初回配置（Bootstrap）。
   - **Ytagarasu の担当**: アプリケーション配備、設定アトミック展開、アプリサーバーTLS証明書配備・更新、OSパッケージ/Wheel適用、systemd制御、自動ロールバック、ハッシュチェーン監査ログ。
   - **意図的スコープ外 (Non-Goals)**: OSカーネル更新・OS再起動を伴うディストリビューションアップグレード、動的ハードウェア変更、跨ノードの複雑な分散バリア同期、組織全体の包括的 IDaaS / 外部 PKI 認証局そのものの代替。

8. **マニフェスト作成支援 CLI の標準化 (`manifest init` / `generate` / `validate`)**
   - ゼロからの手書きによる構文エラーや運用負荷を排除するため、`ytagarasu manifest init`（対話型ウィザード）、`manifest generate --auto`（Go/Python/React/HTMXの自動静的スキャン）、`manifest validate`（スキーマ・セレクタ整合性検証）を標準提供。

9. **メタデータ最適化とガベージコレクション (Sparse Indexing & CAS GC)**
   - 全件ミラーを排除し、マニフェスト定義の閉包（Closure）のみからなるスパースな仮想インデックス（`Packages.gz` / `repomd.xml`）を生成（数KB〜数十KB）。
   - 組み込み CGO フリー SQLite WAL による B-Tree メタデータ管理と、参照ゼロ Blob の Mark-and-Sweep ガベージコレクションにより、長期運用でのメタデータ・ストレージ肥大化を完全防止。

10. **出所検証 (Provenance)・可観測性 (o11y)・厳格モード固定ガード**
    - 配信アセットごとに `X-Ytagarasu-Source: local_cas | upstream_proxy | bundle_import` を明示し、改ざん防止監査ログに完全な出所証跡を記録。
    - 監査基準・防衛要件に準拠するため、設定ファイルで外部通信をコードレベル遮断する `outbound_policy: strict_offline`（モード固定オプション）を標準提供。

11. **脆弱性更新・バージョンアップ対応パイプライン**
    - Dependabot / Trivy 連携による CI 自動パッチバンドル出力、物理搬送によるオフライン即時取り込み、および超緊急時の直接ホットフィックス CLI (`ytagarasu-server patch apply`) の 3 系統を確立。

12. **クラウドネイティブ OSS の積極採用と高性能ラップトップPCでの動作保証**
    - **採用 OSS**: Zot / ORAS (OCI Registry), Smallstep `step-ca` (Local ACME PKI), Sigstore Cosign (署名検証), CNCF SOPS + age (シークレット暗号化), VictoriaMetrics (o11y), Aptly (Go製 APT 管理)。
    - **Dewy 連携 S3 ストレージ設計**: 単一バイナリ完結を基本とし、内蔵 POSIX S3 ゲートウェイ（`versitygw` 等）をデフォルトで提供（メモリ追加 3〜5MB）。外部の **RustFS (`rustfs/rustfs`)** や MinIO にも設定ファイル 1 つで透過的に切り替え可能なプラガブル設計とする。
    - **リソース保証**: すべて CGO フリー単一静的 Go バイナリで構成され、コントロールプレーン全体のメモリ消費量は **200MB 未満（実測約145MB）**。外部クラウドサービス依存ゼロで、市販の高性能ラップトップノートPC（8〜16GB RAM）で軽快に自立稼働可能。

## 結果
- **メリット**:
  - 定常運用でのSSH接続が不要となり、セキュリティと監査性が大幅に向上。
  - 搬送前に不整合や依存欠落を100%事前検知（`blocked`）でき、オフライン現地でのデプロイ失敗リスクが極小化。
  - パッケージ更新、設定ファイル、バイナリが1つのリリース単位として固定され、再現性と追跡性が担保される。
  - 複数サービス・複数ロールが単一バンドル内で安全に分離・管理される。
- **トレードオフ / 制約**:
  - オンライン環境での依存関係解決エンジンの実装（APT / RPM メタデータパーサー）に一定の初期開発コストを要する。
  - 初期導入時に1度だけAnsibleまたは手動プロビジョニングが必要。
  - OSカーネル更新やOS再起動はOS保守手順（Ansible等）を併用する必要がある。

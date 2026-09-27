<!--
Copyright 2026 [Copyright Holder]

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Author: [YOUR_NAME]
-->

# ADR 0006: マニフェスト作成支援におけるホワイトリスト作成支援の採用とKISS原則に基づくブラックリスト撤廃

## ステータス
承認済み (Accepted)

## コンテキスト
オフライン対応デプロイ基盤 `ytagarasu` において、新規環境への移行やレガシーサーバーの集約を加速するため、既存サーバーの稼働状態（パッケージ、ディレクトリ、配置バイナリ、systemd サービス）から `manifest.yaml` を自動抽出・下書き生成する「マニフェスト作成支援機能（リバースエンジニアリング）」の要求が提起されました。

本機能の設計にあたり、以下の本質的な運用・セキュリティ上の課題に直面しました：
1. **構成不明のジレンマ**: 運用者は既存サーバーの内部構成（どのバイナリがどこから来たのか、どこに関連ファイルがあるか）を正確に把握できていないからこそツールを求めている。
2. **セキュリティの原則（Deny by Default）**: 許可されたもの以外は一切触れない「ホワイトリスト方式」がセキュリティ（SIRT）の唯一の正解である。しかし、初見のサーバーで最初から完全なホワイトリストを人間が入力することは不可能である。
3. **過剰設計の罠（過剰なハイブリッドアプローチ）**: 当初、ホワイトリスト（`--scan-dirs`）とブラックリスト（`.ytagarasuignore`）を併用し、最長一致優先度ルールや否定パターン（`!`）、競合解決マトリクスを導入する「ハイブリッド方式」を検討した。しかし、これは設定ファイルの散乱と認知負荷を増大させ、KISSの法則（Keep It Simple, Stupid）に反することが判明した。
4. **バイナリ出所不明の現実**: 現行サーバーに配置されているバイナリの元リポジトリやビルド元 URL が分からないケースにおいて、どのようにマニフェスト化するかという落とし所が必要であった。

## 検討された選択肢 (Alternatives Considered)

### 選択肢 1: ブラックリスト（除外）方式のみ (`.ytagarasuignore` 方式)
- **概要**: サーバー全体（`/`）を探索対象とし、ユーザーが不要なパスを `.ytagarasuignore` で除外していく方式。
- **評価**: **【却下】**
  - 未知のファイルや秘密鍵（`/root`, `/home`, 認証情報）が誤って取り込まれる重大なセキュリティ事故リスクがある。
  - 企業のセキュリティ審査（SIRT）を通過できない。

### 選択肢 2: 単純ハイブリッド方式 (ホワイトリスト ＋ ブラックリスト多重評価)
- **概要**: `--scan-dirs` でホワイトリストを指定し、その配下に対して `.ytagarasuignore`（最長一致・否定記号 `!`）を評価する方式。
- **評価**: **【却下】**
  - 「ホワイトリスト作成支援」が存在するにもかかわらず、さらに別ファイル（ignore）を持たせるのは過剰設計。
  - 「どちらのルールが優先されるのか」「ネストしたルールで何が勝つのか」の推論をユーザーに強いる。安易にオプションを並べて運用者に認知負荷を丸投げしている。

### 選択肢 3: ホワイトリスト作成支援型 2 段階ワークフロー ＋ KISS原則によるブラックリスト完全撤廃
- **概要**:
  - 設定ファイルは `discovery-plan.yaml` のみとし、`.ytagarasuignore` は完全廃止する。
  - **Phase 1 (下見: `survey`)**: サーバーを変更しない Read-Only 実行で systemd サービス起点に関連構成をピンポイント検出し、候補カタログ `discovery-plan.yaml` を出力する。
  - **Phase 2 (取捨選択: 人間によるレビュー)**: ユーザーは生成されたプランを開き、不要な行をエディタで削除（またはコメントアウト）する。
  - **Phase 3 (確定: `generate`)**: プランに残った行のみを 100% 厳密なホワイトリストとしてマニフェストに出力する。
- **評価**: **【採用】**
  - セキュリティ原則（Deny by Default）を 100% 遵守。
  - ユーザーの認知負荷が極小（YAML の行を消すだけ、glob 構文の学習不要）。
  - プルリクエストで `git diff discovery-plan.yaml` を見れば監査が一目瞭然。

## 意思決定 (Decision)

**選択肢 3 を採用する。** 具体的な仕様・アーキテクチャは以下の通り決定する：

1. **Single Source of Truth (SSOT) の一本化**:
   - 設定ファイルは `discovery-plan.yaml` のみ。ブラックリスト別ファイル（`.ytagarasuignore`）は製品から完全撤廃する。
   - ルール競合、最長一致判定、否定パターン（`!`）等の複雑な評価エンジンは実装しない（KISS の法則）。
2. **1本道ワークフロー（下見 ➔ 取捨選択 ➔ 確定）**:
   - `ytagarasu discover survey` で下書きを生成し、人間が確定させた上で `ytagarasu discover generate` でマニフェストを出力する。
3. **バイナリ出所不明問題への現実的落とし所（Mode A / Mode B）**:
   - **Mode B (GitOps指向 / デフォルト)**: `ingest: false`。バイナリ実体はコピーせず、サイズ・ELF形式・SHA256ハッシュを記録し、URL欄に `# TODO: 配布元URLを記載` とコメント出力。
   - **Mode A (レガシー延命 / 実体吸い上げ)**: `ingest: true`（または `--ingest`）。現物バイナリをそのまま吸い上げ、ytagarasu バンドル（CAS）内に直接同梱。
4. **SIRT ビルトイン安全弁**:
   - 秘密鍵（`*.pem`, `*.key`）等の機密情報は、ユーザーの設定有無にかかわらず、ツールのビルトイン安全弁により自動マスク（`<REDACTED_SECRET>`）を実施する。

## 結果と影響 (Consequences)

### ポジティブな影響
- **極めて明快なユーザー体験**: ユーザーはツールの裏側にある複雑な優先度計算を一切意識せず、直感的にマニフェストを作成できる。
- **完全なセキュリティ保証**: 「意図せぬファイルが漏れ出る」リスクが構造的にゼロになる（Deny by Default）。
- **コードベースのシンプル化**: 複雑な ignore パターン解析器や多重パスフィルターを保守する必要がなくなり、テスト容易性と堅牢性が劇的に向上する。

### ネガティブな影響と緩和策
- **一時ファイル（ログ等）の混入懸念**:
  - *緩和策*: `ytagarasu` はバックアップツールではなく、アプリデプロイ基盤である。下見エンジンが「systemd の ExecStart / EnvironmentFile」のみをピンポイントで抽出するため、ログやキャッシュなどのゴミファイルが最初からプランに入り込まない。

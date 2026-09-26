---
name: github-pages-e2e-report
description: "E2Eテスト結果を機密サニタイズ付きで集約し、go test -json出力からスタンドアロンHTMLレポートを生成、GitHub Pagesへ自動デプロイし、テスト実行履歴を蓄積・可視化するためのプラクティス。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, OpenCode, OpenClaw, and other AI coding agents.
allowed-tools: Read Edit Write Glob Grep Agent
metadata:
  author: "[YOUR_NAME]"
  version: "1.0.0"
---

**Persona:** あなたはプロジェクトの **E2E テストレポーティング・スペシャリスト (E2E Test Reporting Specialist)** です。テスト結果の安全な公開、品質トレンドの可視化、および継続的デリバリーパイプラインにおけるテスト報告の自動化を専門とします。

# E2E テスト HTML レポート & GitHub Pages 自動デプロイ指針

## 1. アーキテクチャ概要

```
go test -json → e2e_raw.json → generate_e2e_report.py → index.html + history.json
                                       ↓
                              GitHub Pages Deploy
                         (actions/upload-pages-artifact)
```

## 2. 機密サニタイズ (Sensitive Data Masking)

`scripts/generate_e2e_report.py` はテスト出力に含まれる機密情報を自動マスキングします：

| パターン | マスク後 |
| :--- | :--- |
| `Bearer <token>` | `Bearer ***MASKED_TOKEN***` |
| `client_secret=<secret>` | `client_secret=***MASKED_SECRET***` |
| `"access_token": "<token>"` | `"access_token": "***MASKED_TOKEN***"` |
| `-----BEGIN PRIVATE KEY-----...` | `***MASKED_PRIVATE_KEY***` |

## 3. 履歴管理 (Execution History)

- `history.json` に最大 50 件のテスト実行履歴を蓄積
- 初回デプロイ時は GitHub Pages の既存 `history.json` を HTTP GET で取得・マージ
- 各履歴エントリ: コミットSHA、ブランチ名、Pass/Fail数、実行時間、タイムスタンプ

## 4. CI ワークフロー設定

```yaml
# ci.yml に必要な top-level 設定
permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: "pages"
  cancel-in-progress: false
```

### 必須ジョブ構成
1. **E2E テスト実行ジョブ**: `go test -v -json ./... | tee test_reports/e2e_raw.json`
2. **レポート生成ステップ**: `python3 scripts/generate_e2e_report.py`
3. **アーティファクトアップロード**: `actions/upload-artifact` (14日保持)
4. **Pages アーティファクト**: `actions/upload-pages-artifact` (main ブランチのみ)
5. **deploy-pages ジョブ**: `actions/deploy-pages`

## 5. ローカル実行

```bash
# E2E テストを実行し HTML レポートを生成
make e2e-report

# レポートをブラウザで確認
open test_reports/index.html
```

## 6. カスタマイズ

| 環境変数 | 用途 | デフォルト |
| :--- | :--- | :--- |
| `E2E_REPORT_TITLE` | HTMLレポートのプロジェクト名 | リポジトリ名 |
| `GITHUB_REPOSITORY` | GitHub Pages 履歴取得先 | 自動検出 |

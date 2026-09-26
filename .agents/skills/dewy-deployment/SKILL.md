---
name: dewy-deployment
description: "Dewy（オブジェクトストレージ経由のプル型バイナリ自動デプロイ）のアーキテクチャ設計、S3/さくらのクラウド オブジェクトストレージとの連携、GoReleaserによるマルチプラットフォームビルド、systemd/Ansibleによるプロビジョニング、SOPS+ageによるシークレット暗号化管理、Docker検証環境、およびGitHub ActionsからのCI/CDリリースパイプラインを適用・レビューする際に使用します。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, OpenCode, OpenClaw, and other AI coding agents.
allowed-tools: Read Edit Write Glob Grep Agent
metadata:
  author: "[YOUR_NAME]"
  version: "1.0.0"
---

**Persona:** あなたはプロジェクトの **Dewy デプロイメント・スペシャリスト (Dewy Deployment Specialist)** です。オブジェクトストレージ経由のゼロダウンタイムバイナリ自動デプロイの設計、運用、および検証を専門とします。

# Dewy デプロイメント・ガバナンス指針

## 1. Dewy アーキテクチャ概要

Dewy はオブジェクトストレージ（S3互換）からバイナリを自動PULL し、Graceful Restart（プロセスソケット引き継ぎ）でゼロダウンタイム切り替えを行うプル型デプロイエージェントです。

```
[GoReleaser / CI] → [S3 Object Storage] ← [Dewy Agent on Server] → [App Binary]
```

### リリースフロー
1. **tagpr** がバージョンタグ（`v*`）を自動付与
2. **GoReleaser** がマルチプラットフォームバイナリ（`linux/amd64`, `linux/arm64`）をクロスコンパイル
3. CI/dewyctl が S3 にバイナリを PUT（アーキテクチャ名統一: `amd64`, `arm64`）
4. サーバー上の **Dewy デーモン** が新バージョンを自動検知 → PULL → Graceful Restart

## 2. 設定テンプレート

| ファイル | 用途 |
| :--- | :--- |
| `deploy/dewy/dewy.env.example` | Dewy エージェント環境変数設定 |
| `deploy/dewy/systemd/dewy@.service` | systemd ユニットテンプレート |
| `deploy/dewy/docker-compose.dewy.yml` | ローカル Docker 検証環境 |
| `deploy/dewy/secrets.example.yml` | SOPS 暗号化対象シークレットサンプル |
| `.sops.yaml` | age 鍵による SOPS 暗号化ルール |
| `.github/workflows/deploy.yml` | GoReleaser + S3 リリース CI/CD |

## 3. アーキテクチャ命名規則（陥りがちな罠）

| Go / GoReleaser | Dewy / S3 Key | 注意 |
| :--- | :--- | :--- |
| `GOARCH=amd64` | `amd64` | ✅ 一致 |
| `GOARCH=arm64` | `arm64` | ✅ 一致 |
| `x86_64` (uname -m) | `amd64` に変換必須 | ⚠️ 不一致の罠 |

## 4. SOPS + age によるシークレット管理

```bash
# age 鍵ペア生成
age-keygen -o key.txt

# 暗号化
sops -e secrets.example.yml > secrets.enc.yml

# 復号
SOPS_AGE_KEY_FILE=key.txt sops -d secrets.enc.yml
```

## 5. 検証チェックリスト

- [ ] GoReleaser でマルチプラットフォームビルドが成功すること
- [ ] S3 にバイナリが正しいキー名でアップロードされること
- [ ] Dewy エージェントが新バージョンを検知し自動ダウンロードすること
- [ ] Graceful Restart によりゼロダウンタイムで切り替わること
- [ ] Docker Compose 環境で E2E 検証が通ること

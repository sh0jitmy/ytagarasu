---
name: github-issue-creator
description: "ytagarasuリポジトリの公式Issueテンプレート（feature_request.yml / bug_report.yml）に100%完全準拠し、独自セクションを一切作らずにGitHub Issueを自動起票するスキル。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, OpenCode, OpenClaw, and other AI coding agents.
metadata:
  author: [YOUR_NAME]
  version: "1.0.0"
  openclaw:
    emoji: "📋"
    requires:
      bins:
        - gh
        - git
---

# GitHub Issue 自動起票スキル (GitHub Issue Creator Skill)

## 1. 最優先遵守ルール (Zero Custom Sections)
- **独自のセクションや項目の作成は厳禁**:
  - 「担当ロール」「独自要件」「DoD」などの独自見出しを作成してはならない。
  - 必ず `.github/ISSUE_TEMPLATE/` に定義されたセクション構成をそのまま使用する。
- **タイトル形式**:
  - 機能要望の場合: `[FEATURE]: <簡潔な要約>`
  - バグ報告の場合: `[BUG]: <簡潔な要約>`

## 2. 機能要望 (`feature_request.yml`) の規定セクション
1. `### 📝 提案の概要`
2. `### 💡 動機・背景`
3. `### 📦 影響範囲` (チェックボックス形式)
4. `### 🔀 代替案の検討`
5. `### 📎 補足情報`

## 3. 起票コマンド
```bash
gh issue create --title "[FEATURE]: タイトル" --body "..." --label "enhancement"
```

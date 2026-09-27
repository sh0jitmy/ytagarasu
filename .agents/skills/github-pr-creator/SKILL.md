---
name: github-pr-creator
description: "直近のコミットログや git diff を分析し、英語タイトルと日本語ボディでリポジトリの pull_request_template.md に基づく GitHub プルリクエスト（PR）を自動作成する際に使用します。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, OpenCode, OpenClaw, and other AI coding agents.
metadata:
  author: [YOUR_NAME]
  version: "1.0.0"
  openclaw:
    emoji: "🚀"
    homepage: https://github.com/sh0jitmy/go_template
    requires:
      bins:
        - git
        - gh
    install: []
allowed-tools: Read Edit Write Glob Grep Bash(git:*,gh:*) Agent AskUserQuestion
---

> [!IMPORTANT]
> **ガバナンスと変更管理:**
> 本スキルは自動プルリクエスト生成のためのAIエージェントの行動指針です。人間から明示的な変更依頼がない限り、本スキルファイルを勝手に書き換えてはなりません。

**Persona:** あなたはリテールエンジニアリングの生産性を最大化する自動化のスペシャリストです。開発者が書いたコードの差分やコミット履歴を完璧に理解し、正確で読みやすく整理された日本語のプルリクエストを瞬時に作成します。

# GitHub プルリクエスト自動作成スキル

## 1. 動作フロー (AIエージェント用)

開発者から「PRを作成して」「プルリクエストを出して」などの指示を受けた場合、以下のステップを厳格に順守してください。

### Step 1: 変更内容の収集と分析
- `git status` を実行し、未コミットの変更があるか確認します。未コミットの変更がある場合、まずはコミットを完了するようユーザーに促します（基本は手動コミットを促すか、自動コミットの許可を得ます）。
- `git log origin/main..HEAD` （または `git log main..HEAD`）を実行して直近のコミットログを収集します。
- `git diff main...HEAD` を実行して、ベースブランチ（`main`）との具体的なコード差分を分析します。

### Step 2: リモートプッシュ状態の検証 (重要)
- **AIは絶対に勝手に `git push` を実行してはなりません**。プッシュはユーザー自身が手動で行う必要があります。
- リモートブランチが既にプッシュされているかを以下のコマンドで検証します：
  ```bash
  git ls-remote --exit-code --heads origin $(git branch --show-current)
  ```
- **リモートに存在しない場合**:
  - 「リモートへのプッシュが検出されませんでした。PRを作成する前に、手動で以下のコマンドを実行してプッシュしてください：`git push -u origin <branch>`。プッシュ完了後、再度お知らせください。」とメッセージを出力して処理を一旦停止（終了）します。

### Step 3: 英語PRタイトル & 日本語PRボディの自動生成
- **PRタイトル（Title）**:
  - グローバルな開発整合性と検索性を高めるため、PR タイトルは必ず**英語（English）**（Conventional Commits 形式：`feat: ...`, `fix: ...`, `chore: ...`, `docs: ...` 等）で作成します。日本語でのタイトル設定は行いません。
- **PR本文（Body）の厳格なテンプレート遵守規則（絶対ルール）**:
  - リポジトリの [.github/pull_request_template.md](file://.github/pull_request_template.md) を読み込みます。
  - **テンプレートの見出し、セクション名、構成を 100% 厳密に維持しなければなりません。独自のフォーマットへの改変やセクションの省略・順序入替は固く禁止します。**
  - **各セクションの入力要件**:
    1. **`## 📝 概要 / Summary`**:
       - なぜこの変更を行ったのか（Why）、何が変わったのか（What）の要約を日本語で記述します。
    2. **`## 🔗 関連する Issue / Related Issues` (最重要)**:
       - **対応する Issue が存在する場合は、必ず `Closes #<Issue番号>`、`Fixes #<Issue番号>`、または `Resolves #<Issue番号>` を記載してください。**（本文中に単に "Issue #xx" と書くだけでは GitHub 上で自動クローズされず、Issue が放置される原因となるため、キーワードを用いた正確なリンクを義務付けます）。
       - 複数ある場合は `- Closes #12`、`- Closes #13` と改行して列挙します。
       - 該当 Issue がない場合のみ `- 該当なし` と記載します。
    3. **`## 📦 変更カテゴリ / Change Category`**:
       - テンプレートの選択肢をすべて残し、変更ファイルに応じて該当する項目の `[ ]` を `[x]` に書き換えます。
    4. **`## 🛠️ 変更内容 / Changes`**:
       - ファイルごと、あるいは機能ブロックごとに変更内容を具体的な箇条書きで記述します。
    5. **`## 🧪 検証チェックリスト / Verification Checklist`**:
       - テンプレートのチェックリスト項目をすべて維持し、実際に検証済みの項目に `[x]` を付与します。
    6. **`## 🚨 注意事項・懸念点 / Notes & Concerns`**:
       - 破壊的変更、互換性の懸念、パフォーマンスへの影響などを記述します。特になければ `- なし` と明記します。

### Step 4: PRの作成実行
- 生成したPRボディテキストを一時ファイル（例: `/Users/shjtmy/.gemini/antigravity-ide/brain/<conversation-id>/scratch/pr_body.md`）に書き込みます。
- 以下のコマンドを提案し、ユーザーの承認を得た上で実行します（デフォルトは `--draft` 推奨ですが、ユーザー指示に準拠します）：
  ```bash
  bash scripts/create_pr.sh --title "PRタイトル" --body-file "/path/to/pr_body.md" --draft
  ```
- コマンド成功時に出力されるPR of URLをユーザーに分かりやすく提示します。

---

## 2. コール規約とエラー処理

- **認証エラー**:
  - `gh auth status` が失敗する場合は、「`gh auth login` コマンドを使用して GitHub CLI の認証を行ってください」とユーザーに促します。
- **ブランチエラー**:
  - 現在のブランチが `main` または `master` の場合は、直接PRを作成できないため、「トピックブランチを作成してそちらでコミットした上でプッシュしてください」とエラー終了します。

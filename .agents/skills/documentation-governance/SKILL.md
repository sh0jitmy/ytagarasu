---
name: documentation-governance
description: "ユーザー体験（UX: 操作感、知覚フィードバック、設定手順、挙動等）が変化した際に、マニュアル（docs/manual.md等）やREADME.mdを必ず都度見直して最新状態に同期・反映するドキュメンテーション・ガバナンス。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, OpenCode, OpenClaw, and other AI coding agents.
allowed-tools: Read Edit Write Glob Grep Agent
metadata:
  author: [YOUR_NAME]
  version: "1.1.0"
---

> [!IMPORTANT]
> **「機能追加・UI変更時の README.md & manual.md 同時同期」絶対遵守原則:**
> 新しい機能、CLIサブコマンド、Web UI（タブ・ボタン・画面）、設定パラメータ、またはテストレポートを追加・変更した際は、**必ず同一コミット / 同一PR内で [README.md](file://README.md) および [docs/manual.md](file://docs/manual.md) を見直し、最新状態に同期しなければなりません**。
> 「コードやテストの実装だけで作業を完了とし、ドキュメント同期を後回しにする行為」は重大なガバナンス違反です。
> PR作成前・タスク完了前には、必ず以下の3点を確認・更新してください：
> 1. [README.md](file://README.md) の「主要機能」「スクリーンショット」「クイックスタート」「CLIリファレンス」「E2Eレポートリンク」に新機能・新UIが反映されているか
> 2. [docs/manual.md](file://docs/manual.md) の詳細運用手順、UI操作手順、コマンド仕様、公開レポートURLが最新に同期されているか
> 3. GitHub Pages や関連ドキュメント内の内部リンクに 404 Not Found が一切発生しないか

**Persona:** あなたはプロジェクトの **ユーザー体験（UX）＆ドキュメンテーション・ガバナンス監査官 (Documentation Governance Specialist)** です。新機能やUIの追加、操作体験の変化に常にアンテナを張り、マニュアルやREADMEが1行たりとも陳腐化することを許さず、ユーザーがドキュメントを読めば最新の体験・機能を100%再現・理解できる状態を維持する責任を持ちます。

# ドキュメンテーション・ガバナンス指針

## 1. 機能・UI変更トリガーと同期チェックリスト（必須点検項目）

以下の契機が発生した場合は、コード修正と同時に必ず本スキルに基づきドキュメントを即座に更新します。

| 変更契機（Trigger） | 変化の具体例 | 同期すべきドキュメント項目（必須） |
| :--- | :--- | :--- |
| **① 新機能・サブコマンドの追加** | ・`discover survey` / `generate` 等のCLI追加<br>・新パッケージエコシステムやモード追加 | ・[README.md](file://README.md) の「主要機能」「クイックスタート」<br>・[docs/manual.md](file://docs/manual.md) の「運用手順」「CLIリファレンス」 |
| **② Web UI（画面・コンポーネント）の追加・変更** | ・新タブ（`/ui/discover` 等）やモーダルの新設<br>・ボタン・チェックボックス・入力フォームの追加 | ・[README.md](file://README.md) の「スクリーンショット一覧（画像追加）」<br>・[docs/manual.md](file://docs/manual.md) の「Web UI操作手順」「UIリファレンス」 |
| **③ レポート・GitHub Pages の追加・変更** | ・マトリクスレポートやE2E詳細レポートの新設<br>・レポート間相互リンクの追加 | ・[README.md](file://README.md) の「多層E2Eテスト」「公開URLリスト」<br>・[docs/manual.md](file://docs/manual.md) の「GitHub Pages レポート公開URL」 |
| **⑤ 操作・知覚体験の変化 (Interaction/Perception)** | ・ボタンのトグル化やクリック操作の変更<br>・画面フィードバックやインジケータ挙動変化 | ・[docs/manual.md](file://docs/manual.md) の「操作手順」「画面UIリファレンス」<br>・[README.md](file://README.md) の「5分で体験するクイック手順」 |
| **⑥ 障害・自己解決体験の変化 (Troubleshooting)** | ・死活監視や異常検知時のインジケータ挙動変化<br>・エラー時の回避策やFAQ項目の変化 | ・[docs/manual.md](file://docs/manual.md) の「トラブルシューティング (FAQ)」 |

---

## 2. 都度の更新実行プロセス

1. **ユーザー体験（UX）変化の特定**:
   - 行った変更によって、「ユーザーが何を見て、何を操作し、何を聞き、どう感じるか」という体験に差分が生じたかを分析する。
2. **マニュアルの該当セクション特定**:
   - [docs/manual.md](file://docs/manual.md) の目次から、その体験変化に対応するセクション（構成図、起動手順、操作シナリオ、UIリファレンス表、FAQ）を特定する。
3. **正確な体験の差分反映**:
   - 画面のラベル表記、操作手順、スピーカーから聞こえる台詞や音質、設定YAMLの書き方を、実際のユーザー体験と完全に一致させる。
4. **README.md の要約同期**:
   - 外部ユーザーや初見の開発者が最初に読む [README.md](file://README.md) に、新しいユーザー体験が正しく魅力的に表現されているか確認し更新する。
5. **再現性セルフチェック**:
   - マニュアル通りに操作して、記載通りのユーザー体験が得られるかを最終確認する。

---

## 3. ガバナンス品質基準 (Quality Gate)

- **体験と記述の完全一致率 100%**: ユーザーが画面を開いて操作した際、「ドキュメントと書いてあることが違う」「聞こえる音が違う」「ボタンの名前が違う」という認知的不一致を 0 件にする。
- **再現性**: 初見の開発者やユーザーが、[docs/manual.md](file://docs/manual.md) を上から順に実行して一切の迷いなく設計通りの体験を得られること。

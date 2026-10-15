---
name: maintainer-peer-review
description: "ytagarasuチームとして将来本コードを保守・運用・障害対応する当事者責任に基づく、7大観点（コンセプト適合性、Issue整合性、既存コード親和性、将来保守性、セキュリティ、CI/マルチOS堅牢性、フロントエンド品質完全性）プルリクエストレビュー標準。"
user-invocable: true
license: Apache-2.0
---

# ytagarasu 当事者保守主査レビュー標準 (Maintainer Peer Review Standard)

本スキルは、`sh0jitmy/ytagarasu` リポジトリにおいて、レビュアーが「将来自分たちが本コードを長期的に保守・運用・深夜オンコール対応する当事者である」という強いオーナーシップを持ってプルリクエストを審査するためのレビュー標準です。

## 【最重要鉄則】ゲート0：CIオールグリーン完全確認義務 (CI Green Before Review Gate)

**CIがすべてパス（オールグリーン）する前にレビューを開始・承認することは厳禁とします。**

1. **レビュー着手の絶対前提**:
   - プルリクエストに紐づくすべての CI ジョブ（Build, Test, golangci-lint, govulncheck, E2E Matrix ubuntu/windows, Agent-Server E2E等）が **`COMPLETED / SUCCESS`（完全合格）** に達していることを GitHub CLI または Web UI で確認した後にのみ、コードレビューを開始する。
2. **CI 失敗（RED）・実行中（PENDING）時の対応**:
   - CIが1つでもコケている、または実行中である段階で「Approve」や「形式的レビューコメント」を投稿してはならない。
   - CI失敗時はレビュー審査を即時中断し、起票者へ「CIが失敗しているため、まずCIの修正とオールグリーン復帰を優先せよ」と即座に差し戻す。
3. **レビューエビデンスへの CI 合格明記義務**:
   - レビュー本文・コメントの冒頭に、必ず審査対象の「コミットハッシュ」および「CI全ジョブ合格（例: 13/13 jobs passed）」を明記し、監査証跡とする。

---

## レビュー審査 7大観点チェックリスト

### 1. プロダクトコンセプト適合性 (Product Concept Alignment)
- **エアギャップ閉域網完結**: PythonやNode.js等の外部ランタイムを前提とせず、Pure Go単一バイナリで動作するか？
- **現場手動介入性 (Escape Hatch)**: 障害時にブラックボックス化せず、現場運用者が手動で即座に切り戻せるエスケープハッチ（`ytg supervisor rollback`）が担保されているか？

### 2. Issue 整合性 (Issue Specification Integrity)
- **要求仕様の網羅性**: 起票された Issue の技術的要件を満たしているか？
- **外部非公開文書の混入防止**: 外部開発者から参照できない内部ADR番号や内部文書が混入していないか？
- **CLIエルゴノミクス**: 公式短縮バイナリ `ytg` 配下で既存全コマンドが透過的に利用可能か？

### 3. 既存コード・アーキテクチャ親和性 (Legacy Code Affinity)
- **マニフェストデータ構造との整合**: `internal/manifest` のデータモデル（`services`, `applications`, `processes`）とシームレスに結合しているか？
- **パッケージ展開との責務境界**: パッケージ展開エンジン（`internal/pkgengine`）との境界が明確で、配置後にスムーズに看取りへ引き渡せるか？

### 4. 将来保守性・トラブルシュート容易性 (Maintainability by Maintainers)
- **状態の透過的調査**: 状態ファイル `.ytg-supervisor.state` が人間可読な JSON であり、現場で `cat` や `jq` で即座に確認可能か？
- **親切なエラーメッセージ**: エラーログに PID、終了シグナル、リトライ回数が明記されているか？
- **並行性安全性**: Goroutine リークや Mutex競合（デッドロック）が排除されているか？

### 5. セキュリティ・堅牢性 (Security & Hardening)
- **コマンドインジェクション対策**: `exec.Command` 引数に適切な静的解析回避（`//nolint:gosec`）が付与されているか？
- **ファイルパーミッション**: 状態ファイルが `0600` で作成され、改ざんを防止しているか？
- **機密情報の完全秘匿**: ログやステータス出力にトークンやパスワード等のクレデンシャルが漏洩しないか？

### 6. CI / クロスプラットフォーム堅牢性 (CI & Multi-OS Resilience)
- **Linux & Windows 実機 E2E 自動化**: GitHub Actions の OS Matrix（`ubuntu-latest`, `windows-latest`）で実機テストが 100% パスしているか？
- **並行テスト準拠**: 単体テストが `t.Parallel()` に完全準拠しているか？
- **クレデンシャル非混入**: PRやエビデンスに機密情報が一切含まれていないか？

### 7. フロントエンド & Web UI 品質完全性 (Frontend & UI/UX Integrity)
- **CSSクラス完全性 (Zero Missing Classes)**: テンプレート内で使用されている全クラスが CSS（`dashboard.css` 等）に完全定義されているか（`make css-lint` パス）？
- **状態監視画面の更新頻度原則 (1s Real-Time Polling)**: プロセス監視やシステムリソース等の状態監視を行う画面・コンポーネントの更新頻度が基本1秒（`1.0s` / HTMX `every 1s`）に設定されているか？
- **動的状態永続化 (Stateful Action & Polling Persistence)**: 停止・再起動などのアクション後、1秒定期ポーリング（HTMX `every 1s`）が走っても状態が巻き戻らず維持されるか？
- **実機レンダリング検証**: 文字列のアサーションだけでなく、Headless Chrome等による実機スクリーンショットでスタイル崩れがないことが確認されているか？
- **生きた状態監視の視覚化 (Living System & Anti-Stall)**: 画面が静止・怪しく見えないよう、心拍パルス（Heartbeat）、診断サイクル通番、精密ミリ秒タイムスタンプ、動的レイテンシが担保されているか？
- **CUD Triple Coding**: 記号・英語・色彩の3重識別（◆ RUNNING, ■ STOPPED, ▲ WARN, ✖ CRITICAL）が正確に担保されているか？

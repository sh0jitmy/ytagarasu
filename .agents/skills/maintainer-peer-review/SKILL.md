---
name: maintainer-peer-review
description: "ytagarasuチームとして将来本コードを保守・運用・障害対応する当事者責任に基づく、6大観点（コンセプト適合性、Issue整合性、既存コード親和性、将来保守性、セキュリティ、CI/マルチOS堅牢性）プルリクエストレビュー標準。"
user-invocable: true
license: Apache-2.0
---

# ytagarasu 当事者保守主査レビュー標準 (Maintainer Peer Review Standard)

本スキルは、`sh0jitmy/ytagarasu` リポジトリにおいて、レビュアーが「将来自分たちが本コードを長期的に保守・運用・深夜オンコール対応する当事者である」という強いオーナーシップを持ってプルリクエストを審査するための 6 大観点チェックリストです。

## レビュー審査 6大観点

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

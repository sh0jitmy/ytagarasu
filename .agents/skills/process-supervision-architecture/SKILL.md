---
name: process-supervision-architecture
description: "ytagarasu / ytg におけるマニフェスト駆動プロセス統合監視 (ManifestSupervisor)、自己修復、エスケープハッチ・ロールバック、Pure Go エアギャップ常駐管理、デッドロック防止規約。"
user-invocable: true
license: Apache-2.0
---

# プロセス統合監視 & 自己修復アーキテクチャ標準 (Process Supervision Architecture)

本スキルは、`ytagarasu` および 3文字短縮CLI `ytg` におけるプロセス常駐看取り、クラッシュ自動復旧、マニフェスト統合管理の設計・実装・保守における普遍的原則を定義します。

## 1. コア原則 (Core Principles)
1. **Pure Go / 外部ランタイム依存ゼロ**:
   - Python (`supervisord`) や Node.js (`PM2`) などの外部ランタイムをエアギャップ閉域網に持ち込ませず、単一静的バイナリ内で完結する。
2. **マニフェスト統合駆動 (Manifest-Driven)**:
   - 単一のプロセス監視にとどまらず、`manifest.yaml`（`processes`, `services`, `applications`）に定義された全プロセスを一括読み込みし、統合的にライフサイクルを統制する。
3. **個別再起動とマニフェスト一括再起動**:
   - `ytg supervisor restart <name>`: 対象プロセスのみを安全に再起動し、他サービスへの影響をゼロにする。
   - `ytg supervisor restart --all`: マニフェスト配下の全プロセスへ一括シグナルを送り、システム全体の同期再起動を可能にする。
4. **現場手動介入性 (Escape Hatch Rollback)**:
   - 自動再起動が無限クラッシュループに陥らないようサーキットブレーカー（`MaxRetries`）で保護し、運用者が手動で即座に前世代へ切り戻せる `ytg supervisor rollback <name|--all>` を備える。

## 2. 状態管理と透過性 (State Telemetry & Transparency)
- **状態ファイル永続化**:
  - デーモン稼働中はカレントディレクトリの `.ytg-supervisor.state`（または環境変数指定パス）に各プロセスの稼働状態（PID、状態、リスタート回数、コマンド）を JSON 形式で定期書き込みする。
  - パーミッションは厳格に `0600`（所有者のみ読み書き）とし、他ユーザーからのPID偽装・改ざんを防止する。
- **CLIからの透過参照**:
  - `ytg supervisor status` および `ytg supervisor status --json` は状態ファイルから即座にテレメトリを取得し、テーブルまたは構造化JSONで出力する。
  - 障害時は現場管理者が `cat .ytg-supervisor.state` や `jq` で直接状態を確認できる透過性を維持する。

## 3. 並行性・デッドロック回避設計 (Concurrency Safety)
1. **`exec.Cmd.Wait()` 単一待機パターン**:
   - 同一の `exec.Cmd` に対し別ゴルーチンから二重に `Wait()` を呼ぶとデッドロック（600秒タイムアウト）が発生する。
   - プロセス監視ゴルーチンのみが `Wait()` を呼び、終了時は `close(waitDone)` チャネルにより安全に終了を通知すること。
2. **Mutex 二重ロック防止**:
   - `Rollback()` や `Restart()` 等の複合メソッド内部で `Stop()` や `Start()` を呼ぶ場合、自分自身の `mu.Lock()` を保持したまま呼び出さないこと（再帰ロック禁止）。
3. **Gosec G204 / G702 静的解析対策**:
   - `os.Args` を介した既存バイナリへの安全な委譲や動的バイナリ実行には、明示的に `//nolint:gosec // Intentional delegation to ...` コメントを付与すること。

## 4. CUI エルゴノミクスと統一バイナリ命名
- **3〜4文字のショートバイナリ体系**:
  - メイン統合CLI: `ytg` (3文字)
  - 中央サーバー: `ytgs` (4文字, `ytg server` と等価)
  - エージェント: `ytga` (4文字, `ytg agent` と等価)
  - Web UI: `ytgw` (4文字, `ytg web` と等価)
- **完全後方互換**:
  - 既存の `ytagarasu`, `ytagarasu-server`, `ytagarasu-agent` は互換ラッパーとして残し、既存スクリプトやCIを一切破壊しないこと。

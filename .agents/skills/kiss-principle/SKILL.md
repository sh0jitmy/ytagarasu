---
name: kiss-principle
description: "KISSの法則（Keep It Simple, Stupid）設計・実装ガバナンススキル。過剰設計（Over-engineering）、尚早な抽象化（Premature Abstraction）、複雑な設定・レイヤーの増殖を排除し、コードベースと運用を極限までシンプルに保つための原則とレビュー指針。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, OpenCode, OpenClaw, and other AI coding agents.
metadata:
  author: "ytagarasu maintainers"
  version: "1.0.0"
  openclaw:
    emoji: "🎯"
allowed-tools: Read Edit Write Glob Grep Bash Agent
---

> [!IMPORTANT]
> **ガバナンスと変更管理:**
> 本スキルファイルは ytagarasu プロジェクトの基本設計・実装指針です。自己満足の複雑化を排し、オンプレミス・閉域網現場で保守しやすい「愚直なまでのシンプルさ（KISS）」を最優先とします。

# KISS 設計・実装ガバナンススキル (Keep It Simple, Stupid)

本スキルは、**「シンプルであることは、スマートであることよりも常に価値が高い」** という哲学のもと、過剰設計（Over-engineering）や自己満足の複雑化を排除し、オンプレミス・閉域網の現場で誰でも理解・保守できる極小・堅牢なシステムを構築するための統制プロトコルです。

---

## 1. なぜオンプレミス現場で KISS が絶対原則なのか

クラウドと異なり、オンプレミス・閉域網（エアギャップ）環境では：
1. **障害時にググれない**: ネットワーク遮断環境では、ドキュメントにない黒魔術的抽象化や複雑な挙動は運用者の致命傷になる。
2. **深夜の緊急対応は当直の1人で行われる**: 複雑なレイヤー（多層継承、過剰なDIコンテナ、メタプログラミング）があると、1行のバグ修正に数時間を要する。
3. **シンプル＝壊れる箇所が最小**: コード行数（LoC）が少なく、概念が少ないほど、バグと脆弱性の潜む余地が根本から消滅する。

---

## 2. KISS の 6 大原則 (The 6 Pillars of KISS)

### ① Do the Simplest Thing That Could Possibly Work (最も単純に動くものを作れ)
- 最初から将来の拡張（「いつか分散合意が必要になるかも」「いつか複数DBを切り替えるかも」）を想定したレイヤーを作ってはならない。
- **YAGNI (You Aren't Gonna Need It)**: 今必要な最小限の機能だけを愚直に実装する。

### ② Direct over Indirection (迂回より直接)
- 実装が1つしかないのにインターフェースを切る（Interface Pollution）ことを禁止する。
- 具象構造体を直接受け渡し、真にモックや差し替えが必要になった瞬間に初めて最小のインターフェースを抽出する。
- 過剰なデザインパターン（Abstract Factory, Builder, Strategy の濫用）を排除し、直接初期化・直接呼び出しを優先する。

### ③ Flat over Nested (ネストよりフラット)
- 制御構造はガード節（Guard Clause）と早期リターン（Early Return）を徹底し、インデントの深さを最大 2 階層までに抑える。
- ディレクトリ構造も無駄に細分化（`internal/domain/service/infrastructure/repository/...`）せず、コンポーネント単位でフラットに配置する。

### ④ Readable over Clever (スマートさより可読性)
- トリッキーなワンライナー、難解なビット演算、高度すぎるリフレクションなどの「賢そうに見えるコード」を禁止する。
- 1年後の自分や、新任のオンプレミス運用担当者が読んだ瞬間に意図が伝わる、平凡で直感的なコードを書く。

### ⑤ Zero-Magic Configuration (魔法のない設定)
- 複雑なDSL、動的設定インジェクション、環境変数の多段マクロを排除する。
- デフォルト値だけで即座に起動し、必要な変更は単一の平易な設定ファイル（YAML/JSON）またはCLIフラグで明示的に指定できるようにする。

### ⑥ Less Code is Better Code (より少ないコードこそ正義)
- 300行の洗練されたフレームワークより、30行の標準ライブラリの組み合わせを愛せ。
- コードを増やす提案よりも、不要なコードを削除（Delete）して同じ目的を達成する提案を組織の最高評価とする。

---

## 3. コードレビュー・自己監査チェックリスト (KISS Audit)

| 観点 | 問い | 判定基準 (PASS / REJECT) |
| :--- | :--- | :--- |
| **インターフェース** | そのインターフェースは本当に2つ以上の実装があるか？ | 1つしかないなら具象構造体に戻す (REJECT) |
| **レイヤー数** | 関数呼び出しが何重のラッパーを経て実処理に到達しているか？ | 3重以上の単なるパススルーは即刻インライン化 (REJECT) |
| **外部依存** | その機能は標準ライブラリ数行で書けないか？ | 外部ライブラリを安易に `go get` していたら削除 (REJECT) |
| **設定項目** | 現場で誰も触らないチューニングパラメータが増えていないか？ | 不要な設定フラグは定数化または削除 (REJECT) |
| **可読性** | この関数はコメントなしで上から下に流し読みできるか？ | 早期リターンを用いて左端に揃える (REJECT) |

---

## 4. Anti-Patterns & Refactoring (アンチパターンと是正例)

### ❌ アンチパターン 1: 1つしかない実装への過剰なインターフェース
```go
// 悪い例: 実装が1つしかないのに無駄にインターフェースとFactoryを作る
type ProcessSupervisorInterface interface {
    Supervise() error
}
type ProcessSupervisorFactory interface {
    Create() ProcessSupervisorInterface
}
```
```go
// 良い例 (KISS): 構造体を直接定義し、シンプルにNewする
type Supervisor struct {
    // ...
}
func NewSupervisor() *Supervisor {
    return &Supervisor{}
}
```

### ❌ アンチパターン 2: 深いネストとelse
```go
// 悪い例: 多段ネストで何が正常系かわからない
func Handle(p *Process) error {
    if p != nil {
        if p.IsAlive() {
            if err := p.Ping(); err == nil {
                return p.Restart()
            } else {
                return err
            }
        }
    }
    return ErrInvalid
}
```
```go
// 良い例 (KISS & Early Return): ガード節で左端を揃える
func Handle(p *Process) error {
    if p == nil || !p.IsAlive() {
        return ErrInvalid
    }
    if err := p.Ping(); err != nil {
        return err
    }
    return p.Restart()
}
```

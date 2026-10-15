---
name: effective-go
description: "Effective Go ガイドおよびGo言語公式イディオム標準スキル。命名規約、制御構造（Early Return）、関数設計、データ初期化（new vs make）、インターフェース（Accept interfaces, return structs）、並行処理（通信によるメモリ共有）、エラーハンドリング、panic/recoverの境界統制。"
user-invocable: true
license: Apache-2.0
compatibility: Designed for Claude Code, Cursor, OpenCode, OpenClaw, and other AI coding agents.
metadata:
  author: "ytagarasu maintainers"
  version: "1.0.0"
  openclaw:
    emoji: "🐹"
allowed-tools: Read Edit Write Glob Grep Bash Agent
---

> [!IMPORTANT]
> **ガバナンスと変更管理:**
> 本スキルファイルは ytagarasu プロジェクトの Go 言語公式イディオム標準です。コードの実装・リファクタリング・PRレビューにおいて例外なく本ガイドラインを適用してください。

# Effective Go 標準スキル (Effective Go Idioms & Standards)

本スキルは、Go言語の公式スタイルガイドである **『Effective Go』** およびGo公式プラクティスに基づき、慣用的（Idiomatic）、明瞭（Clear）、堅牢（Robust）、かつ効率的なGoコードを実装・レビューするための技術標準です。

---

## 1. 命名規約 (Names & Formatting)

### ① パッケージ名 (Package Names)
- 簡潔、明瞭、すべて小文字の単一単語（例: `supervisor`, `bundle`, `audit`）。アンダースコアやCamelCaseは禁止。
- パッケージ利用側で `supervisor.New()` や `bundle.Verify()` のように型・関数と組み合わせて自然に読めるように設計する。
- `util`, `common`, `helper`, `base` などの無意味で責務が曖昧なゴミ箱パッケージの作成を禁止する。

### ② ゲッター・セッター (Getters & Setters)
- ゲッター関数に `Get` プレフィックスを付けてはならない。
  - ⭕ `obj.Owner()` / `obj.SetOwner("admin")`
  - ❌ `obj.GetOwner()`
- 大文字エクスポートされたフィールドがある場合、自明なゲッター/セッターをわざわざ作らない（フィールド直接アクセスで十分）。

### ③ インターフェース命名 (Interface Names)
- 1つのメソッドのみを持つインターフェースは、メソッド名に `-er`（または `-or`）サフィックスを付与する。
  - `Reader`, `Writer`, `Closer`, `Supervisor`, `Deployer`
- 既存の標準ライブラリ（`io.Reader`, `fmt.Stringer` 等）と同じシグネチャを持つ場合は、必ず同じ名前・意味で実装する。

---

## 2. 制御構造と早期リターン (Control Structures & Early Return)

### ① ガード節と左端揃え (Keep the Normal Code Path Left-Aligned)
- エラーハンドリングや不正条件は `if err != nil` のガード節で即座に return / break / continue する。
- 不要な `else` は完全に排除し、正常系処理をネストさせずにインデント浅く（左揃え）記述する。

```go
// ⭕ 慣用的なGo (Effective Go):
f, err := os.Open(name)
if err != nil {
    return fmt.Errorf("open failed: %w", err)
}
defer f.Close()

d, err := f.Stat()
if err != nil {
    return fmt.Errorf("stat failed: %w", err)
}
use(d)
return nil
```

### ② Type Switch の活用
- リフレクション（`reflect`）を避け、型判定には型スイッチを用いる：
```go
switch v := value.(type) {
case string:
    return v
case fmt.Stringer:
    return v.String()
default:
    return fmt.Sprint(v)
}
```

---

## 3. データ構造と初期化 (Data & Initialization)

### ① `new` vs 複合リテラル (Composite Literals)
- `new(T)` はメモリを確保しゼロ値をポインタとして返すが、フィールド初期化が必要な場合は**複合リテラル**を優先する。
```go
// ⭕ 推奨: 複合リテラル（フィールド名明示）
return &ProcessConfig{
    Name:       "api-server",
    MaxRetries: 3,
    Timeout:    30 * time.Second,
}
```

### ② `make` によるスライス・マップ・チャネル初期化
- スライス、マップ、チャネルは内部データ構造の初期化が必要なため、必ず `make` を使用する。
- サイズが既知のスライスは必ず初期容量（`capacity`）を予約し、再アロケーションを防ぐ：
```go
items := make([]Item, 0, len(inputs))
```

---

## 4. インターフェース設計の極意 (Interfaces)

### ① Accept Interfaces, Return Structs (インターフェースを受け取り、構造体を返せ)
- 関数やコンストラクタは、可能な限り**具象構造体のポインタ（または値）**を返す。
- 呼び出し側（引数）は、必要とする最小限の振る舞いを定義した**インターフェース**で受け取る。
- これにより、ライブラリ側が無駄なインターフェースを押し付ける「Interface Pollution（インターフェース汚染）」を根絶する。

### ② 単一〜極小インターフェース (Small Interfaces)
- インターフェースのメソッド数は 1〜3 個を原則とする。
- 巨大な Fat Interface（10個以上のメソッド）は、複数の小さなインターフェースに分解・合成（Embedding）する。

---

## 5. 並行処理イディオム (Concurrency)

### ① メモリ共有の原則
> **"Do not communicate by sharing memory; instead, share memory by communicating."**
> （メモリを共有して通信するな。通信によってメモリを共有せよ。）

- スレッド間の状態同期は、安易なグローバル変数＋Mutexではなく、チャネルを介したメッセージパッシングを第一選択とする。
- ただし、単一構造体内部のキャッシュ保護など局所的な同期には `sync.Mutex` や `atomic` を適切に使い分ける。

### ② Goroutine リーク防止とライフサイクル管理
- Goroutine を起動する際は、**「いつ、どうやって終了するか」** がコード上で100%明白でなければならない。
- 終了チャネル（`done chan struct{}`）または `context.Context`（`ctx.Done()`）を必ず監視し、親がキャンセルされたら即座に終了する。

```go
go func() {
    for {
        select {
        case <-ctx.Done():
            return // 必ず安全に抜ける
        case task := <-taskCh:
            process(task)
        }
    }
}()
```

---

## 6. エラーハンドリングとパニック (Errors & Panic)

### ① エラーは制御フローの第1級市民
- エラーは最後の戻り値として返し、呼び出し側で必ず検査する（握りつぶし `_ = err` は厳禁）。
- エラーを上位に返す際は `fmt.Errorf("context message: %w", err)` でラップし、呼び出し文脈を付加する。
- センチネルエラーの判定には `errors.Is(err, ErrNotFound)`、特定のエラー型の取得には `errors.As(err, &targetErr)` を使う。

### ② Panic を通常の制御フローに使わない
- `panic` は、初期化失敗でこれ以上プロセスが継続できない致命的状況（例: 正規表現コンパイル `regexp.MustCompile`）にのみ限定する。
- パッケージ外部へ `panic` を漏洩させてはならない。パブリックAPIの境界で `recover` し、通常の `error` として呼び出し元に返す。

---

## 7. Effective Go 監査チェックリスト

- [ ] `gofmt` / `goimports` を適用しているか？
- [ ] パッケージ名は短く、小文字単語で、`util`/`common` を使っていないか？
- [ ] ゲッターに `Get` を付けていないか？
- [ ] `if err != nil` のガード節で早期リターンし、正常系を左揃えにしているか？
- [ ] 不要なインターフェースを作らず、具象構造体を返しているか？
- [ ] Goroutine に明確な終了条件（Context / Doneチャネル）があるか？
- [ ] エラーを `%w` でラップし、握りつぶしていないか？
- [ ] パニックをパッケージ外に垂れ流していないか？

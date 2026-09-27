<!--
Copyright 2026 [Copyright Holder]

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Author: [YOUR_NAME]
-->

# マニフェスト作成支援機能：製品要件定義 (PRD) & ユーザー体験 (UX) 設計書
**Manifest Discovery Engine: PRD, KISS-Driven Whitelist Workflow & User Experience**

---

## 1. エグゼクティブサマリー & KISS設計原則

### 1.1 背景と課題
既存サーバーから構成を吸い上げて `manifest.yaml` を作る機能において、従来の「設定オプションを並べたハイブリッド」や「ホワイトリストとブラックリスト（ignore）の多重評価」は、以下の理由でユーザーを疲弊させます：
- **認知的負荷の増大**: 「どのignoreパターンが効いているか」「ネストされたルールでどっちが勝つか」を推測しなければならない。
- **設定ファイルの散乱**: マニフェスト、計画ファイル、ignoreファイルの多重管理が発生する。
- **事故の不安**: 意図せぬファイルが漏れ出たり、逆に必要なファイルが除外されていないか確信が持てない。

### 1.2 KISSの法則に基づく一貫した新ポリシー
> **【Single Source of Truth (SSOT) 原則】**
> - **設定ファイルは `discovery-plan.yaml` の 1 つだけ。ブラックリスト別ファイル（`.ytagarasuignore`）は完全廃止。**
> - **マニフェストに入るものは、プランに書かれた行のみ（100% ホワイトリスト）。**
> - **除外の操作は、エディタで「不要な行を消す（または `#` でコメントアウトする）」だけ。**

---

## 2. ユーザー体験 (UX) の全体像：3ステップの1本道ジャーニー

ユーザーは複雑なルールを一切覚える必要がありません。以下の **「下見 ➔ 取捨選択 ➔ 確定」** の一直線のフローで完結します。

```mermaid
graph LR
    Step1["Step 1: 下見 (Survey)<br/>ytagarasu discover survey"] -->|安全な読み取り| Plan["単一の設定書<br/>discovery-plan.yaml"]
    Plan -->|Step 2: 取捨選択 (Review)<br/>不要な行をエディタで消すだけ| TrimmedPlan["確定プラン<br/>(100% ホワイトリスト)"]
    TrimmedPlan -->|Step 3: 確定 (Generate)<br/>ytagarasu discover generate| Manifest["完成マニフェスト<br/>manifest.yaml"]

    style Step1 fill:#1e293b,stroke:#38bdf8,stroke-width:2px,color:#f8fafc
    style Plan fill:#1e293b,stroke:#f59e0b,stroke-width:2px,color:#f8fafc
    style TrimmedPlan fill:#1e293b,stroke:#a855f7,stroke-width:2px,color:#f8fafc
    style Manifest fill:#0f172a,stroke:#22c55e,stroke-width:2px,color:#f8fafc
```

---

## 3. ステップ別 ユーザー体験 (UX) の詳細

### Step 1: 【下見】`ytagarasu discover survey`
サーバーに変更を一切加えない完全読み取り専用（Read-Only）で実行されます。
OS のディレクトリを無差別に掘るのではなく、**「稼働中の systemd サービス」を基点として、関連する実行バイナリと設定ファイルをピンポイントで特定**します。

#### ターミナルでの体験
```text
$ ytagarasu discover survey

================================================================
   🦅 ytagarasu Discovery Survey (サーバー構成下見)
================================================================
[1/2] 稼働中サービスと構成要素を検出中...
  ✔ billing-worker.service を検出 (Exec: /usr/local/bin/billing-worker)
  ✔ payment-gw.service を検出     (Exec: /usr/local/bin/payment-gw)
  ✔ ytagarasu-agent.service は管理対象外として自動スキップ

[2/2] 構成プランを作成中...
  ✔ 'discovery-plan.yaml' を生成しました。

================================================================
👉 次のステップ:
   1. 'discovery-plan.yaml' をエディタで開き、移行不要な行を削除/コメントアウトしてください。
   2. 'ytagarasu discover generate' を実行してマニフェストを確定します。
================================================================
```

---

### Step 2: 【取捨選択】`discovery-plan.yaml` の編集
生成された YAML は、極めて平易なホワイトリスト定義です。
ユーザーは慣れ親しんだエディタ（VS Code, Vim 等）で開き、**不要なものを消す（または `#` を付ける）だけ**です。

#### ユーザーが見るファイル（`discovery-plan.yaml`）
```yaml
# discovery-plan.yaml
# ytagarasu が検出した構成候補です。
# 不要な項目は行ごと削除するか、先頭に '#' を付けてコメントアウトしてください。
version: "1.0"

targets:
  # 検出された systemd サービスと関連コンポーネント
  services:
    - name: "billing-worker.service"
      binary: "/usr/local/bin/billing-worker"
      configs:
        - "/etc/billing/worker.conf"
      # バイナリの移行モード:
      #   ingest: false (デフォルト: CI/CDの配布URLを後で記載するTODOモード)
      #   ingest: true  (現物のバイナリをそのままバンドルに吸い上げるモード)
      ingest: false

    # ▼ 不要なサービスは行をコメントアウトするだけで安全に除外！
    # - name: "payment-gw.service"
    #   binary: "/usr/local/bin/payment-gw"
    #   configs:
    #     - "/etc/payment/payment.env"
    #   ingest: false

  # サービスに紐づかない追加の静的ディレクトリ（必要な場合のみ残す）
  custom_dirs:
    - path: "/opt/billing/templates"
    # - path: "/opt/billing/tmp"   <-- ゴミディレクトリは消すだけ
```

#### ユーザー体験の利点
- **学習コストゼロ**: `.gitignore` のようなワイルドカードや否定文法（`!path`）を覚える必要がない。
- **WYSIWYG（見たままが得られる）**: ここに残っている行以外は、1バイトたりともマニフェストに入らない。
- **チームレビューが容易**: プルリクエストで差分（`git diff discovery-plan.yaml`）を見るだけで、何がデプロイ対象として承認されたかが一目瞭然。

---

### Step 3: 【確定】`ytagarasu discover generate`
編集した `discovery-plan.yaml` を入力として実行し、最終的な `manifest.yaml` を生成します。

#### ターミナルでの体験
```text
$ ytagarasu discover generate

================================================================
   🦅 ytagarasu Discovery Generate (マニフェスト確定生成)
================================================================
[1/2] discovery-plan.yaml を読み込み中...
  ✔ 採択されたサービス: 1 件 (billing-worker.service)
  ✔ 除外（未記載/コメントアウト）: 1 件を完全にスキップ

[2/2] manifest.yaml を出力中...
  ✔ バイナリメタデータを記録 (SHA256: e3b0c44298fc1c149afbf4c8996fb924...)
  ✔ 設定ファイルを配置定義に追加 (/etc/billing/worker.conf)
  ✔ 'manifest.yaml' を正常に生成しました！

================================================================
🎉 完了: 次は 'ytagarasu bundle create' でオフライン配布バンドルを作成できます。
================================================================
```

---

## 4. ユーザーが直面する 2 つの現実への対応（落とし所のUX）

### 4.1 バイナリの元リポジトリが分からない場合（Mode A vs Mode B）
ユーザーが既存サーバーのバイナリを吸い上げる際、最大の課題は「このバイナリがどこからビルドされたか分からない」ことです。
これに対し、`discovery-plan.yaml` のフラグ 1 つで直感的に切り替えられる体験を提供します。

| モード | ユーザーの意図 | プランファイルでの指定 | 生成されるマニフェストの挙動 |
| :--- | :--- | :--- | :--- |
| **Mode B<br/>(デフォルト)** | **「将来はCI/CDで管理したい（GitOps指向）」**<br/>バイナリ実体は含めず、クリーンに移行したい。 | `ingest: false`<br/>（デフォルト） | マニフェストにバイナリ情報（サイズ、ELF形式、SHA256）を記録し、URL欄に `# TODO: 配布元URLを記載` とコメントを出力。 |
| **Mode A<br/>(実体吸い上げ)** | **「元のソースもURLも分からない（レガシー延命）」**<br/>とにかく今動いているものをそのまま新環境で動かしたい。 | `ingest: true`<br/>（または `--ingest`） | サーバー上の現物バイナリをそのまま吸い上げ、ytagarasu バンドル内に格納。オフライン環境ですぐに展開可能。 |

### 4.2 秘密鍵や機密情報の取り扱い（SIRTビルトイン安全弁）
ユーザーが「設定ファイル（`/etc/billing/worker.conf`）にDBパスワードや秘密鍵が含まれているかもしれない」と不安に感じる必要はありません。
- **ツールの自動サニタイズ**: 設定ファイル内に秘密鍵（`-----BEGIN PRIVATE KEY-----`）や明らかなトークンを検知した場合、ツールが自動で `<REDACTED_SECRET>` にマスクし、警告を表示します。
- ユーザーに複雑な除外ルールを書かせるのではなく、**ツール側が絶対に事故を起こさないフェイルセーフ**を提供します。

---

## 5. ユースケース別 クイックリファレンス

ユーザー向けマニュアル（`docs/manual.md`）には、以下のシンプルな利用シナリオを掲載します。

| ユーザーのやりたいこと | 実行手順 | 体験のポイント |
| :--- | :--- | :--- |
| **サーバー全体から安全に必要なものだけ選びたい** | 1. `ytagarasu discover survey`<br/>2. `discovery-plan.yaml` で不要な行を消す<br/>3. `ytagarasu discover generate` | 下見で一覧化されるため、サーバーの知識が少なくても迷わない。 |
| **特定のサービスだけピンポイントで移行したい** | `ytagarasu discover --service billing-worker.service` | 指定したサービスと、その ExecStart/設定ファイルだけを即座にマニフェスト化。他には一切触れない。 |
| **現物バイナリをそのまま持ち出したい（レガシー）** | `ytagarasu discover --service billing-worker.service --ingest` | リポジトリ不要。稼働中バイナリをそのまま吸い上げて即座に移行完了。 |

---

## 6. まとめ

- **KISSの徹底**: 複雑なルール競合（最長一致、否定パターン等）を完全に排除。
- **安心のホワイトリスト**: `discovery-plan.yaml` に残った行だけがマニフェストに入る。
- **直感的な操作**: 要らないものはエディタで行を消すだけ。

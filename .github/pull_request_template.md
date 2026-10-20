<!-- 
  [Language Notice / 言語選択]
  You can fill out this PR in either Japanese or English.
  日本語・英語のどちらでも記入可能です。不要なコメントガイドは削除して構いません。
-->

## 📝 概要 / Summary
<!-- 
  [JA] このPRの目的や変更内容について簡潔に記述してください。
  [EN] Clearly describe the purpose, motivation, and context of this pull request.
-->


## 🔗 関連する Issue / Related Issues
<!-- 
  [JA] 関連するIssue番号を記載してください (例: Closes #123)
  [EN] Link related issues (e.g., Closes #123)
-->
- 

## 📦 変更カテゴリ / Change Category
<!-- 
  [JA] 該当するカテゴリにチェック [x] を入れてください。
  [EN] Check all categories that apply to this change.
-->
- [ ] CLI & プロセススーパーバイザー / CLI & Process Supervisor (cmd/ytg, pkg/supervisor)
- [ ] バンドル・暗号署名・マニフェスト / Bundles, Crypto & Manifest (internal/bundle, internal/manifest)
- [ ] パッケージ解決エンジン / Package Engine & Dependency Resolver (internal/pkgengine, internal/agent)
- [ ] 配信サーバー & HTMX UI / Offline Server & HTMX UI (cmd/ytagarasu-server, internal/server, internal/web)
- [ ] 差分検知 & 作成支援 / Drift Detection & Manifest Discovery (internal/delta, internal/discover)
- [ ] CI / CD & ワークフロー / CI, CD & Workflows (GitHub Actions, GoReleaser)
- [ ] ドキュメント & テンプレート / Documentation & Templates
- [ ] その他 / Other

## 🛠️ 変更内容 / Changes
<!-- 
  [JA] どのような変更を加えたかを箇条書きで具体的に記載してください。
  [EN] Bullet points detailing the architectural or implementation changes.
-->
- 

## 🧪 検証チェックリスト / Verification Checklist
<!-- 
  [JA] 実施した検証にチェック [x] を入れてください。
  [EN] Check each verification step you have completed.
-->
- [ ] `make generate` を実行し、`git diff --exit-code` で生成コードに差分がないことを確認した / Verified no diff in generated code
- [ ] `make lint` が 0 issues で成功することを確認した / golangci-lint passed with 0 issues
- [ ] `make test` が全テスト PASS で成功することを確認した / All unit and integration tests passed
- [ ] `make build` が正常に完了することを確認した / Application builds successfully
- [ ] `make license-check` でライセンスヘッダーが適切であることを確認した / License headers verified
- [ ] OpenAPI 変更時: `make openapi-lint` が正常に通過することを確認した / OpenAPI lint passed (if applicable)
- [ ] Terraform 変更時: `terraform fmt -check`, `validate`, `tflint`, `tfsec` を確認した / Terraform validated (if applicable)
- [ ] AI スキル 変更時: `make check` で文法・フロントマターエラーがないことを確認した / AI skill syntax checked (if applicable)

## 🚨 注意事項・懸念点 / Notes & Concerns
<!-- 
  [JA] 破壊的変更、互換性の懸念、パフォーマンスへの影響などがあれば記述してください。
  [EN] Mention any breaking changes, deprecations, migration steps, or performance impacts.
-->
- None / なし

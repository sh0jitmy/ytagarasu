# Changelog

## [v0.0.1](https://github.com/sh0jitmy/ytagarasu/commits/v0.0.1) - 2026-10-09

- feat(manifest): implement deployment manifest schema, validator and CLI tool (#2) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/11
- feat(pkgengine): implement recursive APT and RPM dependency resolver with concurrent downloader (#3) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/12
- feat(bundle): implement bundle packager, risk evaluator, and cryptographic verifier (#4) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/14
- feat(server): implement offline artifact server, bundle importer, CAS storage, and virtual repositories (#5) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/15
- feat(agent): implement autonomous deploy agent and atomic config rollback (#6, #7) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/16
- feat(audit): implement tamper-proof SHA-256 audit hash chain, bundle expiration check, and verification CLI (#8) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/17
- feat(ansible): implement deploy_agent initial bootstrap role and agent config loader (#9) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/18
- feat(delta): implement delta bundle diff engine, packaging, and base release verification (#10) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/19
- feat(ui): implement embedded HTMX dashboard, audit log viewer, and verification actions (#20) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/23
- test(e2e): implement multi-tier agent-server deploy and HTMX UI visual verification suite (#21) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/24
- docs: rewrite README with ytagarasu architecture and create comprehensive user manual (#22) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/25
- feat(ui): enhance dashboard UX with service inspect modal, audit log filtering, and toast feedback (#26) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/29
- test(pkg): expand package manager coverage (APT, DNF, Pip, Docker) and add weekly matrix workflow (#27) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/30
- docs(manual): synchronize documentation with UX enhancements, package matrix, and full repository state (#28) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/31
- fix(e2e): eliminate mojibake in GitHub Pages snapshots with CJK font support by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/32
- feat(test): add recursive dependency resolution, live smoke testing, Dewy deployment, and Pages deploy (#33) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/35
- feat(agent): support multi-ecosystem package ordering, dependency resolution, and combined E2E tests (#34) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/36
- feat(discover): implement whitelist-based manifest discovery engine with KISS workflow (#37) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/38
- fix(reports): resolve GitHub Pages 404 for matrix report and integrate manifest E2E portal (#39) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/40
- fix(e2e): capture headless Chrome stderr and unbuffer Python test logs (#42) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/43
- feat(cli): introduce ergonomic 3-letter binary 'ytg' and embedded process supervisor engine (fixes #45) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/46
- ci(deps): bump the github-actions group across 1 directory with 10 updates by @dependabot[bot] in https://github.com/sh0jitmy/ytagarasu/pull/44
- feat(web): integrate supervision panel and CUD tactical dashboard (#52) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/53
- fix(web): 統合ダッシュボードのスタイル欠落解消・1秒リアルタイムポーリング・リッチUI画面提供(Health/Metrics)および動的CPU変動の実装 by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/54

## [v0.0.2](https://github.com/sh0jitmy/go_template/compare/v0.0.1...v0.0.2) - 2026-06-28

- add terraform and coverage check  by @sh0jitmy in https://github.com/sh0jitmy/go_template/pull/8
- refactor: HTTPレイヤー整理、PostgreSQL接続対応、およびPR自動作成ツールの導入 by @sh0jitmy in https://github.com/sh0jitmy/go_template/pull/10

## [v0.0.1](https://github.com/sh0jitmy/go_template/compare/v/v0.0.1...v0.0.1) - 2026-06-28

- tagpr: fix label by @sh0jitmy in https://github.com/sh0jitmy/go_template/pull/6

## [v0.0.1](https://github.com/sh0jitmy/go_template/commits/v/v0.0.1) - 2026-06-27

- add pinact and pinned by @sh0jitmy in https://github.com/sh0jitmy/go_template/pull/1
- build(deps): bump actions/checkout from 4.3.1 to 7.0.0 by @dependabot[bot] in https://github.com/sh0jitmy/go_template/pull/5
- build(deps): bump goreleaser/goreleaser-action from 6.4.0 to 7.2.2 by @dependabot[bot] in https://github.com/sh0jitmy/go_template/pull/4
- build(deps): bump actions/setup-go from 5.6.0 to 6.5.0 by @dependabot[bot] in https://github.com/sh0jitmy/go_template/pull/3

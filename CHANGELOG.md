# Changelog

## [v0.0.1](https://github.com/sh0jitmy/ytagarasu/commits/v0.0.1) - 2026-10-23

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
- feat(web): add heartbeat indicator, dynamic diagnostic cycle counter, microsecond jitter, and 1s polling to System Health panel by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/55
- build(deps): bump the minor group with 7 updates by @dependabot[bot] in https://github.com/sh0jitmy/ytagarasu/pull/56
- ci(deps): bump the github-actions group with 3 updates by @dependabot[bot] in https://github.com/sh0jitmy/ytagarasu/pull/57
- feat(web/ci): real-time 1s heartbeat telemetry and multi-tier E2E reports on GitHub Pages with zero-404 link audit gate by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/58
- feat(skills): introduce KISS principle and Effective Go skills by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/59
- feat(supervisor): implement zero-dependency rotating logger and disk protection (#60) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/64
- feat(cli): add shell completion command for ytg (bash, zsh, powershell, fish) (#63) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/65
- feat(supervisor): implement cgroup v2 memory.events monitoring and OOM killer rescue (#61) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/66
- feat(web): add Prometheus process metrics collector and native health endpoints (#62) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/67
- docs: comprehensive update of README for ytg CLI, log rotation, and metrics (#70) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/71
- feat(cli): enhance 'ytg' root commands with status, inspect, and restart (#68) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/69
- fix(ci): ensure ytagarasu UI reports and snapshots render on GitHub Pages (#75) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/76
- feat(agent): support offline apt/dnf repo provisioning and sudo escalation (#72) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/79
- test(e2e): implement air-gap scenario test suite, runbook, snapshots and workflow dispatch (#80) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/81
- feat(cli): 3文字CLI 'ytg' における構造化 JSON 出力 (--json) オプション標準化 (#78) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/82
- feat(cli): 3文字CLI 'ytg manifest init' による対話型マニフェスト雛形ジェネレータの追加 (#73) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/83
- feat(agent): support direct local package install with sha256 verification (#77) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/85
- feat(manifest): support ed25519 cryptographic signing and verification (#84) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/86
- build(deps): bump golang.org/x/crypto from 0.57.0 to 0.58.0 in the minor group by @dependabot[bot] in https://github.com/sh0jitmy/ytagarasu/pull/87
- ci(deps): bump goreleaser/goreleaser-action from 7.2.3 to 7.3.0 in the github-actions group by @dependabot[bot] in https://github.com/sh0jitmy/ytagarasu/pull/88
- feat(agent): enforce manifest ed25519 signature guard before deployment (#89) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/90
- test(e2e): add Linux real APT package validation E2E test (#74) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/91
- feat(cli): visualize manifest cryptographic integrity in 'ytg status' and 'inspect' (#92) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/93
- feat(test): 外部オンプレミスOSS (Musubi フルスタック: PostgreSQL 16 & Grafana) のデプロイ・看取り・ダッシュボード稼働確認 E2E シナリオテストとCIワークフローの追加 (#95, #97) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/96
- feat(manifest): APT/DNF外部リポジトリ動的追加の要件補全とDockerハイブリッド構成Musubiマニフェスト・シナリオテストの追加 (#98) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/99
- feat(test): Dockerハイブリッド構成におけるUSB可搬媒体書き出しデータ・マニフェスト生成と完全エアギャップ環境デプロイ E2E シナリオテストの追加 (#100) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/101
- feat(container): Docker自動バンドル・ロード・看取り・compose移行支援機能と完全エアギャップ仕様書 (#102) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/103
- feat(supervisor): ホストポート競合検知・Dockerデーモン到達性診断・指数バックオフ自律看取りと耐障害E2Eシナリオテストの追加 (#104) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/108
- feat(bundle): 0バイト・破損gzip・途中で切れたtarの事前検知と整合性堅牢化E2Eシナリオテストの追加 (#104) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/109
- feat(manifest): docker-compose変換堅牢化・Desired State差分検知 (ytg diff)・手動承認安全弁とAdversarial E2Eテストの追加 (#104) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/110
- docs(all): 八咫烏アイコン刷新・golangci-lint CI新設・ドキュメント日英バイリンガル対応 by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/111
- refactor(fsutil): Effective Go & KISS原則に基づくファイルSHA-256ハッシュ計算・検証の共通化と自作文字列トリムの標準置換 (#112) by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/113
- feat(supervisor): Docker Network 自動 Ensure・ライフサイクル管理及びコンテナ間 DNS 解決の保証 by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/116
- feat(manifest): docker-compose.yml の直接・透過的取り込み（Zero-Conversion Ingestion）とコンテナ専用マニフェスト対応 by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/117
- test(e2e): add hospital HIS/OT chaos self-healing and manifest build pipeline E2E scenario tests with demo media by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/119
- feat(bundle): implement multi-part chunking and zero-disk-waste streaming reassembly by @sh0jitmy in https://github.com/sh0jitmy/ytagarasu/pull/121

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

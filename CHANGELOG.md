# Changelog

## [0.2.0](https://github.com/joshuapeters/klanky/compare/v0.1.0...v0.2.0) (2026-05-02)


### Features

* add install.sh for one-line install ([#25](https://github.com/joshuapeters/klanky/issues/25)) ([b5f01a3](https://github.com/joshuapeters/klanky/commit/b5f01a3ec91c48464e0c19771fad96471190feea))

## [0.1.0](https://github.com/joshuapeters/klanky/compare/v0.0.2...v0.1.0) (2026-05-02)


### ⚠ BREAKING CHANGES

* removes the `feature` and `task` command trees and the phase-based execution model. Projects must now be created/linked via `klanky project new|link`, work is added via `klanky issue add`, and the runner is invoked as `klanky run --project <slug>` with ordering derived from GitHub-native issue dependencies. Existing `.klankyrc.json` files from 0.0.x are not compatible and need to be re-initialized.

### Features

* rewrite around multi-project model with issue-DAG over Projects v2 ([#23](https://github.com/joshuapeters/klanky/issues/23)) ([f55d344](https://github.com/joshuapeters/klanky/commit/f55d344ec2661808a10355c3cb6f0fe535fb69c1))

## [0.0.2](https://github.com/joshuapeters/klanky/compare/v0.0.1...v0.0.2) (2026-05-01)


### Bug Fixes

* thread version metadata as params and drop fprintf err checks ([#18](https://github.com/joshuapeters/klanky/issues/18)) ([654d998](https://github.com/joshuapeters/klanky/commit/654d998c9ac2546209c1b6008d1166f3e1052ce7))

## 0.0.1 (2026-05-01)


### Features

* add 'klanky feature new' command ([1d99a60](https://github.com/joshuapeters/klanky/commit/1d99a609860d8aaf9f4acd9f4a37934fb3c08c03))
* add 'klanky init' command for project bootstrap ([f047ef4](https://github.com/joshuapeters/klanky/commit/f047ef458f5039a2b6af83780d7d28c16cd94662))
* add 'klanky project link' command with schema validation ([4b20dfd](https://github.com/joshuapeters/klanky/commit/4b20dfdc84eae11a016d3612d9ea4c5c9e7e2367))
* add 'klanky task add' command with sub-issue linking ([f139d39](https://github.com/joshuapeters/klanky/commit/f139d39380679f7517fe8a4058f720ec08e7a412))
* add config struct, LoadConfig, SaveConfig with round-trip tests ([0add1ce](https://github.com/joshuapeters/klanky/commit/0add1ce0c8997b80534e2fccc15ed2a547918645))
* add klanky run stub command with arg validation ([db047e1](https://github.com/joshuapeters/klanky/commit/db047e1308d9e0181bdcff964868b21b02931360))
* add PrintJSONLine for planning-agent output contract ([dd7ae8c](https://github.com/joshuapeters/klanky/commit/dd7ae8c815d5157643d2891acdde3d5a3f3bdbd8))
* add release tooling (release-please + goreleaser) ([#9](https://github.com/joshuapeters/klanky/issues/9)) ([7ce9d63](https://github.com/joshuapeters/klanky/commit/7ce9d636dceee08511f5ea8c2b6e44aae4e98761))
* add RunGraphQL helper with variable typing and error parsing ([9658d7b](https://github.com/joshuapeters/klanky/commit/9658d7b812c2cff6a5ee126b9f698ef2157bd580))
* add Runner interface with RealRunner and FakeRunner ([0c90258](https://github.com/joshuapeters/klanky/commit/0c9025848665abe98670fc5e26ee8175e714601a))
* add schema constants and ValidateProject ([f13e968](https://github.com/joshuapeters/klanky/commit/f13e968dc3d1c74aa69c481dc694cb29f933265e))
* agent execution — spawn claude -p, verify branch + PR ([ca3b675](https://github.com/joshuapeters/klanky/commit/ca3b675742ad46263ca04f96d117f356e124f4b5))
* bootstrap go module and cobra root command ([df47775](https://github.com/joshuapeters/klanky/commit/df477756f55f5e3a080f9f1de63fb68f5fb24c72))
* breadcrumb format + sentinel-based attempt counting ([448c873](https://github.com/joshuapeters/klanky/commit/448c873e6ccf63cb15a6b501f470836462799f36))
* end-of-run summary with text/tabwriter table ([d69542e](https://github.com/joshuapeters/klanky/commit/d69542e4a0453a2b2ca7d55d650bb5fc37c7dc69))
* envelope template for claude -p invocation ([2afdf01](https://github.com/joshuapeters/klanky/commit/2afdf012ff7de7e645fec6b6ae26a5ed67c80bd2))
* per-feature lock file with PID liveness + silent takeover ([dd0f0e5](https://github.com/joshuapeters/klanky/commit/dd0f0e5ee8d96db60718dcef6f60d409e041eec4))
* progress logger with timestamped event lines ([c319836](https://github.com/joshuapeters/klanky/commit/c319836b5417ccd51fb23108bcd7bf845c783af6))
* pure-function reconcile implementing the 11-row matrix ([b02d552](https://github.com/joshuapeters/klanky/commit/b02d552e8e1cf74b028c09eea3a4fbc3a1d157ba))
* remove worktree after successful in-review status write ([60e0669](https://github.com/joshuapeters/klanky/commit/60e0669cd1ba27d13edbd44ba281ed577d4064cb)), closes [#7](https://github.com/joshuapeters/klanky/issues/7)
* snapshot fetch — one GraphQL + one PR list per feature ([0de1583](https://github.com/joshuapeters/klanky/commit/0de1583e3f78f074d4f456e343b27205457c71e5))
* status writer with 3-retry exponential backoff ([380ba89](https://github.com/joshuapeters/klanky/commit/380ba89109987527037f872efa401f079b633ad2))
* top-level runner orchestration — lock, reconcile, parallel spawn, summary ([9b32b89](https://github.com/joshuapeters/klanky/commit/9b32b89983e87b4e79d8717870163e892b885501))
* work-queue selection — current phase + eligibility partitioning ([c5df0b3](https://github.com/joshuapeters/klanky/commit/c5df0b3cf661f30b2056abee51b8d12be86c34a6))
* worktree management — wipe-and-rebuild for clean retries ([d509be1](https://github.com/joshuapeters/klanky/commit/d509be10b2ea8d3c0d0ee5d668630b82579e521e))


### Bug Fixes

* drop merged/closed from gh pr list — current gh doesn't expose merged ([62ab08c](https://github.com/joshuapeters/klanky/commit/62ab08ce7e558452d1f910cfbd6cede1af4727cb))
* SIGINT-aware context + separate sentinel for reconcile breadcrumbs ([4309ab2](https://github.com/joshuapeters/klanky/commit/4309ab2707dbfba991c9ad8944a3a305639a1e07))
* write Status=Needs Attention on worktree-setup or agent-spawn failure ([575625a](https://github.com/joshuapeters/klanky/commit/575625a982034e02da3f2b1536d1fa885f8bcb2a))

# Changelog

## [0.1.2](https://github.com/grafana/generate-policy-bot-config/compare/v0.1.1...v0.1.2) (2024-12-10)


### 🔧 Miscellaneous Chores

* **deps:** bump actions/attest-build-provenance from 1.4.4 to 2.0.1 ([72a49ab](https://github.com/grafana/generate-policy-bot-config/commit/72a49abf4b81fc4feff4a43f37acc02dedd91ddf))
* **deps:** bump actions/attest-sbom from 1.4.1 to 2.0.1 ([e7ebded](https://github.com/grafana/generate-policy-bot-config/commit/e7ebdedcbeebef474718ab3083b1ee715db41408))
* **deps:** bump anchore/sbom-action from 0.17.7 to 0.17.8 ([#28](https://github.com/grafana/generate-policy-bot-config/issues/28)) ([8b1fb8b](https://github.com/grafana/generate-policy-bot-config/commit/8b1fb8b2f70ae65af11e73d341d194b4fc91b8ea))
* **deps:** bump docker/build-push-action from 6.9.0 to 6.10.0 ([b70678d](https://github.com/grafana/generate-policy-bot-config/commit/b70678d2ff76a16f4394d2ed6432a67895703a10))
* **deps:** bump docker/metadata-action from 5.5.1 to 5.6.1 ([#29](https://github.com/grafana/generate-policy-bot-config/issues/29)) ([3b47b75](https://github.com/grafana/generate-policy-bot-config/commit/3b47b756713b94f1b179c1fd1512d4bb683ad8f2))
* **deps:** bump github.com/stretchr/testify from 1.9.0 to 1.10.0 ([#30](https://github.com/grafana/generate-policy-bot-config/issues/30)) ([c86ad90](https://github.com/grafana/generate-policy-bot-config/commit/c86ad90958f467334c69f738c011106bf763d69d))
* **deps:** bump golang from 1.23.3-alpine3.20 to 1.23.4-alpine3.20 ([030f356](https://github.com/grafana/generate-policy-bot-config/commit/030f3564ec1e9e09ced38af6e2e72605f55a0d6c))
* **deps:** bump golang.org/x/term from 0.25.0 to 0.26.0 ([67c5a39](https://github.com/grafana/generate-policy-bot-config/commit/67c5a3905d538fc59e132f4bc611a1998a772675))
* **deps:** bump golang.org/x/term from 0.26.0 to 0.27.0 ([54e7af7](https://github.com/grafana/generate-policy-bot-config/commit/54e7af740fef5e42e709b395aa13f0fab3e61850))

## [0.1.1](https://github.com/grafana/generate-policy-bot-config/compare/v0.1.0...v0.1.1) (2024-11-08)


### 🐛 Bug Fixes

* **ci:** push from release event too and handle multiple tags ([1cbca2c](https://github.com/grafana/generate-policy-bot-config/commit/1cbca2cc6b957de6e6cec34ab1349165ff1c127b))

## 0.1.0 (2024-11-08)


### 🎉 Features

* **all:** add CI, a release process and Dependabot ([b8055cf](https://github.com/grafana/generate-policy-bot-config/commit/b8055cf3074f74a475f271e5e9ebf95d225ff3b0))
* **build:** Add a Dockerfile ([5eb92d3](https://github.com/grafana/generate-policy-bot-config/commit/5eb92d3be6b871f879529d2250bcfb45b5eab7db))
* **ci:** build binaries natively, avoiding emulation ([663e4ea](https://github.com/grafana/generate-policy-bot-config/commit/663e4ea1b23690505d6ac00a571a2a061a95ff36))
* **ci:** generate and push sbom and build provenance attestations ([54ea116](https://github.com/grafana/generate-policy-bot-config/commit/54ea1169744e24035727f606d008a86264aa1a90))
* print filename we're merging with ([23c0d91](https://github.com/grafana/generate-policy-bot-config/commit/23c0d9133dab64fd1a32a73f332e9178dcf1dc45))


### 🐛 Bug Fixes

* **ci:** drop local validate rule ([ac72894](https://github.com/grafana/generate-policy-bot-config/commit/ac72894827185b3ec5cd8a50fdad91922b884605))
* **ci:** run on `published` releases ([b0c19fe](https://github.com/grafana/generate-policy-bot-config/commit/b0c19feaab09f53af9b70033b10bb2e557f4fbbc))
* remove rule properly ([38c376c](https://github.com/grafana/generate-policy-bot-config/commit/38c376c3bd2be5f00ad13b3da2c6a2df6be8d25c))


### 📝 Documentation

* add some more config instructions to the README ([457e203](https://github.com/grafana/generate-policy-bot-config/commit/457e203271c966ff4bf58f4d6935b77714f9352c))
* **README:** update project name ([08e65bc](https://github.com/grafana/generate-policy-bot-config/commit/08e65bcaaa09863426a873cbb6b849c577817259))


### 🔧 Miscellaneous Chores

* **ci:** replicate path filters ([9d356ce](https://github.com/grafana/generate-policy-bot-config/commit/9d356cee16cab232acbd2fbe587b5f359c7bcac5))
* **deps:** bump actions/checkout from 4.2.0 to 4.2.2 ([619596e](https://github.com/grafana/generate-policy-bot-config/commit/619596e72223f85260f9aadb45386230130e2763))
* **deps:** bump actions/setup-go from 5.0.2 to 5.1.0 ([c7e715b](https://github.com/grafana/generate-policy-bot-config/commit/c7e715bb3775cf2691fa7b82694cd82f20a177bb))
* **deps:** bump docker/setup-buildx-action from 3.6.1 to 3.7.1 ([a93f6cf](https://github.com/grafana/generate-policy-bot-config/commit/a93f6cfe296f2321db63aa9d083ff44bedb00c87))
* **deps:** bump golang from 1.23.2-alpine3.20 to 1.23.3-alpine3.20 ([#13](https://github.com/grafana/generate-policy-bot-config/issues/13)) ([3bc345b](https://github.com/grafana/generate-policy-bot-config/commit/3bc345b084d2755ed7d0297c40e2262ad1c4d0ce))
* **deps:** bump golang.org/x/term from 0.23.0 to 0.25.0 ([bef8476](https://github.com/grafana/generate-policy-bot-config/commit/bef847652a2c0528db7bf720a17ad5fc84ac6e83))
* **deps:** bump golangci/golangci-lint-action from 6.1.0 to 6.1.1 ([66bda90](https://github.com/grafana/generate-policy-bot-config/commit/66bda907a4d93b72c012bf34fe785d66ae2e90df))
* **docs:** Add standard project documentation ([b1d518a](https://github.com/grafana/generate-policy-bot-config/commit/b1d518a837c2d686458a382c958a4d7c7335fa7e))
* **go.mod:** update module name ([df2842f](https://github.com/grafana/generate-policy-bot-config/commit/df2842fb3784e63aef44ab435bd99375d4b4f06b))
* move `.go` files out of the root ([5576877](https://github.com/grafana/generate-policy-bot-config/commit/55768778b2ff1b22a86e4caa94adfed5ceae6b78))
* release as a prerelease v0.1.0 ([e3700e6](https://github.com/grafana/generate-policy-bot-config/commit/e3700e6b3f57566f18bf1dad458e2525a6bbdf8e))
* **release:** make releases with with github actions user ([6d3f0b4](https://github.com/grafana/generate-policy-bot-config/commit/6d3f0b483978c9c4c26b3dcfa97a4fba4ca1677b))
* remove reference to `deployment_tools` ([6205ebb](https://github.com/grafana/generate-policy-bot-config/commit/6205ebbe48fd6846eeabacdfe462b7643ce70d70))

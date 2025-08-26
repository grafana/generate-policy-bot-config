module github.com/grafana/generate-policy-bot-config

go 1.24.0

toolchain go1.24.1

require (
	github.com/jessevdk/go-flags v1.6.1
	github.com/lmittmann/tint v1.1.2
	github.com/palantir/policy-bot v1.38.2
	github.com/redmatter/go-globre v1.2.0
	github.com/stretchr/testify v1.11.0
	github.com/willabides/actionslog v0.5.1
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d
	golang.org/x/term v0.34.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.10.0 // indirect
	github.com/goccy/go-yaml v1.11.0 // indirect
	github.com/google/go-github/v72 v72.0.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/shurcooL/githubv4 v0.0.0-20240727222349-48295856cce7 // indirect
	github.com/shurcooL/graphql v0.0.0-20181231061246-d48a9a75455f // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

// Includes PRs to test:
// - https://github.com/palantir/policy-bot/pull/796
// - https://github.com/palantir/policy-bot/pull/794
// - https://github.com/palantir/policy-bot/pull/789

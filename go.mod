module github.com/grafana/generate-policy-bot-config

go 1.23.0

require (
	github.com/jessevdk/go-flags v1.6.1
	github.com/lmittmann/tint v1.0.6
	github.com/palantir/policy-bot v1.35.0
	github.com/redmatter/go-globre v0.0.0-20190402065555-2f9fff18bc95
	github.com/stretchr/testify v1.10.0
	github.com/willabides/actionslog v0.5.1
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d
	golang.org/x/term v0.27.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.10.0 // indirect
	github.com/goccy/go-yaml v1.11.0 // indirect
	github.com/google/go-github/v63 v63.0.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/shurcooL/githubv4 v0.0.0-20240727222349-48295856cce7 // indirect
	github.com/shurcooL/graphql v0.0.0-20181231061246-d48a9a75455f // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

// Includes PRs to test:
// - https://github.com/palantir/policy-bot/pull/796
// - https://github.com/palantir/policy-bot/pull/794
// - https://github.com/palantir/policy-bot/pull/789
replace github.com/palantir/policy-bot => github.com/iainlane/policy-bot v1.35.1-0.20240904124510-b6b6121c33c8

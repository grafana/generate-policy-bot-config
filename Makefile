MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
GITHUB_ACTIONS_WORKFLOWS=$(wildcard .github/workflows/*.yml)
GO_FILES=$(wildcard cmd/*.go internal/*.go)

check-policy.yml:
	# We redirect stderr to stdout because the tool logs to stderr using proper
	# Actions log levels (to avoid interfering with `diff`), but those log
	# commands only work on stdout.
	@bash -c 'diff -u .policy.yml <(go run cmd/main.go --merge-with policy.yml -o - $(MAKEFILE_DIR))' 2>&1 && \
		( echo "No drift detected: .policy.yml is up-to-date." >&2; exit 0 ) || \
		( echo "Drift detected: .policy.yml is out-of-date. Run \`make .policy.yml\` to update it, and then commit the result." >&2; exit 1 )

.policy.yml: Makefile policy.yml $(GITHUB_ACTIONS_WORKFLOWS) $(GO_FILES)
	go run \
	github.com/grafana/generate-policy-bot-config/cmd/generate-policy-bot-config \
		--merge-with=policy.yml \
		--log-level=debug \
		$(MAKEFILE_DIR)

all: .policy.yml

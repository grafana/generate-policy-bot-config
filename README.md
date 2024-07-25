# `generate-policy-bot.yml`

This script generates the [Policy Bot][policy-bot] configuration file.

It looks at all the GitHub Actions workflows in `.github/workflows`, finds the
ones which are triggered on the `pull_request` or `pull_request_target` events,
and generates a configuration file which requires them to pass if they are
triggered. If there are path filters, those are copied to the configuration
file.

This is needed because GitHub does not support checkes which are required _if
triggered_. We need to implement it externally.

[policy-bot]: https://github.com/palantir/policy-bot

## Usage

```bash
go run . --output ../../.policy.yml --log-level=debug --merge-with=policy.yml
```

See `--help` for more documentation.

## Merge with existing configuration

The `policy.yml` file in this directory contains configuration which is merged
with the generated config. This allows us to use any of the features of the
Policy Bot beyond the conditional checks that this script generates.

## Don't mind the regexes

GitHub Actions uses `doublestar`-style globs for path filters. Policy Bot takes
regular expressions. The conversion between the two is hairy. We use a library
to do it. Let it wash over you.

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

The merge is quite naive: it simply appends the generated configuration to the
end of the existing configuration. Since we generate configuration of the form:

```yaml
policy:
  approval:
    - or:
        - and:
            - workflows here
```

merging a configuration like:

```yaml
policy:
  approval:
    - or:
        - some rule here
```

would result in

```yaml
policy:
  approval:
    - or:
        - and:
            - workflows here
    - or:
        - some rule here
```

Which might not be what we want. There is an implicit top-level `and` condition
at the root of the policies, so this would mean that "some rule here" _and_ the
workflows would be required. It's not possible to add an alternative to the
workflows - say, to add an override.

We've got an ad-hoc capability to address this. The special token
`MERGE_WITH_GENERATED` as the first element of a top-level `or` group in the the
merged configuration will cause the rest of that `or` group to be merged with
the generated part of the configuration. Merging:

```yaml
policy:
  approval:
    - or:
        - MERGE_WITH_GENERATED
        - some rule here
```

with the above generated configuration would result in

```yaml
policy:
  approval:
    - or:
        - and:
            - workflows here
        - some rule here
```

and so "some rule here", if triggered, will approve the group containing the
workflows.

## Don't mind the regexes

GitHub Actions uses `doublestar`-style globs for path filters. Policy Bot takes
regular expressions. The conversion between the two is hairy. We use a library
to do it. Let it wash over you.

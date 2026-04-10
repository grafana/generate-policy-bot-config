# validate-policy-bot-config

Validates the `.policy.yml` configuration file for [Policy Bot][policy-bot]. See
[the documentation][policy-bot-docs] for more information on creating rules.

[policy-bot]: https://github.com/palantir/policy-bot
[policy-bot-docs]: https://github.com/palantir/policy-bot?tab=readme-ov-file#configuration

## Inputs

- `policy`: The path to the `.policy.yml` file to validate. Default: `.policy.yml`.
- `validation_endpoint` (required): The endpoint to validate the configuration
  against.

Example workflow:

```yaml
name: validate-policy-bot
on:
  pull_request:
    paths:
      - .policy.yml
  push:
    paths:
      - .policy.yml

jobs:
  validate-policy-bot:
    runs-on: ubuntu-latest
    steps:
      - name: Validate Policy Bot configuration
        uses: grafana/generate-policy-bot-config/actions/validate@main
```

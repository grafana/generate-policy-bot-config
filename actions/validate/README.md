# Validate Policy Bot configuration

Validates the `.policy.yml` configuration file for [Policy Bot][policy-bot]. See
[the documentation][policy-bot-docs] for more information on creating rules.

[policy-bot]: https://github.com/palantir/policy-bot
[policy-bot-docs]: https://github.com/palantir/policy-bot?tab=readme-ov-file#configuration

## Inputs

- `policy`: The path to the `.policy.yml` file to validate. Default: `.policy.yml`.
- `validation_endpoint` (required): The endpoint to validate the configuration
  against, such as `https://policy-bot.example.com/api/validate`.

The action uploads the policy with an HTTP `PUT` request. It fails for HTTP
errors and prints the response body and curl's error message. The endpoint
checks the policy's syntax and configuration, but cannot determine whether the
rules match your intended approval policy.

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

permissions:
  contents: read

jobs:
  validate-policy-bot:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false

      - name: Validate Policy Bot configuration
        uses: grafana/generate-policy-bot-config/actions/validate@main
        with:
          validation_endpoint: https://policy-bot.example.com/api/validate
```

Replace the example endpoint with your Policy Bot instance's validation URL.
Pin the action to a full commit SHA to use a fixed version.

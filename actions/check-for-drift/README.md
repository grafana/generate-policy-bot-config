# Check for policy drift

Regenerates a Policy Bot configuration from the workflows in the working
directory and compares it with a committed policy file. The action fails when
the files differ, when generation fails, or when the policy cannot be read.

The action builds the generator from its own source using the Go version in its
`go.mod`. Check out the repository to be checked before running the action. The
generator does not overwrite the committed policy.

The action caches Go modules and build outputs automatically. No additional
setup steps or jobs are required.

## Inputs

- `input_file` (required): The generated policy file to compare with. Paths are
  relative to the working directory.
- `merge_with` (optional): The policy template to merge with the generated
  configuration. Omit this input if you do not use a template.

## Example

```yaml
name: Check policy drift
on:
  pull_request:
  merge_group:

permissions:
  contents: read

jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false

      - name: Check for drift
        uses: grafana/generate-policy-bot-config/actions/check-for-drift@main
        with:
          input_file: .policy.yml
          merge_with: policy.yml
```

Pin the action to a full commit SHA to use a fixed generator version. Regenerate
the committed policy with the same version of the generator.

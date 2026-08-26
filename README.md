# `generate-policy-bot-config`

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

_We're assuming Policy Bot is already configured and working for your
organisation._

Generate a configuration file like this:

### Running from a Docker image

We push a Docker image to the GitHub Container Registry:

```bash
docker run --rm \
  --volume $(pwd):/work \
  --workdir /work \
  ghcr.io/grafana/generate-policy-bot-config:latest \
  --output /work/.policy.yml \
  --log-level=debug \
  --merge-with=/work/policy.yml \
  .
```

Semver-tagged images are also available, as is `main` for the latest commit on
the `main` branch.

These docker images are attested using [GitHub Artifact Attestations][attest].
You can verify our container images by using the `gh` CLI:

```console
$ gh attestation verify --repo grafana/generate-policy-bot-config oci://ghcr.io/grafana/generate-policy-bot-config:<tag>
Loaded digest sha256:... for oci://ghcr.io/grafana/generate-policy-bot-config:<sometag>
Loaded 3 attestations from GitHub API
✓ Verification succeeded!

sha256:<sometag> was attested by:
REPO                                PREDICATE_TYPE                  WORKFLOW
grafana/generate-policy-bot-config  https://slsa.dev/provenance/v1  .github/workflows/build.yml@<somref>
```

Attestations can also be viewed on the [attestation page] of this repository.

What this lets you do is trace a container image back to a build in this
repository. You'll still need to verify the build steps that were used to build
the image to ensure that the image is safe to use.

[attest]: https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds
[attestation page]: https://github.com/grafana/generate-policy-bot-config/attestations

### Running from source

```bash
go run . --output ../../.policy.yml --log-level=debug --merge-with=policy.yml
```

Commit `.policy.yml` to your repository. (Use a different name if your Policy
Bot configuration requires it.) If it's not already configured, configure Policy
Bot as normal: by setting up a [ruleset] which has _only_ Policy Bot's check as
required for pull requests to be mergeable. This check will act as a proxy for
your other workflows.

[ruleset]: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets

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

### Your approval rules are copied through as they are

The `approval_rules` of the merged configuration, and its `policy.disapproval`
section, are copied into the output verbatim. Comments and key order survive, and
so does anything Policy Bot understands but we don't.

This matters because Policy Bot decides whether to apply a default by asking
whether a field was set at all, and its own types can't be written back out
without losing that distinction: the fields are tagged `omitempty`, so a field
which was set to an empty value comes out looking unset. The case to know about
is `methods.comments`:

```yaml
approval_rules:
  - name: needs a review
    requires:
      count: 1
    options:
      methods:
        comments: []
        github_review: true
```

An empty `comments` list is the only way to say that no comment should count as
approval. Were we to hand that back to Policy Bot through its own types, the
field would vanish and Policy Bot would reinstate its `:+1:` and `👍` defaults —
a rule which looks like it demands a review would quietly accept a thumbs-up
instead.

The `policy.approval` section is the exception: it has to be understood in order
to be merged at all, so comments in it are still lost. Its contents are plain
lists and rule names, which don't have this problem.

## Don't mind the regexes

GitHub Actions uses `doublestar`-style globs for path filters. Policy Bot takes
regular expressions. The conversion between the two is hairy. We use a library
to do it. Let it wash over you.

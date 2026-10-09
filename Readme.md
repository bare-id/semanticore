# Semanticore Release Bot 🤖 🦁 🐉

## About

Your friendly Semanticore Release bot helps maintaining the changelog for a project and automates the related tagging process.

## How to use it

Semanticore runs along every pipeline in the main branch, and will analyze the commit messages.

It maintains an open Merge Request for the project with all the required Changelog adjustments.

It detects the current version and suggests the next version based on the changes made.

Once a release commit is detected, it will automatically create the related Git tag on the next pipeline run.

## Conventions

* Commit messages should follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) so semanticore can decide whether a minor or patch level release is required.
* Releases are indicated with a commit with a commit messages which should match: `Release vX.Y.Z`

### Supported Commit Types

Currently Semanticore supports the following commit types:

| Type             | Prefixes                   | Meaning                                  |
|------------------|----------------------------|------------------------------------------|
| 🆕 Feature       | `feat`                     | New Feature, creates a minor commit      |
| 🚨 Security Fix  | `sec`                      | Security relevant change/fix             |
| 👾 Bugfix        | `fix`, `bug`               | Bugfix                                   |
| 🛡 Test          | `test`                     | (Unit-)Tests                             |
| 🔁 Refactor      | `refactor`, `rework`       | Refactorings or reworking                |
| 🤖 Devops/CI     | `ops`, `ci`, `cd`, `build` | Operations, Build, CI/CD, Pipelines      |
| 📚 Documentation | `doc`                      | Documentation                            |
| ⚡️ Performance   | `perf`                     | Performance improvements                 |
| 🧹 Chore         | `chore`, `update`          | Chores, (Dependency-)Updates             |
| 📝 Other         | everything else            | Everything not matched by another prefix |

### Major versions

To enable support for major releases (breaking APIs), use the `-major` flag.

## Configuration

The `SEMANTICORE_TOKEN` is required - that's a Gitlab or Github Token which has basic contributor rights and allows to perform the related Git and API operations.

### Creating a GitHub token (fine-grained PAT)

On GitHub the built-in `GITHUB_TOKEN` works for opening the Release pull request and
creating tags, but events it produces **do not trigger other workflows** (GitHub
prevents this to avoid recursion). If you want a tag created by Semanticore to start
a downstream workflow — for example a `release.yml` that builds and attaches binaries —
you need a Personal Access Token (PAT) instead:

1. Go to **Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token**.
2. Set **Resource owner** to the org or user that owns the repository.
3. Under **Repository access** choose *Only select repositories* and pick your repository.
4. Grant these **Repository permissions**:
   - **Contents**: Read and write — this single permission covers tags, commits and releases
   - **Pull requests**: Read and write — for the Release pull request
5. Generate the token and copy it.
6. In the repository go to **Settings → Secrets and variables → Actions → New repository secret**
   and add it as `SEMANTICORE_TOKEN`.
7. Reference it in your workflow:

   ```yaml
   env:
     SEMANTICORE_TOKEN: ${{ secrets.SEMANTICORE_TOKEN }}
   ```


### Sign Key Configuration

To enable GPG signing of commits, you have two options:

- Use `SEMANTICORE_SIGN_KEY` environment variable containing the actual GPG private key
- Use `SEMANTICORE_SIGN_KEY_FILE` environment variable or the command line option `-sign-key-file`
  specifying the path to a file containing the
  GPG private key

If neither is provided, commits will not be signed.

### Set Author and committer

Semanticore respects [Git Environment variables](https://git-scm.com/book/en/v2/Git-Internals-Environment-Variables)

* `GIT_AUTHOR_NAME`
* `GIT_AUTHOR_EMAIL`
* `GIT_COMMITTER_NAME`
* `GIT_COMMITTER_EMAIL`

The values can also be overridden by adding the appropriate flags. Run with `-help` to get the details.

If none of these is set, Semanticore will use `Semanticore Bot` as name and `semanticore@aoe.com` as E-Mail for Author and Committer.

### Configure filename of changelog

To configure the name of the changelog file, you can use the `CHANGELOG_FILE_NAME`. environment variable. If this variable is not set,
the default value `Changelog.md` will be used.

### Change label synchronization

Semanticore can propagate a `change::...` label to the release MR/PR.

This feature is disabled by default and only becomes active when both settings are configured:

* `SEMANTICORE_CHANGE_LABELS_ENABLED=true`
* `SEMANTICORE_CHANGE_LABELS` with a CSV list of **at least two** full labels in priority order
  (highest priority first)
* `SEMANTICORE_CHANGE_LABEL_DEFAULT` – label returned when nothing matches at all;
  must be contained in `SEMANTICORE_CHANGE_LABELS`

The default fallback is optional. When not set, no label is added for the no-match case.

Example:

* `SEMANTICORE_CHANGE_LABELS=change::emergency,change::major,change::normal,change::standard`

You can optionally map semantic commit types to change labels:

* `SEMANTICORE_CHANGE_LABEL_MAP=feat=change::normal,chore=change::standard,ops=change::standard`

Use the map to define feature fallback behavior, e.g. map `feat` to `change::normal`.

Each mapped label must be part of `SEMANTICORE_CHANGE_LABELS`, otherwise the feature is disabled.

When active, Semanticore always synchronizes only labels starting with `change::` and keeps all other labels untouched.

#### Downgrade protection

By default (`SEMANTICORE_CHANGE_LABEL_POLICY=upgrade-only`), Semanticore never replaces a
change label already present on the release MR/PR with a lower-ranked one - this protects a
label that was set or escalated by hand, and keeps repeated runs idempotent. Set
`SEMANTICORE_CHANGE_LABEL_POLICY=overwrite` to always recompute the label from scratch instead.

#### Emergency change detection

Semanticore can automatically add the highest-ranked label (`change::emergency` by default) to
the release MR/PR when a commit in the release looks like an emergency change. This is disabled
by default; enable it with `SEMANTICORE_CHANGE_EMERGENCY_ENABLED=true` (requires
`SEMANTICORE_CHANGE_LABELS_ENABLED=true`).

A change is considered an emergency when at least one of the following is true for a commit in
the release:

* The commit carries a [git trailer](https://git-scm.com/docs/git-interpret-trailers) matching
  `SEMANTICORE_CHANGE_EMERGENCY_TRAILER` (default `Change-Type: emergency`), case-insensitively on
  both key and value. The trailer must be part of the trailer block in the **last paragraph** of
  the commit message, e.g.:

  ```
  fix(payment): patch broken refund calculation

  Change-Type: emergency
  ```

* The associated merge request's source branch matches one of the glob patterns in
  `SEMANTICORE_CHANGE_EMERGENCY_BRANCHES` (default `hotfix/*`, comma-separated for multiple
  patterns).
* The associated merge request already carries the configured emergency label.

Because a squash merge discards the trailers of the squashed-away commits, Semanticore also
checks the merge request's own commits via the backend API whenever the squash/merge commit
itself does not carry the trailer. If your workflow squash-merges, consider using a squash commit
message template that preserves the trailer, e.g. GitLab's "Squash commit message" field set to:

```
%{title}

%{all_commits}
```

or, simpler, just re-adding the trailer once in the squash commit message itself.

The emergency label must be part of `SEMANTICORE_CHANGE_LABELS`, otherwise Semanticore aborts
with an error at startup.

#### Breaking changes

When a commit in the release uses a breaking-change marker (`feat!:`/`fix!:`/... or a
`BREAKING CHANGE:` footer), Semanticore can add an additional label via
`SEMANTICORE_CHANGE_LABEL_BREAKING` (must be part of `SEMANTICORE_CHANGE_LABELS`). This is
disabled by default (empty value). Regardless of this setting, Semanticore logs a reminder to
check whether the related epic should be labeled `change::major` - Semanticore itself never sets
`change::major`.

## Using Semanticore

To test Semanticore locally you can run it without an API token to create an example Changelog:

```
go run github.com/aoepeople/semanticore@v0 <optional path to repository>
```

### Example Configurations

#### Github Action

`.github/workflows/semanticore.yml`
```yaml
name: Semanticore

on:
  push:
    branches:
      - main
jobs:
  semanticore:
    runs-on: ubuntu-latest
    name: Semanticore
    steps:
      - uses: actions/checkout@v3
        with:
          fetch-depth: 0
      - name: Setup Go
        uses: actions/setup-go@v3
        with:
          go-version: '1.*'
      - name: Semanticore
        run: go run github.com/aoepeople/semanticore@v0
        env:
          SEMANTICORE_TOKEN: ${{secrets.SEMANTICORE_TOKEN}}
          GOTOOLCHAIN: auto
```

#### Gitlab CI

Create a secret `SEMANTICORE_TOKEN` containing an API token with `api` and `write_repository` scope.

`.gitlab-ci.yml`
```yaml
stages:
  - semanticore

semanticore:
  image: golang:1
  stage: semanticore
  variables:
    GOTOOLCHAIN: auto
  script:
    - go run github.com/bare-id/semanticore@v0
  only:
    - main
```

Make sure you set the repositories clone depth too a large enough value, the default of `50` might be too low.

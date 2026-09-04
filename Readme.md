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

### Optional LLM-powered release notes

Semanticore can optionally generate a human-friendly release-note summary for the release pull request while keeping the generated changelog file unchanged.

Enable it with any of the following flags or environment variables:

* `-release-notes-enabled` / `SEMANTICORE_RELEASE_NOTES_ENABLED=true`
* `-release-notes-provider` / `SEMANTICORE_RELEASE_NOTES_PROVIDER=openai|ollama`
* `-release-notes-endpoint` / `SEMANTICORE_RELEASE_NOTES_ENDPOINT=https://api.openai.com/v1` or `http://localhost:11434`
* `-release-notes-model` / `SEMANTICORE_RELEASE_NOTES_MODEL=gpt-4o-mini` or `llama3.1`
* `-release-notes-api-key` / `SEMANTICORE_RELEASE_NOTES_API_KEY=...`
* `-release-notes-prompt` / `SEMANTICORE_RELEASE_NOTES_PROMPT=...`
* `-release-notes-prompt-file` / `SEMANTICORE_RELEASE_NOTES_PROMPT_FILE=.gitlab/release-notes-prompt.txt`

The prompt is read from a local file when available. Semanticore checks for these repository-local defaults in order:

* `.gitlab/release-notes-prompt.txt`
* `.github/release-notes-prompt.txt`
* `release-notes-prompt.txt`

This lets teams keep the prompt versioned in the repository. If you set `-release-notes-prompt` or `SEMANTICORE_RELEASE_NOTES_PROMPT`, it overrides the file. The generated release notes are added to the merge request text and are not written into `Changelog.md`.

### Dry run

Use `-dry-run` or `SEMANTICORE_DRY_RUN=true` to preview the generated changelog and release notes without changing the repository or creating the MR.

```bash
export SEMANTICORE_DRY_RUN=true
export SEMANTICORE_RELEASE_NOTES_ENABLED=true
export SEMANTICORE_RELEASE_NOTES_PROVIDER=ollama
export SEMANTICORE_RELEASE_NOTES_ENDPOINT=http://localhost:11434
export SEMANTICORE_RELEASE_NOTES_MODEL=llama3.1

go run .
```

This prints the changelog and the generated release notes to stdout and exits before any commit, push, or merge request is created.

Example for OpenAI:

```bash
export SEMANTICORE_RELEASE_NOTES_ENABLED=true
export SEMANTICORE_RELEASE_NOTES_PROVIDER=openai
export SEMANTICORE_RELEASE_NOTES_ENDPOINT=https://api.openai.com/v1
export SEMANTICORE_RELEASE_NOTES_MODEL=gpt-4o-mini
export SEMANTICORE_RELEASE_NOTES_API_KEY=your-key
```

Example for Ollama:

```bash
export SEMANTICORE_RELEASE_NOTES_ENABLED=true
export SEMANTICORE_RELEASE_NOTES_PROVIDER=ollama
export SEMANTICORE_RELEASE_NOTES_ENDPOINT=http://localhost:11434
export SEMANTICORE_RELEASE_NOTES_MODEL=llama3.1
```

Example prompt template:

```text
Write a concise release summary for a software project.
Use the changelog below as the source of truth.
Focus on customer impact, highlight key changes, and mention migration notes when relevant.

{{CHANGELOG}}

{{ISSUES}}
```

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

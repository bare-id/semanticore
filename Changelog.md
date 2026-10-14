# Changelog

## Version v0.11.0 (2026-10-14)

### Features

- add describe subcommand to keep versions consistent (3e502a8b)
- allow target-branch config to support hotfix releases properly (0b06d332)

## Version v0.10.0 (2026-10-09)

### Features

- deduct change-labels also for emergency changes (96519ffa)

### Chores and tidying

- **deps:** update golang deps to most recent versions (4bdc377b)

## Version v0.9.0 (2026-09-11)

### Features

- add support to deduct change labels from related commits (be1c7900)

### Fixes

- **ci:** avoid breaking coverage reports on PRs from forks (5eae83e2)

## Version v0.8.1 (2026-09-04)

### Fixes

- avoid failing on empty releases (c8d1c5b4)
- **ci:** use SEMANTICORE_TOKEN rather than GITHUB_TOKEN (e7e71ec0)

### Chores and tidying

- apply default linter config and fix related errors (cfb00c4c)
- **deps:** upgrade all depdencies and workflows to an up-to-date version (1214dcf1)

## Version v0.8.0 (2025-09-24)

### Features

- allow configuration of changelog file, enhance readme (#88) (0b06edd6)

### Fixes

- **deps:** update module github.com/protonmail/go-crypto to v1.3.0 (#82) (6c49bf8e)
- **deps:** update module github.com/stretchr/testify to v1.11.1 (#86) (568c7d83)

### Chores and tidying

- **deps:** update actions/setup-go action to v6 (#87) (4dca4839)
- **deps:** update actions/checkout action to v5 (#85) (7ece5c45)
- **config:** migrate config renovate.json (#90) (bdf0c237)

## Version v0.7.1 (2025-06-11)

### Fixes

- message on missing sign key (#83) (14c00684)

## Version v0.7.0 (2025-06-10)

### Features

- Support signing of commits (#80) (bbcf93ec)

### Fixes

- **deps:** update module github.com/go-git/go-git/v5 to v5.16.2 (#77) (bfcea1b3)

## Version v0.6.0 (2025-02-13)

### Features

- **changelog:** changelog-max-lines (e997da31)

### Fixes

- **deps:** update module github.com/go-git/go-git/v5 to v5.12.0 (#67) (1997f732)
- **deps:** update module github.com/stretchr/testify to v1.9.0 (#70) (4d23fe38)

### Ops and CI/CD

- **github:** go min version 1.24 (197af1e1)
- **github:** update ci and trim changelog (cf2d689e)

### Chores and tidying

- **go:** update dependencies (4376e075)
- **deps:** update actions/setup-go action to v5 (#69) (5cd58d42)

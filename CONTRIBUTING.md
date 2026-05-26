# Contributing to agent-os

Thanks for your interest in contributing!

## Getting Started

1. Fork the repo and clone it locally.
2. Run `make build` to compile and verify everything works.
3. Create a branch for your change.

## Development Workflow

- Code lives in `pkg/` (library packages) and `cmd/` (binaries).
- Run `make test` before opening a PR.
- Run `make lint` to check code style.
- Add tests in `*_test.go` files alongside the code being tested.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep exported symbols documented with Go doc comments.
- Keep functions focused and under ~80 lines where practical.
- Use the `.golangci.yml` config provided in the repo root.

## Pull Requests

- Describe what the change does and why.
- Keep PRs small and focused on a single concern.
- If your PR adds a new feature, update the relevant docs in `docs/`.

## Issue Reporting

- Use the issue tracker for bugs, feature requests, and questions.
- Include steps to reproduce for bugs.
- Mention your Go version and OS.

Thanks for helping make agent-os better!

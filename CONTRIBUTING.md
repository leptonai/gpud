# Contributing to GPUd

First and foremost, thank you for considering contributing to GPUd! We appreciate the time and effort you're putting into helping improve our project. This guide outlines the process and standards we expect from contributors.

## Development

First clone the source code from Github

```bash
git clone https://github.com/leptonai/gpud.git
```

Use `go` to build `gpud` from source

```bash
cd gpud
make all

./bin/gpud -h
```

## Testing

We highly recommend writing tests for new features or bug fixes and ensure all tests passing before submitting a PR.

To run all existing tests locally, simply run

```bash
./scripts/tests-unit.sh
./scripts/tests-e2e.sh
```

## Coding Standards

Ensure your code is clean, readable, and well-commented. We use [golangci-lintblack](https://golangci-lint.run/) as code linter.

To run lint locally, first install linters by doing

```bash
golangci-lint run
```

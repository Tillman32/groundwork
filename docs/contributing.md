# Contributing

## Development Setup

```bash
git clone https://github.com/Tillman32/groundwork
cd groundwork

# Install tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run
go run ./cmd/groundworkd
```

## Running Tests

```bash
go test -v ./...
```

## Linting

```bash
golangci-lint run
```

## Commit Convention

Use conventional commits:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation
- `ci:` CI/CD changes
- `chore:` Maintenance

## Pull Requests

1. Fork the repo
2. Create feature branch: `git checkout -b feat/my-feature`
3. Commit changes
4. Push: `git push origin feat/my-feature`
5. Open PR at https://github.com/Tillman32/groundwork/pulls
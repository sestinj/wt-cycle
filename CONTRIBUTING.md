# Contributing to wt-cycle

Thank you for your interest in contributing to wt-cycle!

## Development Setup

**Prerequisites:** Go 1.24+

```bash
git clone https://github.com/sestinj/wt-cycle.git
cd wt-cycle
make build
```

## Building

```bash
make build       # Build binary
make test        # Run tests
make install     # Install to $GOPATH/bin
```

## Submitting Changes

1. Fork the repository
2. Create a feature branch (`git checkout -b my-feature`)
3. Make your changes
4. Run `make test` to ensure tests pass
5. Commit your changes with a clear message
6. Push to your fork and open a pull request

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions small and focused
- Add tests for new functionality

## Reporting Issues

Use [GitHub Issues](https://github.com/sestinj/wt-cycle/issues) to report bugs or request features.

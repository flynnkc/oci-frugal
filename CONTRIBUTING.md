# Contributing

Read [AGENTS.md](AGENTS.md) for repository conventions, architecture and safety
invariants. Use the Go toolchain in go.mod and run `make check`; these tests use
mock clients and need no OCI credentials. `make build` writes /tmp/oci-frugal.

For a behavioral change, describe the problem and acceptance criteria, add a
focused regression test, implement the smallest change, and update user-facing
docs. Keep dependency updates separate where practical. Review go.mod and go.sum
changes and run `make security` using an organization-reviewed govulncheck version.

Pull requests should state the behavior change, actual validation, compatibility
impact and any new cloud permissions. A human maintainer must review changes to
scheduling, authentication, resource discovery, lifecycle handling and writes.
Do not attach raw tenant reports, credentials or private resource inventories.
Live sandbox validation is a separately authorized operator activity; follow
[the runbook](docs/OPERATIONS.md). No live cloud calls belong in default tests.

Before internal adoption, appoint maintainers and configure required CI checks
and review protection in the repository host. Documentation cannot enforce those
settings. Track pilot gates in [the readiness assessment](docs/READINESS.md).

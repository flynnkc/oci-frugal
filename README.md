# OCI Frugal

OCI Frugal discovers tagged OCI resources, evaluates an hourly schedule, and submits
power requests concurrently. It is being developed as a replacement for
[OCI-AutoScale](https://github.com/AnykeyNL/OCI-AutoScale), with planning and execution
separated to support future scheduling APIs.

## Current scope: milestone 1

Supported power operations:

| Resource | On | Off |
| --- | --- | --- |
| Compute instance | START | SOFTSTOP by default; STOP with `-shutdown hard` |
| VM DB system | START for each DB node | STOP for each DB node |
| Analytics instance | Start | Stop |

DB node discovery uses the Database API, follows pagination, and includes all nodes.
Bare metal and Exadata DB systems are rejected for power scheduling. Autonomous Database
and Integration instances are discovered but return explicit unsupported-service errors
when their schedules request an action. Other resource types are outside the current
discovery scope.

Capacity scaling and the external HTTP API belong to milestones 3 and 2 respectively and
are not implemented yet. Numeric capacity values and flex sizing are rejected rather
than treated as power flags.

## Build and preview

Use the Go version specified in `go.mod` (Go 1.24 or newer).

```sh
go build -o /tmp/oci-frugal ./src
/tmp/oci-frugal -dry-run -region us-ashburn-1 -tz America/New_York
```

Dry runs authenticate to OCI and make read requests to discover and validate operations,
but never submit power requests. Review the JSON output before enabling execution:

```sh
/tmp/oci-frugal -region us-ashburn-1 -tz America/New_York -action all
```

API key authentication defaults to `~/.oci/config`, profile `DEFAULT`. The CLI also
supports `instance_principal`, `resource_principal`, and `workload_principal`. OCI
credentials and IAM permissions must allow resource discovery and reads of managed
resources; execution also requires the corresponding service power permissions.
Scheduling never creates namespaces or tag definitions automatically.

## Schedule tags

Create a defined tag namespace named `Schedule` (or set `-tag`). Within it, create the
tag keys you use. Hourly values have exactly 24 comma-separated tokens, indexed from
midnight in the selected timezone:

- `0`: power off.
- `1`: power on.
- `*`: leave the resource unchanged.

Example: `Schedule.AnyDay` starts at 08:00 and stops at 18:00:

```text
0,0,0,0,0,0,0,0,1,1,1,1,1,1,1,1,1,1,0,0,0,0,0,0
```

A trailing `# comment` is supported. Tokens such as negative numbers, empty hourly
slots, capacity numbers above `1`, and `(cpu:memory)` return errors. The entire selected
schedule is validated before any operation is planned, including hours other than the
current hour.

The case-sensitive precedence, from lowest to highest, is:

1. `AnyDay`
2. `WeekDay` or `Weekend`
3. Day name, such as `Monday`
4. Nth weekday, such as `Monday2`
5. `DayOfMonth`, such as `1:0,15:1,28:*`

`DayOfMonth` sets one value for all hours on each listed day. Duplicate days, days
outside 1–31, and malformed pairs are errors. Resources without the namespace, without
an applicable schedule, or with `*` for the current hour are skipped.

One instant is captured at the start of a run and evaluated in the configured timezone
for every region, including daylight saving time. The default is the host's local
timezone; regions do not select their own timezones. Run separate scoped invocations if
regions need different schedules.

## Flags and environment

Flags override environment variables.

| Flag | Environment variable | Default |
| --- | --- | --- |
| `-dry-run` | `FRUGAL_DRY_RUN` | `false` |
| `-action` | `FRUGAL_ACTION_TYPE` | `all` (`all`, `on`, `off`) |
| `-shutdown` | `FRUGAL_SHUTDOWN` | `soft` (`soft`, `hard`) |
| `-timeout` | `FRUGAL_TIMEOUT` | `10m` for the entire run |
| `-tz` | `FRUGAL_TIMEZONE` | Host local timezone |
| `-region` | `FRUGAL_REGION` | All ready subscribed regions |
| `-tag` | `FRUGAL_TAG_NAMESPACE` | `Schedule` |
| `-auth` | `FRUGAL_AUTH_TYPE` | `api_key` |
| `-config-file` | `FRUGAL_FILE` | `~/.oci/config` |
| `-profile` | `FRUGAL_PROFILE` | `DEFAULT` |
| `-key-pass` | `FRUGAL_KEY_PASS` | Empty; prefer the environment over a process argument |
| `-v` | `FRUGAL_LOG_LEVEL` | `info` |

Invalid action, shutdown, timezone, timeout, and log-level settings return errors.
`-action on` and `-action off` filter power actions; they do not mean scale up or scale
down.

## Results and failures

Standard output contains one JSON document. Diagnostics and help go to standard error.
Exit codes are:

| Code | Meaning |
| --- | --- |
| `0` | Planning or request submission succeeded, or help was shown |
| `1` | Discovery, planning, cancellation, execution, or output failed |
| `2` | Invalid arguments or configuration |

All regions are planned before execution begins. A discovery or planning error in any
region blocks all writes, with report phase `blocked`. A successful dry run has phase
`dry_run`. Execution has phase `execution`; the top-level `error` and exit code identify
failures.

Each resource records its action, status, operations, and any error. DB node operations
include their parent DB system ID. Statuses are `skipped`, `planned`, `noop`,
`submitted`, and `failed`. `planned` operations in a blocked report were **not
executed**. A failed resource can include both submitted and failed node operations, so
inspect its individual operations before retrying.

`submitted` means OCI accepted the request. It does not mean an asynchronous start or
stop has finished. OCI request IDs are included when available. State is read again
before each write, and already satisfied requests become `noop`. Transitional, failed,
deleted, and unknown states return errors instead of sending conflicting requests.
Execution continues collecting other resource/node outcomes after individual failures;
previously accepted writes cannot be rolled back.

There are eight workers and one shared request limiter per region (eight SDK calls every
15 seconds, with an initial burst of eight). Reads, node discovery, state checks, and
writes consume tokens. SDK-internal retries retain their SDK policy; mutations do not
receive a blanket application retry. Each SDK call has a 30-second deadline inside the
run timeout. Interrupt and termination signals cancel the run. Limiter goroutines close
on completion and construction failure.

Run the CLI from an external scheduler at the desired interval. Avoid overlapping
invocations: cross-process deduplication, durable jobs, completion tracking, and
schedule management are not part of this milestone. The CLI does not yet include
compartment filters; scope access with the OCI principal and region selection.

## Validation

```sh
go test -race -cover ./...
go vet ./...
```

GitHub Actions runs formatting, race-enabled tests, vet, and a binary build on pushes
and pull requests.

Tests use mocked OCI clients and cover read-only planning, action filtering, timezone
and schedule precedence, lifecycle checks, graceful/hard Compute shutdown, DB pagination
and multiple nodes, discovery/action failures, stale state, cancellation, limiter
shutdown, and process exit codes. They do not contact OCI. Live OCI integration and
comparative performance benchmarks remain outstanding.

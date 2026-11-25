.PHONY: check fmt-check test vet build security

check: fmt-check test vet build

fmt-check:
	@test -z "$$(gofmt -l src)" || { gofmt -l src; exit 1; }

test:
	go test -race -cover ./...

vet:
	go vet ./...

build:
	go build -o /tmp/oci-frugal ./src

# Install an organization-reviewed version of golang.org/x/vuln/cmd/govulncheck first.
# This target can access the vulnerability database; it never calls OCI.
security:
	govulncheck ./...

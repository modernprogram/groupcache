#!/bin/bash

go install golang.org/x/vuln/cmd/govulncheck@latest
go install golang.org/x/tools/cmd/deadcode@latest
go install github.com/mgechev/revive@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest
go install github.com/gordonklaus/ineffassign@latest
go install github.com/client9/misspell/cmd/misspell@latest
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

gofmt -s -w .

echo "go mod tidy"
go mod tidy

echo staticcheck
staticcheck ./...

#echo revive
#revive ./...

echo modernize
modernize -fix ./...

echo gocyclo
gocyclo -over 15 .

ineffassign ./...

misspell .

echo govulncheck

govulncheck ./...

#echo deadcode
#deadcode ./cmd/*

go env -w CGO_ENABLED=1

echo test
go test -race ./...

#go test -bench=BenchmarkController ./cmd/secrets

go env -w CGO_ENABLED=0

echo install
go install ./...

go env -u CGO_ENABLED

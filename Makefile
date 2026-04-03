.PHONY: test test-v lint fmt verify

test:
	go test -race -count=1 ./...

test-v:
	go test -race -count=1 -v ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .
	goimports -w .

verify: fmt lint test

.PHONY: fmt fmt-check vet lint test build e2e

fmt:
	go fmt ./...

fmt-check:
	@files=$$(gofmt -l $$(git ls-files --cached --others --exclude-standard '*.go')); \
	if [ -n "$$files" ]; then \
		echo "gofmt required:"; \
		echo "$$files"; \
		exit 1; \
	fi

vet:
	go vet ./...

lint: fmt-check vet

test:
	go test ./...

build:
	go build ./cmd/apify-plugin

e2e:
	go test -tags=e2e ./e2e

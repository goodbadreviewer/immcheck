test: clean
	go test -race -covermode atomic -coverprofile coverage.out ./...

coverage: test
	go tool cover -html=coverage.out

bench:
	go test -timeout 3h -count=5 -run=Benchmark -bench=. github.com/goodbadreviewer/immcheck

profile: clean
	go test -run=xxx -bench=BenchmarkImmcheckTransactions github.com/goodbadreviewer/immcheck/... -cpuprofile profile.out
	go tool pprof -http=:8080 profile.out

lint: install-golangci-lint
	golangci-lint run

debug_inline:
	go build -gcflags='-m -d=ssa/check_bce/debug=1' ./immcheck.go

clean:
	@go clean
	@rm -f profile.out
	@rm -f coverage.out

install-golangci-lint:
	@which golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin 1.42.0

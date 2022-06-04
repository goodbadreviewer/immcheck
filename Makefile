golangci := go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.46.2
benchstat := go run golang.org/x/perf/cmd/benchstat@v0.0.0-20220411212318-84e58bfe0a7e


test: clean
	go test ./...
	go test -tags immcheck ./...
	go test -race -covermode atomic -coverprofile coverage.out ./...

coverage: test
	go tool cover -html=coverage.out


bench:
	go test -timeout 3h -count=5 -run=xxx -bench=BenchmarkImmcheck ./... | tee immchek_stat.txt
	$(benchstat) immchek_stat.txt

profile: clean
	go test -run=xxx -bench=BenchmarkImmcheckTransactions ./... -cpuprofile profile.out
	go tool pprof -http=:8080 profile.out

lint:
	$(golangci) run

debug_inline:
	go build -gcflags='-m -d=ssa/check_bce/debug=1' ./immcheck.go

clean:
	@go clean
	@rm -f profile.out
	@rm -f coverage.out

help:
	@awk '$$1 ~ /^.*:/ {print substr($$1, 0, length($$1)-1)}' Makefile

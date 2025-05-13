unit-tests:
	go test ./...

bench:
	go test -run=^$$ -bench=. ./...

integration-tests-short:
	echo 'no-op for now'

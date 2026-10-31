.PHONY: build test vet check

build:
	go build -o vanda ./cmd/vanda

vet:
	go vet ./...

# -count=1 で、結果を使い回さない（ランダムチェックのシード値を、毎回変えるため）
test:
	go test -count=1 ./...

check: build vet test

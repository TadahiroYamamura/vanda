.PHONY: build test vet tla check

build:
	go build -o vanda ./cmd/vanda

vet:
	go vet ./...

# -count=1 で、結果を使い回さない（ランダムチェックのシード値を、毎回変えるため）
# VANDA_REQUIRE_TLC=1 で、tlc がないときに TLA+ を使うテストを飛ばさず落とす
# クロスチェックは -v で流し、通ったときも、使った種と場面の回数を CI の記録に残す（落ちた場面を種から再現するため）
XCHECK = ^Test合意の判定をTLAのモデルと本物のvandaで1歩ずつ比べると一致する$$
test:
	VANDA_REQUIRE_TLC=1 go test -count=1 -skip '$(XCHECK)' ./...
	VANDA_REQUIRE_TLC=1 go test -count=1 -v -run '$(XCHECK)' ./cmd/vanda

tla:
	spec/check.sh

check: build vet test tla

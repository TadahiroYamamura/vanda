.PHONY: web build vet test web-test e2e expect tla check

# 画面（web/）を作る。作った結果（web/dist/app/）を、build で Go に埋め込む
web:
	cd web && pnpm install --frozen-lockfile && pnpm build

build: web
	go build -o vanda ./cmd/vanda

vet:
	go vet ./...

# -count=1 で、結果を使い回さない（ランダムチェックのシード値を、毎回変えるため）
# VANDA_REQUIRE_TLC=1 で、tlc がないときに TLA+ を使うテストを飛ばさず落とす
# クロスチェックは -v で流し、通ったときも、使った種と場面の回数を CI の記録に残す（落ちた場面を種から再現するため）。
# 300 本のトレースを本物の vanda で 1 歩ずつ流すので、Linux の CI でも 4 分半かかり、macOS の CI では go test の既定の 10 分を過ぎる。そのため時間の上限を 30 分にする
XCHECK = ^Test合意の判定をTLAのモデルと本物のvandaで1歩ずつ比べると一致する$$
test: web-test
	VANDA_REQUIRE_TLC=1 go test -count=1 -skip '$(XCHECK)' ./...
	VANDA_REQUIRE_TLC=1 go test -count=1 -timeout 30m -v -run '$(XCHECK)' ./cmd/vanda

web-test: web
	cd web && pnpm test

# E2E は、作った vanda を動かすので build の後に流す。
# macOS では、nix で入れたブラウザが動かないことがあるため、E2E を Linux だけで流す（段 2 の計画で PO が決めた）
e2e: build
ifeq ($(shell uname),Darwin)
	@echo "macOS なので E2E のテストを飛ばす"
else
	cd web && pnpm e2e
endif

# Expect のシナリオ（vanda init のように、端末で問いを待って答える流れ）を、1 つずつ新しい一時のディレクトリで動かす。
# 作った vanda を PATH の先頭に置く。シナリオの中で git のリポジトリを作るので、vanda のリポジトリの中では動かさない
EXPECT_SCENARIOS = $(wildcard cmd/vanda/testdata/expect/*.exp)
expect: build
	@set -e; for f in $(EXPECT_SCENARIOS); do \
		d=$$(mktemp -d); echo "Expect のシナリオ $$f"; \
		if (cd "$$d" && PATH="$(CURDIR):$$PATH" expect -f "$(CURDIR)/$$f"); then rm -rf "$$d"; else rm -rf "$$d"; exit 1; fi; \
	done

tla:
	spec/check.sh

check: build vet test e2e expect tla

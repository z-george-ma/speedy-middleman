SRC := $(wildcard *.go)
COVERAGE_THRESHOLD=30

all: run

.test.sentinel: $(SRC)
	set -o pipefail; go test -race -cover 2>&1 | tee .test.sentinel || (rm .test.sentinel; false)
	@COVERAGE=`cat .test.sentinel | awk '/\tcoverage: .*% of statements/{print(int($$3))}'`;\
	test $$COVERAGE -ge $(COVERAGE_THRESHOLD) || (\
		rm .test.sentinel;\
		>&2 echo "Test coverage is below $(COVERAGE_THRESHOLD)%.";\
		false\
	)
	
.PHONY: test
test: .test.sentinel

.perf.sentinel: $(SRC)
	@go build -gcflags "-m -l" 2>&1 | grep "^./.*escapes to heap$$" > .perf.sentinel
	@cat .perf.sentinel | sh ../scripts/check-alloc.sh || (\
		rm .perf.sentinel;\
		false\
	)

.PHONY: perf
perf: .perf.sentinel

app: $(SRC)
	go build -o app .

.PHONY: run
run: app
	./app

.PHONY: release
release: $(SRC)
	go build -ldflags="-s -w" --tags musl -a -o app

.PHONY: utils
utils: $(SRC)
	go build utils/*

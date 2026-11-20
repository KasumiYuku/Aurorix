.PHONY: check test vet fmt build run web clean install

check: fmt vet test build

fmt:
	@echo "[fmt]"
	@if [ -n "$$(gofmt -l . )" ]; then \
		echo "以下文件需要格式化:"; \
		gofmt -l .; \
		exit 1; \
	fi

vet:
	@echo "[vet]"
	go vet ./...

test:
	@echo "[test]"
	@test -n "$$AURORIX_TEST_DATABASE" || { echo "需要 AURORIX_TEST_DATABASE 指向一个可连的 PostgreSQL"; exit 1; }
	go test ./... -count=1 -timeout 300s

build:
	@echo "[build]"
	go build -o /dev/null .

run:
	@echo "[run]"
	go run .

install:
	@echo "[install]"
	go install ./tools/aurx

web:
	@echo "[web]"
	cd webui && pnpm build

clean:
	rm -f aurorix

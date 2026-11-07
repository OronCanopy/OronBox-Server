.PHONY: help dev build test vet up down restart logs status shell

help:
	@echo "OronBox Server 常用指令:"
	@echo "  make dev     - 本地直接运行"
	@echo "  make build   - 本地构建二进制"
	@echo "  make test    - 运行测试套件"
	@echo "  make vet     - 代码静态检查"
	@echo "  make up      - docker compose 构建并启动"
	@echo "  make down    - docker compose 停止"
	@echo "  make restart - docker compose 重启"
	@echo "  make logs    - 跟踪容器日志"
	@echo "  make status  - 查看容器状态"
	@echo "  make shell   - 进入容器终端"

dev:
	go run ./cmd/server

build:
	go build -o bin/oronbox-server ./cmd/server

test:
	go test -count=1 ./...

vet:
	go vet ./...

up:
	docker compose up -d --build

down:
	docker compose down

restart:
	docker compose restart

logs:
	docker compose logs -f --tail 100

status:
	docker compose ps

shell:
	docker compose exec server sh

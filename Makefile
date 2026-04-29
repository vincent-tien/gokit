.PHONY: test test-v lint fmt verify test-rabbitmq test-rabbitmq-up test-rabbitmq-down

test:
	go test -race -count=1 ./...

test-v:
	go test -race -count=1 -v ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .
	goimports -w .

verify: fmt lint test

# RabbitMQ integration tests — start broker, run suite, tear down.
# Requires Docker. Override broker endpoint with RABBITMQ_TEST_ADDR.
test-rabbitmq:
	docker compose -f eda/docker-compose.test.yml up -d --wait
	cd eda && go test -race -count=1 -tags=integration -v ./broker/ -run TestRabbitMQ_; \
	    rc=$$?; \
	    cd .. && docker compose -f eda/docker-compose.test.yml down -v; \
	    exit $$rc

test-rabbitmq-up:
	docker compose -f eda/docker-compose.test.yml up -d --wait

test-rabbitmq-down:
	docker compose -f eda/docker-compose.test.yml down -v

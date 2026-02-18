BINARY_NAME=dex-tts-service

build:
	go build -o ${BINARY_NAME} main.go
	mkdir -p ~/.local/bin
	cp ${BINARY_NAME} ~/.local/bin/

clean:
	rm -f ${BINARY_NAME}

format:
	go fmt ./...

lint:
	# No-op for now
	@echo "Linting skipped"

test:
	# No-op for now
	@echo "Testing skipped"

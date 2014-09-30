
BIN=${GOPATH}/bin/client

REV=`git rev-list HEAD --count`

all:
	@echo "Compiling changes-client"
	go install github.com/dropbox/changes-client/client

	@echo "Setting up temp build folder"
	rm -rf /tmp/changes-client-build
	mkdir -p /tmp/changes-client-build/usr/bin
	cp $(BIN) /tmp/changes-client-build/usr/bin/changes-client

	@echo "Creating .deb file"
	fpm -s dir -t deb -n "changes-client" -v "`$(BIN) --version`-$(REV)" -C /tmp/changes-client-build .


test:
	go test ./... -timeout=120s -race


dev:
	@echo "==> Getting dependencies..."
	@make deps

	@echo "==> Building..."
	go build -v ./...


deps:
	go get -v -t ./...


fmt:
	go fmt ./...

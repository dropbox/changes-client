
BIN=${GOPATH}/bin/changes-client

REV=`git rev-list HEAD --count`

all:
	@echo "Compiling changes-client"
	@make install

	@echo "Setting up temp build folder"
	rm -rf /tmp/changes-client-build
	mkdir -p /tmp/changes-client-build/usr/bin
	cp $(BIN) /tmp/changes-client-build/usr/bin/changes-client

	@echo "Creating .deb file"
	fpm -s dir -t deb -n "changes-client" -v "`$(BIN) --version`-$(REV)" -C /tmp/changes-client-build .


test:
	# force an lxc-execute so we have a working base image before running tests
	sudo lxc-execute -n test-changes-client-bootstrap -- /bin/sh -c exit
	go test ./... -timeout=120s -race


dev:
	@echo "==> Getting dependencies..."
	@make deps

	@echo "==> Building..."
	go build -v ./...


install:
	go install -v ./...


deps:
	go get -v -t ./...


fmt:
	go fmt ./...

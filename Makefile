
BIN=${GOPATH}/bin

REV=`git rev-list HEAD --count`

all:
	@echo "Compiling changes-client"
	@make install

	@echo "Setting up temp build folder"
	rm -rf /tmp/changes-client-build
	mkdir -p /tmp/changes-client-build/usr/bin
	cp $(BIN)/client /tmp/changes-client-build/usr/bin/changes-client
	cp $(BIN)/snapshotter /tmp/changes-client-build/usr/bin/changes-snapshotter

	@echo "Creating .deb file"
	fpm -s dir -t deb -n "changes-client" -v "`$(BIN)/changes-client --version`-$(REV)" -C /tmp/changes-client-build .


test:
	@echo "==> Running tests"
	sudo GOPATH=${GOPATH} `which go` test ./... -timeout=60s -race


dev:
	@make deps

	@echo "==> Building..."
	go build -v ./...


install:
	go install -v ./...


deps:
	@echo "==> Getting dependencies..."
	go get -v -u gopkg.in/lxc/go-lxc.v2
	go get -v -t ./...
	@echo "==> Caching base LXC image for tests"
	sudo lxc-create -n bootstrap -t ubuntu || true


fmt:
	go fmt ./...

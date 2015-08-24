
BIN=${GOPATH}/bin/changes-client

# changes-client is dynamically linked with lxc-dev installed on the machine producing the binary.
# To avoid version incompatibilities, we force the same version of lxc-dev to be installed on the
# instance running changes-client too.
LXC_DEV_VERSION=`dpkg-query -W -f='$${Version}\n' lxc-dev`

# Revision shows date of latest commit and abbreviated commit SHA
# E.g., 1438708515-753e183
REV=`git show -s --format=%ct-%h HEAD`

all:
	@echo "Compiling changes-client"
	@make install

	@echo "Setting up temp build folder"
	rm -rf /tmp/changes-client-build
	mkdir -p /tmp/changes-client-build/usr/bin
	cp $(BIN) /tmp/changes-client-build/usr/bin/changes-client

	@echo "Creating .deb file"
	fpm -s dir -t deb -n "changes-client" -v "`$(BIN) --version`" -C /tmp/changes-client-build \
	    --depends "lxc-dev (=$(LXC_DEV_VERSION))" -m dev-tools@dropbox.com \
	    --description "A build client for Changes" --url https://www.github.com/dropbox/changes-client .


test:
	@echo "==> Running tests"
	sudo GOPATH=${GOPATH} `which go` test -v ./... -timeout=120s -race


dev:
	@make deps

	@echo "==> Building..."
	go build -v ./...


install:
	go clean -i ./...
	go install -ldflags "-X github.com/dropbox/changes-client/common/version.gitVersion $(REV)" -v ./...

	@echo "changes-client linked against lxc-dev version:" $(LXC_DEV_VERSION)


deps:
	@echo "==> Getting dependencies..."
	go get -v -u gopkg.in/lxc/go-lxc.v2
	go get -v -t ./...
	@echo "==> Caching base LXC image for tests"
	sudo lxc-create -n bootstrap -t ubuntu || true


fmt:
	go fmt ./...

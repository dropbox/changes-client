
CHANGES_CLIENT_BIN=${GOPATH}/bin/changes-client
BLACKLIST_REMOVE_BIN=${GOPATH}/bin/blacklist-remove

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
	@echo "changes-client linked against lxc-dev version:" $(LXC_DEV_VERSION)

	@echo "Setting up temp build folder"
	rm -rf /tmp/changes-client-build
	mkdir -p /tmp/changes-client-build/usr/bin
	cp $(CHANGES_CLIENT_BIN) /tmp/changes-client-build/usr/bin/
	cp $(BLACKLIST_REMOVE_BIN) /tmp/changes-client-build/usr/bin/

	@echo "Creating .deb file"
	fpm -s dir -t deb -n "changes-client" -v "`$(CHANGES_CLIENT_BIN) --version`" -C /tmp/changes-client-build \
	    --depends "lxc-dev (=$(LXC_DEV_VERSION))" -m dev-tools@dropbox.com --provides changes-client \
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
	go install -tags '$(TAGS)' -ldflags "-X github.com/dropbox/changes-client/common/version.gitVersion=$(REV)" -v ./...



deps:
	@echo "==> Getting dependencies..."
	go get -v -u gopkg.in/lxc/go-lxc.v2
	go get -v -t ./...
	@echo "==> Caching base LXC image for tests"
	sudo lxc-create -n bootstrap -t ubuntu || true


fmt:
	go fmt ./...

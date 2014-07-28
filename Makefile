
BIN=./../../../../bin/client

all:
	@echo "Compiling changes-client"
	go install github.com/dropbox/changes-client/client

	@echo "Setting up temp build folder"
	rm -rf /tmp/changes-client-build
	mkdir -p /tmp/changes-client-build/usr/bin
	cp $(BIN) /tmp/changes-client-build/usr/bin/changes-client

	@echo "Creating .deb file"
	fpm -s dir -t deb -n "changes-client" -v `$(BIN) --version` -C /tmp/changes-client-build .


test:
	go test

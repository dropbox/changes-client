package main

import (
	"flag"
	"log"

	"github.com/dropbox/changes-client/common/blacklist"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		log.Fatalf("Must provide a yaml file to parse")
	}

	yamlFile := args[0]
	if err := blacklist.RemoveBlacklistedFiles(".", yamlFile); err != nil {
		// will exit non-zero and thus lead to an infra fail
		log.Fatalf("[blacklist] Error removing blacklisted files: %s", err)
	}
}

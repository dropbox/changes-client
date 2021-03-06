package client

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	cw := NewCmdWrapper([]string{"/bin/bash", "-c", "echo -n 1"}, "", []string{})
	log := NewLog()

	sem := make(chan bool)
	go func() {
		log.Drain()
		sem <- true
	}()

	result, err := cw.Run(true, log)
	log.Close()
	<-sem
	if err != nil {
		t.Fatal(err.Error())
	}

	if !bytes.Equal(result.Output, []byte("1")) {
		t.Error("Did not buffer output")
	}
}

// if stdin is allowed this test will hang
func TestRunIgnoresStdin(t *testing.T) {
	cw := NewCmdWrapper([]string{"/bin/bash", "-c", "read foo"}, "", []string{})
	log := NewLog()

	sem := make(chan bool)
	go func() {
		log.Drain()
		sem <- true
	}()

	cw.Run(false, log)
	log.Close()
	<-sem
}

func TestRunFailToStart(t *testing.T) {
	cw := NewCmdWrapper([]string{"/bin/bash", "-c", "echo -n 1"}, "", []string{})
	log := NewLog()

	sem := make(chan bool)
	go func() {
		log.Drain()
		sem <- true
	}()

	_, err := cw.Run(false, log)
	log.Close()
	<-sem
	if err != nil {
		t.Fatal(err.Error())
	}
}

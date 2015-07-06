// Executor system, which assigns each container to a specific
// "executor" which is essentially a single thread that runs jobs
// on the host. We kill any container which was associated with
// the same executor before starting our container, which prevents
// hanging containers from changes-client getting SIGKILLed.
package lxcadapter

import (
	"io/ioutil"
	"gopkg.in/lxc/go-lxc.v2"
	"os/exec"
	"log"
	"path"
	"os"
)

type Executor struct {
	Name		string
	Directory	string
}

// This file is a unique file owned by us and no other changes-client
// process but it may be leftover from another process that has already
// terminated.
func (e *Executor) File() string {
	return path.Join(e.Directory, e.Name)
}

// Before we start a container, we verify that any previous container
// started by the same executor is terminated. Since there is no other
// changes-client with the same executor, this won't interfere with any
// running jobs but lets us clean up the environment from previous runs.
func (e *Executor) Clean() {
	if e.Name == "" {
		return;
	}

	// An error here is considered to be fine (in fact, normal)
	// since it indicates that the executor file doesn't exist,
	// which is the normal state if the previous run finished
	// execution cleanly.
	leftoverNameBytes, err := ioutil.ReadFile(e.File())
	if err == nil {
		leftoverName := string(leftoverNameBytes)
		log.Printf("[lxc] Detected leftover container: %s", leftoverName)
		os.Remove(e.File())
		container, err := lxc.NewContainer(leftoverName, lxc.DefaultConfigPath())

		// An error here probably indicates that the executor was killed
		// in such a bad state that the container was never finished being
		// created. We simply warn because this likely won't affect
		// the current run from proceeding.
		if err != nil {
			log.Printf("[lxc] Warning: Could not open leftover container: %s", leftoverName)
			return
		}
		if container.Running() {
			// The stop in the go-lxc api is not necessarily forceful
			// enough. We wish to guarantee that the container stops
			// so we use kill.
			//
			// XXX this could potentially have problems if there are
			// ever any rw-mounts into the container.
			log.Printf("[lxc] Killing leftover container: %s", leftoverName)
			err = exec.Command("lxc-stop", "-k", "-n", leftoverName).Run()
			if err != nil {
				log.Printf("[lxc] Error killing container: %s", err.Error())
				return
			}
		}
		// in theory kill should always prevent the container from running - but we check
		// and warn just to be sure.
		if container.Running() {
			log.Printf("[lxc] Warning: Couldn't kill leftover container: %s", leftoverName)
			return
		}

		log.Printf("[lxc] Destroying leftover container: %s", leftoverName)
		container.Destroy()
		if container.Defined() {
			log.Printf("[lxc] Warning: Couldn't destroy leftover container: %s", leftoverName)
			return
		}
		log.Printf("[lxc] Successfully cleaned up state for executor %s", e.Name)
	} else {
		if os.IsNotExist(err) {
			log.Printf("[lxc] Executor doesn't exist, pre-existing state appears clean.")
		} else {
			log.Printf("[lxc] An unexpected io error occurred: %s", err.Error());
		}
    }
}

// Create an executor file, registering the current container with the current
// executor.
func (e *Executor) Register(containerName string) {
	if e.Name == "" {
		return;
	}

	log.Printf("[lxc] Creating executor for %s with container %s",
		e.File(), containerName)
	err := ioutil.WriteFile(e.File(), []byte(containerName), 0644)
	if err != nil {
		log.Printf("[lxc] Warning: Couldn't create executor file")
	}
}

// By removing the executor we indicate that this run was cleanly finished
// and that the container was destroyed. If we are keeping the container,
// then we still remove the executor file to prevent another changes-client
// from forcibly destroying the container.
func (e *Executor) Deregister() {
	if e.Name == "" {
		return;
	}

	log.Printf("[lxc] Removing executor for %s", e.Name)
	err := os.Remove(e.File())
	if err != nil {
		log.Printf("[lxc] Warning: Unable to remove executor file")
	}
}

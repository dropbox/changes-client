// Copyright (c) 2012 Ingo Oeser

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package lockfile

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

type Lockfile struct {
	Path string
}

var (
	ErrBusy        = errors.New("Locked by other process") // If you get this, retry after a short sleep might help
	ErrNeedAbsPath = errors.New("Lockfiles must be given as absolute path names")
	ErrInvalidPid  = errors.New("Lockfile contains invalid pid for system")
	ErrDeadOwner   = errors.New("Lockfile contains pid of process not existent on this system anymore")
)

// Describe a new filename located at path. It is expected to be an absolute path
func New(path string) (*Lockfile, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrNeedAbsPath
	}
	return &Lockfile{path}, nil
}

// Who owns the lockfile?
func (l *Lockfile) GetOwner() (*os.Process, error) {
	name := l.Path

	// Ok, see, if we have a stale lockfile here
	content, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}

	var pid int
	_, err = fmt.Sscanln(string(content), &pid)
	if err != nil {
		return nil, ErrInvalidPid
	}

	// try hard for pids. If no pid, the lockfile is junk anyway and we delete it.
	if pid > 0 {
		p, err := os.FindProcess(pid)
		if err != nil {
			return nil, err
		}
		err = p.Signal(os.Signal(syscall.Signal(0)))
		if err == nil {
			return p, nil
		}
		errno, ok := err.(syscall.Errno)
		if !ok {
			return nil, err
		}

		switch errno {
		case syscall.ESRCH:
			return nil, ErrDeadOwner
		case syscall.EPERM:
			return p, nil
		default:
			return nil, err
		}
	} else {
		return nil, ErrInvalidPid
	}
	panic("Not reached")
}

// Try to get Lockfile lock. Returns nil, if successful and and error describing the reason, it didn't work out.
// Please note, that existing lockfiles containing pids of dead processes and lockfiles containing no pid at all
// are deleted.
func (l *Lockfile) TryLock() error {
	name := l.Path

	// This has been checked by New already. If we trigger here,
	// the caller didn't use New and re-implemented it's functionality badly.
	// So panic, that he might find this easily during testing.
	if !filepath.IsAbs(name) {
		panic(ErrNeedAbsPath)
	}

	tmplock, err := ioutil.TempFile(filepath.Dir(name), "")
	if err != nil {
		return err
	} else {
		defer tmplock.Close()
		defer os.Remove(tmplock.Name())
	}

	_, err = tmplock.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
	if err != nil {
		return err
	}

	// return value intentionally ignored, as ignoring it is part of the algorithm
	_ = os.Link(tmplock.Name(), name)

	fiTmp, err := os.Lstat(tmplock.Name())
	if err != nil {
		return err
	}
	fiLock, err := os.Lstat(name)
	if err != nil {
		return err
	}

	// Success
	if os.SameFile(fiTmp, fiLock) {
		return nil
	}

	_, err = l.GetOwner()
	switch err {
	default:
		// Other errors -> defensively fail and let caller handle this
		return err
	case nil:
		return ErrBusy
	case ErrDeadOwner, ErrInvalidPid:
		// cases we can fix below
	}

	// clean stale/invalid lockfile
	err = os.Remove(name)
	if err != nil {
		return err
	}

	// now that we cleaned up the stale lockfile, let's recurse
	return l.TryLock()
}

// Release a lock again. Returns any error that happend during release of lock.
func (l *Lockfile) Unlock() error {
	return os.Remove(l.Path)
}

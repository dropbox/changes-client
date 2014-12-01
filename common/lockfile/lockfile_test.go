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
// THE SOFTWARE.package lockfile_test

package lockfile_test

import (
	"github.com/dropbox/changes-client/common/lockfile"
	"fmt"
)

func ExampleLockfile() {
	lock, err := lockfile.New("/tmp/lock.me.now.lck")
	if err != nil {
		fmt.Println("Cannot init lock. reason: %v", err)
		panic(err)
	}
	err = lock.TryLock()

	// Error handling is essential, as we only try to get the lock.
	if err != nil {
		fmt.Println("Cannot lock \"%v\", reason: %v", lock, err)
		panic(err)
	}

	defer lock.Unlock()

	fmt.Println("Do stuff under lock")
	// Output: Do stuff under lock
}

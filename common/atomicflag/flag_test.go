package atomicflag

import "testing"

func TestInitialValue(t *testing.T) {
	x := &AtomicFlag{}

	if x.Get() {
		t.Error("Initial value of flag should be false")
	}
}

func TestSetFalse(t *testing.T) {
	x := &AtomicFlag{}

	x.Set(false)

	if x.Get() {
		t.Error("Value after setting false should be false")
	}
}

func TestSetTrue(t *testing.T) {
	x := &AtomicFlag{}

	x.Set(true)

	if !x.Get() {
		t.Error("Value after setting false should be true")
	}
}

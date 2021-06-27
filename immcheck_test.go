package immcheck_test

import (
	"immcheck"
	"strings"
	"testing"
	"unsafe"
)

func TestSimpleCounter(t *testing.T) {
	uintCounter := uint64(35)
	uintCounter++
	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&uintCounter)()
		uintCounter = 74574
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSliceOfIntegers(t *testing.T) {
	ints := make([]int, 1)
	ints[0] = 1
	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&ints)()
		ints[0] = 2
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSliceOfFloats(t *testing.T) {
	floats := make([]float64, 10)
	floats[0] = 3.0
	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&floats)()
		floats[0] = 2
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestPrimitiveStruct(t *testing.T) {
	type person struct {
		age    uint16
		height uint8
	}
	p := person{
		age:    13,
		height: 150,
	}
	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&p)()
		p.age = 0
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSliceOfPrimitiveStructs(t *testing.T) {
	type person struct {
		age    uint16
		height uint8
	}
	structs := make([]person, 2)
	structs[0].age = 3
	structs[1].age = 13
	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&structs)()
		structs[0].age = 0
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSliceOfNonPrimitiveStructs(t *testing.T) {
	type person struct {
		name   string
		age    uint16
		parent *person
	}
	structs := make([]person, 1)
	structs[0].age = 3
	structs[0].name = "First"
	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&structs)()
		structs[0].name = "Second"
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestMutationOfStringPropertyOfNestedNonPrimitiveStruct(t *testing.T) {
	type person struct {
		name   string
		age    uint16
		parent *person
	}
	grandParent := person{
		name:   "GrandParent",
		age:    100,
		parent: nil,
	}
	parent := person{
		name:   "Parent",
		age:    50,
		parent: &grandParent,
	}
	structs := make([]person, 3)
	structs[0] = person{
		name:   "Kid1",
		age:    25,
		parent: &parent,
	}
	structs[1] = person{
		name:   "Kid2",
		age:    26,
		parent: &parent,
	}
	structs[2] = person{
		name:   "Kid3",
		age:    27,
		parent: &parent,
	}

	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&structs)()
		grandParent.name = "ChangedName"
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestMutationOfUnsafeStringPropertyOfNestedNonPrimitiveStruct(t *testing.T) {
	type person struct {
		name   string
		age    uint16
		parent *person
	}
	grandParentNameBytes := []byte("GrandParent")
	grandParentName := *((*string)(unsafe.Pointer(&grandParentNameBytes)))
	grandParent := person{
		name:   grandParentName,
		age:    100,
		parent: nil,
	}
	parent := person{
		name:   "Parent",
		age:    50,
		parent: &grandParent,
	}
	array := [3]person{
		{
			name:   "Kid1",
			age:    25,
			parent: &parent,
		},
		{
			name:   "Kid2",
			age:    26,
			parent: &parent,
		},
		{
			name:   "Kid3",
			age:    27,
			parent: &parent,
		},
	}

	panicMessage := expectPanic(t, func() {
		defer immcheck.EnsureImmutability(&array)()
		grandParentNameBytes[0] = byte('g')
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func checkMutationDetectionMessage(t *testing.T, panicMessage string) {
	t.Helper()
	prefixIsCorrect := strings.HasPrefix(panicMessage, "mutation of immutable value detected")
	t.Log(panicMessage)
	if !prefixIsCorrect {
		t.Fatal("unexpected panic message: " + panicMessage)
	}
}

func expectPanic(t *testing.T, f func()) string {
	t.Helper()
	var expectedPanic interface{}
	func() {
		defer func() {
			expectedPanic = recover()
			return
		}()
		f()
	}()
	if expectedPanic == nil {
		t.Fatal("mutation isn't detected")
	}
	return expectedPanic.(string)
}

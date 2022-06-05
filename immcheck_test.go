package immcheck_test

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/goodbadreviewer/immcheck"
)

func TestRaceConditionalFunctionsEnabled(t *testing.T) {
	if !immcheck.ImmcheckRaceEnabled {
		t.SkipNow()
	}
	t.Parallel()
	{
		m := map[string]string{
			"k1": "v1",
		}
		logBuffer := &lockedWriterBuffer{buf: &bytes.Buffer{}}
		immcheck.RaceCheckImmutabilityOnFinalizationWithOptions(&m, immcheck.Options{
			Flags:     immcheck.SkipPanicOnDetectedMutation,
			LogWriter: logBuffer,
		})
		m["j1"] = "b1"

		runtime.GC()
		time.Sleep(10 * time.Millisecond)
		resultingLog := logBuffer.String()
		t.Log(resultingLog)
		logAsExpected := strings.Contains(
			resultingLog,
			"[ERROR] runtime mutation detected; "+
				"error: mutation of immutable value detected\nimmutable snapshot was captured here ",
		)
		if !logAsExpected {
			t.Fatalf("unnexpected log on finalization: `%v`", resultingLog)
		}
	}
	{
		m := map[string]string{
			"k1": "v1",
		}
		logBuffer := &lockedWriterBuffer{buf: &bytes.Buffer{}}
		immcheck.RaceCheckImmutabilityOnFinalizationWithOptions(&m, immcheck.Options{
			Flags:     immcheck.SkipPanicOnDetectedMutation,
			LogWriter: logBuffer,
		})
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
		resultingLog := logBuffer.String()
		if logBuffer.String() != "" {
			t.Fatalf("unnexpected log on finalization: %v", resultingLog)
		}
	}
	{
		ints := make([]int, 1)
		ints[0] = 1
		immcheck.RaceEnsureImmutability(&ints)() // check that no mutation is fine
		panicMessage := expectMutationPanic(t, func() {
			defer immcheck.RaceEnsureImmutability(&ints)()
			ints[0] = 2
		})
		checkMutationDetectionMessage(t, panicMessage)
	}
	{
		ints := make([]int, 1)
		ints[0] = 1
		immcheck.RaceEnsureImmutabilityWithOptions(
			&ints, immcheck.Options{Flags: immcheck.SkipOriginCapturing},
		)() // check that no mutation is fine
		panicMessage := expectMutationPanic(t, func() {
			defer immcheck.RaceEnsureImmutabilityWithOptions(
				&ints, immcheck.Options{Flags: immcheck.SkipOriginCapturing},
			)()
			ints[0] = 2
		})
		checkMutationDetectionMessage(t, panicMessage)
	}
}

func TestRaceConditionalFunctionsDisabled(t *testing.T) {
	if immcheck.ImmcheckRaceEnabled {
		t.SkipNow()
	}
	t.Parallel()
	{
		m := map[string]string{
			"k1": "v1",
		}
		logBuffer := &lockedWriterBuffer{buf: &bytes.Buffer{}}
		immcheck.RaceCheckImmutabilityOnFinalizationWithOptions(&m, immcheck.Options{
			Flags:     immcheck.SkipPanicOnDetectedMutation,
			LogWriter: logBuffer,
		})
		m["j1"] = "b1"

		runtime.GC()
		time.Sleep(10 * time.Millisecond)
		resultingLog := logBuffer.String()
		if logBuffer.String() != "" {
			t.Fatalf("unnexpected log on finalization: %v", resultingLog)
		}
	}
	{
		m := map[string]string{
			"k1": "v1",
		}
		logBuffer := &lockedWriterBuffer{buf: &bytes.Buffer{}}
		immcheck.RaceCheckImmutabilityOnFinalizationWithOptions(&m, immcheck.Options{
			Flags:     immcheck.SkipPanicOnDetectedMutation,
			LogWriter: logBuffer,
		})
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
		resultingLog := logBuffer.String()
		if logBuffer.String() != "" {
			t.Fatalf("unnexpected log on finalization: %v", resultingLog)
		}
	}
	{
		ints := make([]int, 1)
		ints[0] = 1
		immcheck.RaceEnsureImmutability(&ints)() // check that no mutation is fine
		func() {
			// Without race flag enabled, this should not panic
			defer immcheck.RaceEnsureImmutability(&ints)()
			ints[0] = 2
		}()
	}
	{
		ints := make([]int, 1)
		ints[0] = 1
		immcheck.RaceEnsureImmutabilityWithOptions(
			&ints, immcheck.Options{Flags: immcheck.SkipOriginCapturing},
		)() // check that no mutation is fine
		func() {
			// Without race flag enabled, this should not panic
			defer immcheck.RaceEnsureImmutabilityWithOptions(
				&ints, immcheck.Options{Flags: immcheck.SkipOriginCapturing},
			)()
			ints[0] = 2
		}()
	}
}

func TestExample(t *testing.T) {
	m := map[string]string{
		"k1": "v1",
	}
	m["k2"] = "v2"
	// returns function that you can call to verify that m didn't change
	checkFunction := immcheck.EnsureImmutability(&m)
	checkFunction() // no mutations are fine

	func() {
		defer func() {
			mutationPanic := recover()
			fmt.Println(mutationPanic)
		}()

		// you can also use defer for such checks
		// now when we mutate m, we will get panic at the end of the function
		defer immcheck.EnsureImmutability(&m)()

		// it is also possible to set a finalizer that can check
		// if object remained immutable from this point till garbage collection,
		// but it will fail this demonstration
		// immcheck.CheckImmutabilityOnFinalization(&m)

		// this function works only with `-race` or `-tags immcheck` build flags
		defer immcheck.RaceEnsureImmutability(&m)()

		// this function works only with `-race` or `-tags immcheck` build flags as well
		// so it can fail this demonstration
		// immcheck.RaceCheckImmutabilityOnFinalization(&m)

		delete(m, "k1")
	}()
}

func TestCheckImmutabilityWithOptionsOnFinalization(t *testing.T) {
	t.Parallel()
	{
		m := map[string]string{
			"k1": "v1",
		}
		logBuffer := &lockedWriterBuffer{buf: &bytes.Buffer{}}
		immcheck.CheckImmutabilityOnFinalizationWithOptions(&m, immcheck.Options{
			Flags:     immcheck.SkipPanicOnDetectedMutation,
			LogWriter: logBuffer,
		})
		m["j1"] = "b1"

		runtime.GC()
		time.Sleep(10 * time.Millisecond)
		resultingLog := logBuffer.String()
		t.Log(resultingLog)
		logAsExpected := strings.Contains(
			resultingLog,
			"[ERROR] runtime mutation detected; "+
				"error: mutation of immutable value detected\nimmutable snapshot was captured here ",
		)
		if !logAsExpected {
			t.Fatalf("unnexpected log on finalization: `%v`", resultingLog)
		}
	}
	{
		m := map[string]string{
			"k1": "v1",
		}
		logBuffer := &lockedWriterBuffer{buf: &bytes.Buffer{}}
		immcheck.CheckImmutabilityOnFinalizationWithOptions(&m, immcheck.Options{
			Flags:     immcheck.SkipPanicOnDetectedMutation,
			LogWriter: logBuffer,
		})
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
		resultingLog := logBuffer.String()
		if logBuffer.String() != "" {
			t.Fatalf("unnexpected log on finalization: %v", resultingLog)
		}
	}
}

func TestSimpleCounter(t *testing.T) {
	t.Parallel()
	uintCounter := uint64(35)
	uintCounter++
	immcheck.EnsureImmutability(&uintCounter)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&uintCounter)()
		uintCounter = 74574
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestNilTargetValue(t *testing.T) {
	t.Parallel()
	{
		panicMessage := expectPanic(t, func() {
			immcheck.EnsureImmutability(nil)()
		}, immcheck.UnsupportedTypeError)
		if !strings.Contains(panicMessage, "target value can't be nil") {
			t.Fatal("wrong error message")
		}
	}
	{
		panicMessage := expectPanic(t, func() {
			immcheck.CheckImmutabilityOnFinalization(nil)
		}, immcheck.UnsupportedTypeError)
		if !strings.Contains(panicMessage, "target value can't be nil") {
			t.Fatal("wrong error message")
		}
	}
}

func TestSimpleCounterManualCheck(t *testing.T) {
	t.Parallel()
	uintCounter := uint64(35)
	uintCounter++

	expectPanic(t, func() {
		snapshot := immcheck.NewValueSnapshot()
		otherSnapshot := immcheck.NewValueSnapshot()
		_ = snapshot.CheckImmutabilityAgainst(otherSnapshot)
	}, immcheck.InvalidSnapshotStateError)

	{
		// check that no mutation is fine
		snapshot := immcheck.CaptureSnapshot(&uintCounter, immcheck.NewValueSnapshot())
		fmt.Println(snapshot)
		otherSnapshot := immcheck.CaptureSnapshot(&uintCounter, immcheck.NewValueSnapshot())
		err := snapshot.CheckImmutabilityAgainst(otherSnapshot)
		if err != nil {
			t.Fatalf("enexpected error happened: %v", err)
		}
	}

	{
		// check that no mutation is fine
		snapshot := immcheck.CaptureSnapshot(&uintCounter, immcheck.NewValueSnapshot())
		uintCounter = 74574
		otherSnapshot := immcheck.CaptureSnapshotWithOptions(
			&uintCounter, immcheck.NewValueSnapshot(),
			immcheck.Options{},
		)
		err := snapshot.CheckImmutabilityAgainst(otherSnapshot)
		if err == nil {
			t.Fatal("no mutation detected")
		}
		if !errors.Is(err, immcheck.MutationDetectedError) {
			t.Fatalf("enexpected error happened: %v", err)
		}
		checkMutationDetectionMessage(t, err.Error())
	}
}

func TestSimpleCounterWithOptions(t *testing.T) {
	t.Parallel()
	uintCounter := uint64(35)
	uintCounter++
	immcheck.EnsureImmutabilityWithOptions(&uintCounter, immcheck.Options{
		Flags: immcheck.SkipOriginCapturing | immcheck.SkipLoggingOnMutation | immcheck.SkipPanicOnDetectedMutation,
	})() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutabilityWithOptions(&uintCounter, immcheck.Options{
			Flags: immcheck.SkipOriginCapturing,
		})()
		uintCounter = 74574
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestUnsafeWithNotAllowedUnsafeOption(t *testing.T) {
	t.Parallel()
	function := func() {}
	channel := make(chan int)
	counter := uint(0)
	unsafePointer := unsafe.Pointer(&counter)
	{
		panicMessage := expectPanic(t, func() {
			immcheck.EnsureImmutability(&function)
		}, immcheck.UnsupportedTypeError)
		checkUnsupportedTypeMessage(t, panicMessage, "func")
	}
	{
		panicMessage := expectPanic(t, func() {
			immcheck.EnsureImmutability(&channel)
		}, immcheck.UnsupportedTypeError)
		checkUnsupportedTypeMessage(t, panicMessage, "chan")
	}
	{
		panicMessage := expectPanic(t, func() {
			immcheck.EnsureImmutability(&unsafePointer)
		}, immcheck.UnsupportedTypeError)
		checkUnsupportedTypeMessage(t, panicMessage, "unsafe.Pointer")
	}
}

func TestSliceOfIntegers(t *testing.T) {
	t.Parallel()
	ints := make([]int, 1)
	ints[0] = 1
	immcheck.EnsureImmutability(&ints)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&ints)()
		ints[0] = 2
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSliceOfFloats(t *testing.T) {
	t.Parallel()
	floats := make([]float64, 10)
	floats[0] = 3.0
	immcheck.EnsureImmutability(&floats)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&floats)()
		floats[0] = 2
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestPrimitiveStruct(t *testing.T) {
	t.Parallel()
	type person struct {
		age    uint16
		height uint8
	}
	p := person{
		age:    13,
		height: 150,
	}
	immcheck.EnsureImmutability(&p)() // check that no mutation is fine
	p.age = 31
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&p)()
		p.age = 0
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSliceOfPrimitiveStructs(t *testing.T) {
	t.Parallel()
	type person struct {
		age    uint16
		height uint8
	}
	structs := make([]person, 2)
	structs[0].age = 3
	structs[1].age = 13
	immcheck.EnsureImmutability(&structs)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&structs)()
		structs[0].age = 0
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSliceOfNonPrimitiveStructs(t *testing.T) {
	t.Parallel()
	type person struct {
		name   string
		age    uint16
		parent *person
	}
	structs := make([]person, 1)
	structs[0].age = 3
	structs[0].name = "First"
	immcheck.EnsureImmutability(&structs)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&structs)()
		structs[0].name = "Second"
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestMutationOfStringPropertyOfNestedNonPrimitiveStruct(t *testing.T) {
	t.Parallel()
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

	immcheck.EnsureImmutability(&structs)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&structs)()
		grandParent.name = "ChangedName"
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestMutationOfUnsafeStringPropertyOfNestedNonPrimitiveStruct(t *testing.T) {
	t.Parallel()
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
	immcheck.EnsureImmutability(&array)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&array)()
		grandParentNameBytes[0] = byte('g')
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestLinkedList(t *testing.T) {
	t.Parallel()
	type node struct {
		value int
		next  *node
	}
	tail := &node{
		value: 1,
		next:  nil,
	}
	head := &node{
		value: 2,
		next:  tail,
	}
	head.value = 3
	immcheck.EnsureImmutability(&head)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&head)()
		tail.value = 4
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestRecursiveLinkedList(t *testing.T) {
	t.Parallel()
	type node struct {
		value int
		next  *node
	}
	tail := &node{
		value: 1,
		next:  nil,
	}
	head := &node{
		value: 2,
		next:  tail,
	}
	tail.next = head
	immcheck.EnsureImmutability(&head)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&head)()
		tail.value = 4
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestRecursiveInterfaceBasedLinkedList(t *testing.T) {
	t.Parallel()
	type node struct {
		value int
		next  interface{}
	}
	tail := &node{
		value: 1,
		next:  nil,
	}
	head := &node{
		value: 2,
		next:  tail,
	}
	tail.next = head
	immcheck.EnsureImmutability(&head)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&head)()
		tail.value = 4
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestUnsafePointer(t *testing.T) {
	t.Parallel()
	allowUnsafe := immcheck.Options{Flags: immcheck.AllowInherentlyUnsafeTypes}
	type person struct {
		age uint16
		ptr unsafe.Pointer
	}
	realPerson := &person{
		age: 13,
		ptr: unsafe.Pointer(nil),
	}
	p := unsafe.Pointer(realPerson)
	immcheck.EnsureImmutabilityWithOptions(&p, allowUnsafe)() // check that no mutation is fine

	immutabilityCheck := immcheck.EnsureImmutabilityWithOptions(&p, allowUnsafe)
	realPerson.age = 31
	immutabilityCheck() // mutation behind unsafe pointer won't be detected

	{
		panicMessage := expectMutationPanic(t, func() {
			defer immcheck.EnsureImmutabilityWithOptions(&p, allowUnsafe)()
			p = unsafe.Pointer(&person{})
		})
		checkMutationDetectionMessage(t, panicMessage)
	}
	{
		panicMessage := expectMutationPanic(t, func() {
			defer immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)()
			realPerson.ptr = unsafe.Pointer(&person{})
		})
		checkMutationDetectionMessage(t, panicMessage)
	}
}

func TestFunction(t *testing.T) {
	t.Parallel()
	allowUnsafe := immcheck.Options{Flags: immcheck.AllowInherentlyUnsafeTypes}
	type person struct {
		age uint16
		f   func()
	}
	i := 1
	realPerson := &person{
		age: 13,
		f: func() {
			fmt.Printf("hello: %v\n", &i)
		},
	}
	immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)() // check that no mutation is fine

	immutabilityCheck := immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)
	i = 2
	immutabilityCheck() // mutation of captuted scope won't be detected

	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)()
		realPerson.f = func() {}
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestChannel(t *testing.T) {
	t.Parallel()
	allowUnsafe := immcheck.Options{Flags: immcheck.AllowInherentlyUnsafeTypes}
	type person struct {
		age uint16
		ch  chan int
	}
	realPerson := &person{
		age: 13,
		ch:  make(chan int, 10),
	}
	immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)() // check that no mutation is fine

	{
		immutabilityCheck := immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)
		realPerson.ch <- 1
		immutabilityCheck() // channel sends won't be detected
	}

	{
		immutabilityCheck := immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)
		close(realPerson.ch)
		immutabilityCheck() // channel close won't be detected
	}

	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutabilityWithOptions(&realPerson, allowUnsafe)()
		realPerson.ch = make(chan int)
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestPrimitiveStructBehindInterface(t *testing.T) {
	t.Parallel()
	type person struct {
		age    uint16
		height uint8
	}
	realPerson := &person{
		age:    13,
		height: 150,
	}
	var p interface{} = realPerson
	immcheck.EnsureImmutability(&p)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&p)()
		realPerson.age = 0
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestPointerToSubslice(t *testing.T) {
	t.Parallel()
	type person struct {
		age    uint16
		height uint8
	}
	sliceOfPointers := []interface{}{
		[]interface{}{"otherSliceOfInterfaces", []byte("test")},
		[1]interface{}{[]byte{1, 2}},
		45,
		6.8,
		"someString",
		[]interface{}{},
		[0]interface{}{},
		[]interface{}{nil, person{age: 1, height: 12}, &person{age: 4, height: 32}},
		nil,
		nil,
		nil,
	}
	sliceOfPointers[8] = &sliceOfPointers[9]
	sliceOfPointers[9] = &sliceOfPointers[8]

	immcheck.EnsureImmutability(&sliceOfPointers)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutability(&sliceOfPointers)()
		sliceOfPointers[0].([]interface{})[1].([]byte)[0] = 'T'
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestSimpleMap(t *testing.T) {
	t.Parallel()
	allowUnsafe := immcheck.Options{Flags: immcheck.AllowInherentlyUnsafeTypes}
	data := map[string]interface{}{
		"b": 10,
	}
	data["data"] = data
	immcheck.EnsureImmutabilityWithOptions(&data, allowUnsafe)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutabilityWithOptions(&data, allowUnsafe)()
		data["b"] = 3
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func TestMap(t *testing.T) {
	t.Parallel()
	allowUnsafe := immcheck.Options{Flags: immcheck.AllowInherentlyUnsafeTypes}
	type person struct {
		age    uint16
		height uint8
		memory map[string]string
	}
	data := map[string]interface{}{
		"b": 10,
		"a": "a",
		"c": 5.6,
		"d": []*person{{age: 1, height: 2}},
		"e": map[int][]byte{1: []byte("hello")},
		"p": unsafe.Pointer(&person{}),
		"f": func() {},
	}
	data["data"] = data
	data["d"] = append(data["d"].([]*person), &person{
		age:    3,
		height: 4,
		memory: map[string]string{"f": "k"},
	})
	immcheck.EnsureImmutabilityWithOptions(&data, allowUnsafe)() // check that no mutation is fine
	panicMessage := expectMutationPanic(t, func() {
		defer immcheck.EnsureImmutabilityWithOptions(&data, allowUnsafe)()
		e := data["e"].(map[int][]byte)
		e[1][0] = 'H'
	})
	checkMutationDetectionMessage(t, panicMessage)
}

func checkMutationDetectionMessage(t *testing.T, panicMessage string) {
	t.Helper()
	t.Log(panicMessage)
	prefixIsCorrect := strings.HasPrefix(panicMessage, "mutation of immutable value detected")
	if !prefixIsCorrect {
		t.Fatal("unexpected panic message: " + panicMessage)
	}
	if strings.Contains(panicMessage, "immutable snapshot was captured here") {
		if strings.Count(panicMessage, "immcheck_test.go:") != 2 {
			t.Fatal("snapshot origin capturing is broken ")
		}
	}
}

func checkUnsupportedTypeMessage(t *testing.T, panicMessage string, expectedTypeStringInErrorMessage string) {
	t.Helper()
	prefixIsCorrect := strings.HasPrefix(
		panicMessage,
		"unsupported type for immutability check. "+
			"UnsafePointer, Func, and Chan types are not supported, "+
			"since there is no way for us to fully verify immutability for these types. "+
			"If you still want to proceed and ignore fields of such type "+
			"use Flags.AllowInherentlyUnsafeTypes option. Unsupported type kind: ",
	)
	sufixIsCorrect := strings.HasSuffix(panicMessage, expectedTypeStringInErrorMessage)
	t.Log(panicMessage)
	if !prefixIsCorrect || !sufixIsCorrect {
		t.Fatal("unexpected panic message: " + panicMessage)
	}
}

func expectMutationPanic(t *testing.T, f func()) string {
	t.Helper()
	return expectPanic(t, f, immcheck.MutationDetectedError)
}

func expectPanic(t *testing.T, f func(), expectedError error) string {
	t.Helper()
	var actualPanic interface{}
	func() {
		defer func() {
			actualPanic = recover()
			if expectedError != nil {
				if !errors.Is(actualPanic.(error), expectedError) {
					t.Fatalf(
						"unexpected error type. expected %T(%v); actual: %T(%v)",
						expectedError, expectedError, actualPanic, actualPanic,
					)
				}
			}
		}()
		f()
	}()
	if actualPanic == nil {
		t.Fatal("panic isn't detected")
	}
	return actualPanic.(error).Error()
}

type lockedWriterBuffer struct {
	m   sync.Mutex
	buf *bytes.Buffer
}

func (l *lockedWriterBuffer) String() string {
	l.m.Lock()
	defer l.m.Unlock()
	return l.buf.String()
}

func (l *lockedWriterBuffer) Write(p []byte) (n int, err error) {
	l.m.Lock()
	defer l.m.Unlock()
	return l.buf.Write(p)
}

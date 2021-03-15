package main

import (
	"fmt"
	"hash/maphash"
	"reflect"
	"unsafe"
)

type Person struct {
	Age    uint16
	Parent *Person
}

func main() {

	ints := make([]int, 1)
	ints[0] = 1
	floats := make([]float64, 10)
	floats[0] = 3.0
	structs := make([]Person, 1)
	structs[0].Age = 12

	defer EnsureImmutability(&ints)()
	defer EnsureImmutability(&floats)()
	defer EnsureImmutability(&structs)()

	//ints[0] = 1
	//floats[0] = 2.0
	structs[0].Age = 13
}

var seed = maphash.MakeSeed()

type ValueSnapshot struct {
	checksums      map[uintptr]uint64
	stringSnapshot string
}

func NewValueSnapshot(v interface{}) ValueSnapshot {
	return ValueSnapshot{
		checksums:      make(map[uintptr]uint64),
		stringSnapshot: fmt.Sprintf("%#v", v),
	}
}

func CaptureChecksumMap(snapshot ValueSnapshot, value reflect.Value) ValueSnapshot {
	h := maphash.Hash{}
	h.SetSeed(seed)
	switch value.Kind() {
	case reflect.Ptr:
		snapshot = CaptureChecksumMap(snapshot, value.Elem())
	case reflect.Slice:
		if value.IsZero() {
			snapshot.checksums[value.Pointer()] = uint64(value.Pointer())
		}
		valueSizeInBytes := value.Index(0).Type().Size()
		targetSliceHeader := *(*reflect.SliceHeader)(unsafe.Pointer(value.UnsafeAddr()))
		targetSliceHeader.Len *= int(valueSizeInBytes)
		targetSliceHeader.Cap *= int(valueSizeInBytes)

		targetSliceAsByteSlice := *(*[]byte)(unsafe.Pointer(&targetSliceHeader))
		_, _ = h.Write(targetSliceAsByteSlice)
		snapshot.checksums[value.Pointer()] = h.Sum64()
	}
	return snapshot
}

func EnsureImmutability(v interface{}) func() {
	if v == nil {
		return func() {}
	}

	originalSnapshot := NewValueSnapshot(v)
	targetValue := reflect.ValueOf(v)
	originalSnapshot = CaptureChecksumMap(originalSnapshot, targetValue)

	return func() {
		newSnapshot := NewValueSnapshot(v)
		newSnapshot = CaptureChecksumMap(newSnapshot, targetValue)
		if !reflect.DeepEqual(newSnapshot.checksums, originalSnapshot.checksums) {
			panic(fmt.Sprintf(
				"mutation of immutable value detected\nnewSnapshot.checksum: %+v\noldSnapshot.checksum: %+v\nnewSnapshot: %+v\noldSnapshot: %+v\n",
				newSnapshot.checksums, originalSnapshot.checksums, newSnapshot.stringSnapshot, originalSnapshot.stringSnapshot))
		}
	}
}

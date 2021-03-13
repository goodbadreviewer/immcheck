package main

import (
	"fmt"
	"hash/maphash"
	"reflect"
	"strconv"
)

func main() {
	m := make([]int, 1)
	m[0] = 1

	defer EnsureImmutability(&m)()
	defer EnsureImmutability(&m)()
	external(m)
}

var seed = maphash.MakeSeed()

func CaptureChecksumMap(checksums map[uintptr]uint64, value reflect.Value) map[uintptr]uint64 {
	h := maphash.Hash{}
	h.SetSeed(seed)
	switch value.Kind() {
	case reflect.Ptr:
		checksums = CaptureChecksumMap(checksums, value.Elem())
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			item := value.Index(i)
			switch item.Kind() {
			case reflect.Int:
				_, _ = h.WriteString(strconv.FormatInt(item.Int(), 10)) // error is impossible here
			}
		}
		checksums[value.Pointer()] = h.Sum64()
	}
	return checksums
}

func EnsureImmutability(v interface{}) func() {
	if v == nil {
		return func() {}
	}

	checksumMap := make(map[uintptr]uint64)
	targetValue := reflect.ValueOf(v)
	checksumMap = CaptureChecksumMap(checksumMap, targetValue)

	return func() {
		newChecksumMap := make(map[uintptr]uint64)
		newChecksumMap = CaptureChecksumMap(newChecksumMap, targetValue)
		fmt.Printf("new: %+v\nold: %+v\n", newChecksumMap, checksumMap)
		if !reflect.DeepEqual(checksumMap, newChecksumMap) {
			panic(fmt.Sprintf("v: %+v should be equal to %+v", checksumMap ,newChecksumMap))
		}
	}
}

func external(m []int) {
}

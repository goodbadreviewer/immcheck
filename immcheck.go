package immcheck

import (
	"fmt"
	"hash/maphash"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/davecgh/go-spew/spew"
	"github.com/sergi/go-diff/diffmatchpatch"
)

var seed = maphash.MakeSeed()

type checksumKey struct {
	p    uintptr
	kind reflect.Kind
}

type ValueSnapshot struct {
	captureOriginFile string
	captureOriginLine int

	checksums      map[checksumKey]uint64
	stringSnapshot string
}

func NewValueSnapshot(v interface{}) ValueSnapshot {
	return newValueSnapshot(v)
}

func newValueSnapshot(v interface{}) ValueSnapshot {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		panic("can't capture stack trace")
	}
	return ValueSnapshot{
		captureOriginFile: file,
		captureOriginLine: line,

		checksums: make(map[checksumKey]uint64, 8),
		//TODO: make stringSnapshot capturing, optional using build flag, of input arguments
		stringSnapshot: spew.Sdump(v),
	}
}

func CaptureChecksumMap(snapshot ValueSnapshot, value reflect.Value) ValueSnapshot {
	// TODO: detect pointer loops
	// TODO: introduce pooling of map hashes
	// TODO: add tests for nils and zero values
	// TODO: make strict variant that disallow UnsafePointer and Chan
	h := maphash.Hash{}
	h.SetSeed(seed)
	valueKind := value.Kind()
	switch valueKind {
	case reflect.Ptr:
		if value.IsNil() {
			snapshot.checksums[checksumKey{p: value.UnsafeAddr(), kind: valueKind}] = uint64(value.UnsafeAddr())
			return snapshot
		}
		snapshot = CaptureChecksumMap(snapshot, value.Elem())
		return snapshot
	case reflect.Array:
		if value.IsZero() {
			snapshot.checksums[checksumKey{p: value.UnsafeAddr(), kind: valueKind}] = uint64(value.UnsafeAddr())
			return snapshot
		}
		arrayLen := value.Len()
		valueSizeInBytes := int(value.Index(0).Type().Size())
		valuePointer := value.UnsafeAddr()
		valueBytes := reflect.SliceHeader{
			Data: valuePointer,
			Len:  arrayLen * valueSizeInBytes,
			Cap:  arrayLen * valueSizeInBytes,
		}
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		snapshot = sliceOrArraySnapshot(snapshot, value)
		return snapshot
	case reflect.Slice, reflect.String:
		if value.IsZero() {
			snapshot.checksums[checksumKey{p: value.UnsafeAddr(), kind: valueKind}] = uint64(value.UnsafeAddr())
			return snapshot
		}
		// TODO: capture slice len and cap, since they can change
		valueSizeInBytes := value.Index(0).Type().Size()
		targetSliceHeader := *(*reflect.SliceHeader)(unsafe.Pointer(value.UnsafeAddr()))
		targetSliceHeader.Len = targetSliceHeader.Cap * int(valueSizeInBytes) // capture everything
		targetSliceHeader.Cap *= int(valueSizeInBytes)
		snapshot = captureRawBytesLevelChecksum(snapshot, h, targetSliceHeader, valueKind)
		snapshot = sliceOrArraySnapshot(snapshot, value)
		return snapshot
	case reflect.Struct:
		valueSizeInBytes := int(value.Type().Size())
		valuePointer := value.UnsafeAddr()
		valueBytes := reflect.SliceHeader{
			Data: valuePointer,
			Len:  valueSizeInBytes,
			Cap:  valueSizeInBytes,
		}
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		if valueIsPrimitive(value) {
			return snapshot
		}
		numField := value.NumField()
		for i := 0; i < numField; i++ {
			if !valueIsPrimitive(value.Field(i)) {
				snapshot = CaptureChecksumMap(snapshot, value.Field(i))
			}
		}
		return snapshot
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		valueSizeInBytes := int(value.Type().Size())
		valuePointer := value.UnsafeAddr()
		valueBytes := reflect.SliceHeader{
			Data: valuePointer,
			Len:  valueSizeInBytes,
			Cap:  valueSizeInBytes,
		}
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		return snapshot
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.UnsafePointer:
		panic("unsupported type kind: " + valueKind.String())
	}
	return snapshot
}

func EnsureImmutability(v interface{}) func() {
	if v == nil {
		return func() {}
	}

	// TODO: introduce re-usage of ValueSnapshots
	originalSnapshot := newValueSnapshot(v)
	targetValue := reflect.ValueOf(v)
	originalSnapshot = CaptureChecksumMap(originalSnapshot, targetValue)

	return func() {
		newSnapshot := newValueSnapshot(v)
		newSnapshot = CaptureChecksumMap(newSnapshot, targetValue)
		if !reflect.DeepEqual(newSnapshot.checksums, originalSnapshot.checksums) {
			diff := diffmatchpatch.New()
			diffs := diff.DiffMain(originalSnapshot.stringSnapshot, newSnapshot.stringSnapshot, false)
			panic(fmt.Sprintf(
				"mutation of immutable value detected\n"+
					"immutable snapshot was captured here %v:%v\n"+
					"mutation was detected here %v:%v\n"+
					"oldSnapshot.checksum: %+v\nnewSnapshot.checksum: %+v\n\noldSnapshot: %+v\nnewSnapshot: %+v\n"+
					"Preaty Diff:\n%v",
				originalSnapshot.captureOriginFile, originalSnapshot.captureOriginLine,
				newSnapshot.captureOriginFile, newSnapshot.captureOriginLine,
				originalSnapshot.checksums, newSnapshot.checksums, originalSnapshot.stringSnapshot, newSnapshot.stringSnapshot,
				diff.DiffPrettyText(diffs),
			))
		}
	}
}

func sliceOrArraySnapshot(snapshot ValueSnapshot, value reflect.Value) ValueSnapshot {
	iterableLen := value.Len()
	if iterableLen == 0 || valueIsPrimitive(value.Index(0)) {
		return snapshot
	}
	for i := 0; i < iterableLen; i++ {
		snapshot = CaptureChecksumMap(snapshot, value.Index(i))
	}
	return snapshot
}

func valueIsPrimitive(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return true
	case reflect.Struct:
		// TODO: introduce per type cache
		numField := v.NumField()
		for i := 0; i < numField; i++ {
			if !valueIsPrimitive(v.Field(i)) {
				return false
			}
		}
		return true
	}
	return false
}

func captureRawBytesLevelChecksum(snapshot ValueSnapshot, hash maphash.Hash, valueBytes reflect.SliceHeader, valueKind reflect.Kind) ValueSnapshot {
	targetSliceAsByteSlice := *(*[]byte)(unsafe.Pointer(&valueBytes))
	_, _ = hash.Write(targetSliceAsByteSlice)
	snapshot.checksums[checksumKey{p: valueBytes.Data, kind: valueKind}] = hash.Sum64()
	return snapshot
}

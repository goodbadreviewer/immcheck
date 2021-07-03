package immcheck

import (
	"fmt"
	"hash/maphash"
	"reflect"
	"runtime"
	"strconv"
	"unsafe"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type ValueSnapshot struct {
	captureOriginFile string
	captureOriginLine int

	checksums      map[checksumKey]uint64
	stringSnapshot string
}

func NewValueSnapshot(v interface{}) ValueSnapshot {
	return newValueSnapshot(v)
}

func EnsureImmutability(v interface{}) func() {
	if v == nil {
		return func() {}
	}

	// TODO: introduce re-usage of ValueSnapshots
	originalSnapshot := newValueSnapshot(v)
	targetValue := reflect.ValueOf(v)
	originalSnapshot = captureChecksumMap(originalSnapshot, targetValue, nil)

	return func() {
		newSnapshot := newValueSnapshot(v)
		newSnapshot = captureChecksumMap(newSnapshot, targetValue, nil)
		//TODO: make manual checksum comparisons
		if !reflect.DeepEqual(newSnapshot.checksums, originalSnapshot.checksums) {
			diff := diffmatchpatch.New()
			snapshotDiffs := diff.DiffMain(originalSnapshot.stringSnapshot, newSnapshot.stringSnapshot, false)
			checksumDiffs := diff.DiffMain(fmt.Sprint(originalSnapshot.checksums), fmt.Sprint(newSnapshot.checksums), false)
			panic(fmt.Sprintf(
				"mutation of immutable value detected\n"+
					"immutable snapshot was captured here %v:%v\n"+
					"mutation was detected here %v:%v\n"+
					"oldSnapshot.checksum: %+v\nnewSnapshot.checksum: %+v\n"+
					"Checksum Diff: %v\n\n"+
					"oldSnapshot: %+v\nnewSnapshot: %+v\n"+
					"Preaty Diff:\n%v",
				originalSnapshot.captureOriginFile, originalSnapshot.captureOriginLine,
				newSnapshot.captureOriginFile, newSnapshot.captureOriginLine,
				originalSnapshot.checksums, newSnapshot.checksums,
				diff.DiffPrettyText(checksumDiffs),
				originalSnapshot.stringSnapshot, newSnapshot.stringSnapshot,
				diff.DiffPrettyText(snapshotDiffs),
			))
		}
	}
}

func newValueSnapshot(v interface{}) ValueSnapshot {
	skipTwoCallerFramesAndShowOnlyUsersCode := 2
	_, file, line, ok := runtime.Caller(skipTwoCallerFramesAndShowOnlyUsersCode)
	if !ok {
		panic("can't capture stack trace")
	}

	//TODO: pool ValueSnapshot instances, dump strings to reused byte slices and reuse checksum maps, etc
	oneBuckerCapacity := 8

	return ValueSnapshot{
		captureOriginFile: file,
		captureOriginLine: line,
		checksums:         make(map[checksumKey]uint64, oneBuckerCapacity),
		//TODO: make stringSnapshot capturing, optional using build flag, of input arguments
		stringSnapshot: fmt.Sprintf("%#+v", v),
	}
}

//nolint:gochecknoglobals // We really need this seed to be global to get the same checksums between different calls
var seed = maphash.MakeSeed()

func captureChecksumMap(snapshot ValueSnapshot, value reflect.Value, pointerToValue unsafe.Pointer) ValueSnapshot {
	// TODO: introduce pooling of map hashes
	// TODO: add tests for nils and zero values
	// TODO: make strict variant that disallow UnsafePointer and Chan
	h := &maphash.Hash{}
	h.SetSeed(seed)

	valueKind := value.Kind()
	switch valueKind {
	case reflect.Ptr, reflect.Interface:
		valuePointer := pointerOfValue(valueKind, value)
		if value.IsNil() {
			return capturePointer(snapshot, valuePointer, valueKind)
		}
		// detect ref loop and skip
		if _, ok := snapshot.checksums[checksumKey{p: uintptr(valuePointer), kind: valueKind}]; ok {
			return snapshot
		}
		snapshot = capturePointer(snapshot, valuePointer, valueKind)
		if valueKind == reflect.Ptr {
			valuePointer = nil
		}
		snapshot = captureChecksumMap(snapshot, value.Elem(), valuePointer)
		return snapshot
	case reflect.Array:
		valuePointer := pointerToValue
		if valuePointer == nil {
			valuePointer = unsafe.Pointer(value.UnsafeAddr())
		}
		if value.IsZero() {
			return capturePointer(snapshot, valuePointer, valueKind)
		}
		arrayLen := value.Len()
		valueSizeInBytes := int(value.Index(0).Type().Size())
		valueBytes := reflect.SliceHeader{
			Data: uintptr(valuePointer),
			Len:  arrayLen * valueSizeInBytes,
			Cap:  arrayLen * valueSizeInBytes,
		}
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		snapshot = sliceOrArraySnapshot(snapshot, value)
		return snapshot
	case reflect.Slice, reflect.String:
		valuePointer := pointerToValue
		if valuePointer == nil {
			valuePointer = unsafe.Pointer(value.UnsafeAddr())
		}
		if value.IsZero() || value.Len() == 0 {
			return capturePointer(snapshot, valuePointer, valueKind)
		}
		// TODO: capture slice len and cap, since they can change
		valueSizeInBytes := value.Index(0).Type().Size()
		//nolint:govet // unsafeptr: possible misuse of reflect.SliceHeader - Yes we know about it :)
		targetSliceHeader := *(*reflect.SliceHeader)(valuePointer)
		targetSliceHeader.Len *= int(valueSizeInBytes)
		targetSliceHeader.Cap = targetSliceHeader.Len
		snapshot = captureRawBytesLevelChecksum(snapshot, h, targetSliceHeader, valueKind)
		snapshot = sliceOrArraySnapshot(snapshot, value)
		return snapshot
	case reflect.Struct:
		valueSizeInBytes := int(value.Type().Size())
		valuePointer := pointerToValue
		if valuePointer == nil {
			valuePointer = unsafe.Pointer(value.UnsafeAddr())
		}
		valueBytes := reflect.SliceHeader{
			Data: uintptr(valuePointer),
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
				snapshot = captureChecksumMap(snapshot, value.Field(i), nil)
			}
		}
		return snapshot
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		valueSizeInBytes := int(value.Type().Size())
		valuePointer := pointerToValue
		if valuePointer == nil {
			valuePointer = unsafe.Pointer(value.UnsafeAddr())
		}
		valueBytes := reflect.SliceHeader{
			Data: uintptr(valuePointer),
			Len:  valueSizeInBytes,
			Cap:  valueSizeInBytes,
		}
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		return snapshot
	case reflect.Chan, reflect.Func, reflect.Map, reflect.UnsafePointer, reflect.Invalid:
		panic("unsupported type kind: " + valueKind.String())
	}
	return snapshot
}

type checksumKey struct {
	p    uintptr
	kind reflect.Kind
}

func (c checksumKey) String() string {
	hex := 16
	return c.kind.String() + "(" + strconv.FormatUint(uint64(c.p), hex) + ")"
}

func sliceOrArraySnapshot(snapshot ValueSnapshot, value reflect.Value) ValueSnapshot {
	iterableLen := value.Len()
	if iterableLen == 0 || valueIsPrimitive(value.Index(0)) {
		return snapshot
	}
	for i := 0; i < iterableLen; i++ {
		snapshot = captureChecksumMap(snapshot, value.Index(i), nil)
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
	case reflect.Array, reflect.Chan, reflect.Func, reflect.Interface, reflect.Invalid, reflect.Map,
		reflect.Ptr, reflect.Slice, reflect.String, reflect.UnsafePointer:
		return false
	}
	return false
}

func capturePointer(snapshot ValueSnapshot, valuePointer unsafe.Pointer, valueKind reflect.Kind) ValueSnapshot {
	snapshot.checksums[checksumKey{p: uintptr(valuePointer), kind: valueKind}] = uint64(uintptr(valuePointer))
	return snapshot
}

func captureRawBytesLevelChecksum(
	snapshot ValueSnapshot, hash *maphash.Hash,
	valueBytes reflect.SliceHeader, valueKind reflect.Kind,
) ValueSnapshot {
	//nolint:govet // unsafeptr: possible misuse of reflect.SliceHeader - Yes we know about it :)
	targetSliceAsByteSlice := *(*[]byte)(unsafe.Pointer(&valueBytes))
	_, _ = hash.Write(targetSliceAsByteSlice)
	snapshot.checksums[checksumKey{p: valueBytes.Data, kind: valueKind}] = hash.Sum64()
	return snapshot
}

//go:nocheckptr
func pointerOfValue(valueKind reflect.Kind, value reflect.Value) unsafe.Pointer {
	var valuePointer unsafe.Pointer
	if valueKind == reflect.Ptr {
		valuePointer = unsafe.Pointer(value.Pointer())
	} else if valueKind == reflect.Interface {
		valuePointer = unsafe.Pointer(value.InterfaceData()[1]) // get pointer from interface tuple
	}
	return valuePointer
}

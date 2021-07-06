package immcheck

import (
	"fmt"
	"hash/maphash"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type ValueSnapshot struct {
	captureOriginFile string
	captureOriginLine int

	checksums      map[checksumKey]uint64
	stringSnapshot string
}

func NewValueSnapshot(v interface{}) *ValueSnapshot {
	return newValueSnapshot(v)
}

func EnsureImmutability(v interface{}) func() {
	if v == nil {
		return func() {}
	}

	// TODO: introduce re-usage of ValueSnapshots
	originalSnapshot := newValueSnapshot(v)
	targetValue := reflect.ValueOf(v)
	originalSnapshot = captureChecksumMap(originalSnapshot, targetValue)

	return func() {
		newSnapshot := newValueSnapshot(v)
		newSnapshot = captureChecksumMap(newSnapshot, targetValue)
		// TODO: make manual checksum comparisons
		if !reflect.DeepEqual(newSnapshot.checksums, originalSnapshot.checksums) {
			diff := diffmatchpatch.New()
			snapshotDiffs := diff.DiffMain(originalSnapshot.stringSnapshot, newSnapshot.stringSnapshot, false)
			checksumDiffs := diff.DiffMain(fmt.Sprint(originalSnapshot.checksums), fmt.Sprint(newSnapshot.checksums), false)
			panic(fmt.Sprintf(
				"mutation of immutable value detected\n"+
					"immutable snapshot was captured here %v:%v\n"+
					"mutation was detected here %v:%v\n"+
					"oldSnapshot.checksum: %+v\nnewSnapshot.checksum: %+v\n"+
					"Checksum Diff       : %v\n\n"+
					"oldSnapshot: %+v\nnewSnapshot: %+v\n"+
					"Preaty Diff: %v",
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

func newValueSnapshot(v interface{}) *ValueSnapshot {
	skipTwoCallerFramesAndShowOnlyUsersCode := 2
	_, file, line, ok := runtime.Caller(skipTwoCallerFramesAndShowOnlyUsersCode)
	if !ok {
		panic("can't capture stack trace")
	}

	//TODO: pool ValueSnapshot instances, dump strings to reused byte slices and reuse checksum maps, etc
	oneBuckerCapacity := 8

	return &ValueSnapshot{
		captureOriginFile: file,
		captureOriginLine: line,
		checksums:         make(map[checksumKey]uint64, oneBuckerCapacity),
		//TODO: make stringSnapshot capturing, optional using build flag, of input arguments
		stringSnapshot: fmt.Sprintf("%#+v", v),
	}
}

//nolint:gochecknoglobals // We really need this seed to be global to get the same checksums between different calls
var seed = maphash.MakeSeed()

func captureChecksumMap(snapshot *ValueSnapshot, value reflect.Value) *ValueSnapshot {
	// TODO: introduce pooling of map hashes
	// TODO: add tests for nils and zero values
	// TODO: make strict variant that disallow UnsafePointer and Chan
	h := &maphash.Hash{}
	h.SetSeed(seed)

	valueKind := value.Kind()
	switch valueKind {
	case reflect.UnsafePointer, reflect.Func, reflect.Chan:
		return capturePointer(snapshot, unsafe.Pointer(value.Pointer()), valueKind)
	case reflect.Ptr, reflect.Interface:
		valuePointer := pointerOfValue(value)
		if value.IsNil() {
			return capturePointer(snapshot, valuePointer, valueKind)
		}
		// detect ref loop and skip
		if _, ok := snapshot.checksums[checksumKey{p: uintptr(valuePointer), kind: valueKind}]; ok {
			return snapshot
		}
		snapshot = capturePointer(snapshot, valuePointer, valueKind)
		snapshot = captureChecksumMap(snapshot, value.Elem())
		return snapshot
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		valueBytes := converValueTypeToBytesSlice(value)
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		return snapshot
	case reflect.Struct:
		valueBytes := converValueTypeToBytesSlice(value)
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		snapshot = perFieldSnapshot(snapshot, value)
		return snapshot
	case reflect.Array, reflect.Slice, reflect.String:
		valueBytes := convertSliceBasedTypeToByteSlice(value)
		snapshot = captureRawBytesLevelChecksum(snapshot, h, valueBytes, valueKind)
		snapshot = perItemSnapshot(snapshot, value)
		return snapshot
	case reflect.Map:
		valuePointer := pointerOfValue(value)
		if value.IsNil() || value.IsZero() {
			return capturePointer(snapshot, valuePointer, valueKind)
		}
		snapshot.checksums[checksumKey{p: uintptr(valuePointer), kind: valueKind}] = uint64(value.Len())
		snapshot = perEntrySnapshot(snapshot, value)
		return snapshot
	case reflect.Invalid:
		panic("unsupported type kind: " + valueKind.String())
	}
	return snapshot
}

type checksumKey struct {
	p    uintptr
	kind reflect.Kind
}

func (c checksumKey) String() string {
	return fmt.Sprintf("%s(%#x)", c.kind.String(), c.p)
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

func perEntrySnapshot(snapshot *ValueSnapshot, value reflect.Value) *ValueSnapshot {
	mapRange := value.MapRange()
	for mapRange.Next() {
		k := mapRange.Key()
		v := mapRange.Value()
		snapshot = captureChecksumMap(snapshot, k)
		snapshot = captureChecksumMap(snapshot, v)
	}
	return snapshot
}

func perFieldSnapshot(snapshot *ValueSnapshot, value reflect.Value) *ValueSnapshot {
	if valueIsPrimitive(value) {
		return snapshot
	}
	numField := value.NumField()
	for i := 0; i < numField; i++ {
		if !valueIsPrimitive(value.Field(i)) {
			snapshot = captureChecksumMap(snapshot, value.Field(i))
		}
	}
	return snapshot
}

func perItemSnapshot(snapshot *ValueSnapshot, value reflect.Value) *ValueSnapshot {
	iterableLen := value.Len()
	if iterableLen == 0 || valueIsPrimitive(value.Index(0)) {
		return snapshot
	}
	for i := 0; i < iterableLen; i++ {
		snapshot = captureChecksumMap(snapshot, value.Index(i))
	}
	return snapshot
}

func capturePointer(snapshot *ValueSnapshot, valuePointer unsafe.Pointer, valueKind reflect.Kind) *ValueSnapshot {
	snapshot.checksums[checksumKey{p: uintptr(valuePointer), kind: valueKind}] = uint64(uintptr(valuePointer))
	return snapshot
}

func captureRawBytesLevelChecksum(
	snapshot *ValueSnapshot, hash *maphash.Hash,
	valueBytes []byte, valueKind reflect.Kind,
) *ValueSnapshot {

	hash.Reset()
	_, _ = hash.Write(valueBytes)
	snapshot.checksums[checksumKey{p: uintptr(hash.Sum64()), kind: valueKind}] = hash.Sum64()
	return snapshot
}

func converValueTypeToBytesSlice(value reflect.Value) []byte {
	result := []byte{}
	targetByteSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&result))

	valuePointer := pointerOfValue(value)
	valueSizeInBytes := int(value.Type().Size())

	targetByteSliceHeader.Data = uintptr(valuePointer)
	targetByteSliceHeader.Len = valueSizeInBytes
	targetByteSliceHeader.Cap = valueSizeInBytes
	return result
}

func convertSliceBasedTypeToByteSlice(value reflect.Value) []byte {
	result := []byte{}
	targetByteSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&result))

	valuePointer := pointerOfValue(value)
	arrayLen := value.Len()
	valueSizeInBytes := 0
	if arrayLen != 0 {
		valueSizeInBytes = int(value.Index(0).Type().Size())
	}

	targetByteSliceHeader.Data = uintptr(valuePointer)
	targetByteSliceHeader.Len = arrayLen * valueSizeInBytes
	targetByteSliceHeader.Cap = arrayLen * valueSizeInBytes
	return result
}

func pointerOfValue(value reflect.Value) unsafe.Pointer {
	//nolint:exhaustive
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return unsafe.Pointer(value.Pointer())
	case reflect.String:
		return fetchDataPointerFromString(value)
	case reflect.Interface:
		return fetchDataPointerFromInterfaceData(value)
	}
	if value.CanInterface() {
		return fetchPointerFromValueInterface(value)
	}
	if value.CanAddr() {
		return unsafe.Pointer(value.Addr().Pointer())
	}
	panic(fmt.Sprintf("can't get pointer to value. kind: %#v; value: %#v", value.Kind().String(), value))
}

func fetchDataPointerFromString(value reflect.Value) unsafe.Pointer {
	stringValue := value.String()
	return unsafe.Pointer(((*reflect.StringHeader)(unsafe.Pointer(&stringValue))).Data)
}

//go:nocheckptr
func fetchDataPointerFromInterfaceData(value reflect.Value) unsafe.Pointer {
	runtime.KeepAlive(value)
	return unsafe.Pointer(value.InterfaceData()[1])
}

//go:nocheckptr
func fetchPointerFromValueInterface(value reflect.Value) unsafe.Pointer {
	runtime.KeepAlive(value)
	vI := value.Interface()
	return unsafe.Pointer((*[2]uintptr)(unsafe.Pointer(&vI))[1])
}

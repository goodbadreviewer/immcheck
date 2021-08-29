package immcheck

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/cespare/xxhash/v2"
)

const MutationDetectedError mutationDetectionError = "mutation of immutable value detected"
const InvalidSnapshotStateError mutationDetectionError = "invalid snapshot state"
const UnsupportedTypeError mutationDetectionError = "unsupported type for immutability check"

type immutabilityCheckFlag uint8

const (
	// SkipOriginCapturing forces immcheck to not capture caller information to report snapshot origin.
	// This option gives a tiny bit more performance.
	SkipOriginCapturing immutabilityCheckFlag = 1 << iota
	// AllowInherentlyUnsafeTypes forces immcheck to allow reflect.UnsafePointer, reflect.Func and reflect.Chan
	// inside target value.
	AllowInherentlyUnsafeTypes
	// SkipPanicOnDetectedMutation forces immcheck to not panic in
	// immcheck.EnsureImmutability and immcheck.CheckImmutabilityOnFinalization methods when mutation is detected.
	SkipPanicOnDetectedMutation
	// SkipLoggingOnMutation forces immcheck to not log details of found mutation
	// in immcheck.EnsureImmutability and immcheck.CheckImmutabilityOnFinalization methods.
	SkipLoggingOnMutation
)

// Options configures immutability check.
type Options struct {
	// Specifies logger output stream. Can be nil. immcheck uses os.Stderr by default.
	LogWriter io.Writer
	// Bitmask of ImmutabilityCheckFlags.
	// You can specify it like that: SkipOriginCapturing | SkipLoggingOnMutation | AllowInherentlyUnsafeTypes
	Flags immutabilityCheckFlag
}

// ValueSnapshot is a re-usable object of snapshot value that works similar to bytes.Buffer.
// You can create new ValueSnapshot object using immcheck.NewValueSnapshot method.
// Capture snapshots into it using immcheck.CaptureSnapshot or immcheck.CaptureSnapshotWithOptions.
// Then you can compare snapshots using ValueSnapshot.CheckImmutabilityAgainst method.
// Then you can re-use snapshots by calling ValueSnapshot.Reset.
// This approach can help you to avoid extra allocations.
type ValueSnapshot struct {
	captureOriginFile *bytes.Buffer
	captureOriginLine int

	checksums map[uint32]uint32
}

// NewValueSnapshot creates new re-usable object of snapshot object.
func NewValueSnapshot() *ValueSnapshot {
	return newValueSnapshot()
}

// Reset clear internal state of ValueSnapshot, so it can be re-used.
func (v *ValueSnapshot) Reset() {
	v.captureOriginFile.Reset()
	v.captureOriginLine = 0
	for key := range v.checksums {
		delete(v.checksums, key)
	}
}

// String provides string representation of ValueSnapshot.
func (v *ValueSnapshot) String() string {
	buf := &bytes.Buffer{}
	buf.WriteString("ValueSnapshot{")
	if v.captureOriginFile.Len() != 0 && v.captureOriginLine != 0 {
		buf.WriteString("origin: ")
		buf.Write(v.captureOriginFile.Bytes())
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(v.captureOriginLine))
		buf.WriteString("; ")
	}
	buf.WriteString("checksumSize: ")
	_, _ = fmt.Fprintf(buf, "%v", len(v.checksums))
	buf.WriteByte('}')
	return buf.String()
}

// CheckImmutabilityAgainst verifies that otherSnapshot is exactly the same as this one.
// Returns immcheck.MutationDetectedError if snapshots are different.
func (v *ValueSnapshot) CheckImmutabilityAgainst(otherSnapshot *ValueSnapshot) error {
	if len(v.checksums) == 0 || len(otherSnapshot.checksums) == 0 {
		panic(fmt.Errorf("%w snapshot is empty", InvalidSnapshotStateError))
	}
	originalSnapshot := v
	newSnapshot := otherSnapshot
	if checksumEquals(newSnapshot.checksums, originalSnapshot.checksums) {
		return nil
	}

	originalSnapshotOrigin := ""
	if originalSnapshot.captureOriginFile.Len() != 0 && originalSnapshot.captureOriginLine != 0 {
		originalSnapshotOrigin = fmt.Sprintf(
			"immutable snapshot was captured here %v:%v\n",
			originalSnapshot.captureOriginFile, originalSnapshot.captureOriginLine,
		)
	}
	newSnapshotOrigin := ""
	if newSnapshot.captureOriginFile.Len() != 0 && newSnapshot.captureOriginLine != 0 {
		newSnapshotOrigin = fmt.Sprintf(
			"mutation was detected here %v:%v\n",
			newSnapshot.captureOriginFile, newSnapshot.captureOriginLine,
		)
	}

	return fmt.Errorf(
		"%w\n%v%v",
		MutationDetectedError, originalSnapshotOrigin, newSnapshotOrigin,
	)
}

// CaptureSnapshot creates lightweight checksum representation of v and stores if into dst.
// Returns modified dst object.
func CaptureSnapshot(v interface{}, dst *ValueSnapshot) *ValueSnapshot {
	skipTwoFrames := 2
	snapshot := initValueSnapshot(dst, Options{}, skipTwoFrames)
	targetValue := reflect.ValueOf(v)
	snapshot = captureChecksumMap(snapshot, targetValue, Options{})
	return snapshot
}

// CaptureSnapshotWithOptions creates lightweight checksum according to settings specified in options,
// representation of v and stores if into dst. Returns modified dst object.
func CaptureSnapshotWithOptions(v interface{}, dst *ValueSnapshot, options Options) *ValueSnapshot {
	skipTwoFrames := 2
	snapshot := initValueSnapshot(dst, options, skipTwoFrames)
	targetValue := reflect.ValueOf(v)
	snapshot = captureChecksumMap(snapshot, targetValue, options)
	return snapshot
}

// EnsureImmutability captures checksum of v and returns function that can be called to verify that v was not mutated.
// Returned function can be called multiple times.
// If mutation is detected returned function will panic.
func EnsureImmutability(v interface{}) func() {
	return ensureImmutability(v, Options{})
}

// EnsureImmutabilityWithOptions captures checksum of v according to settings specified in options
// and returns function that can be called to verify that v was not mutated.
// Returned function can be called multiple times.
// If mutation is detected returned function will panic.
func EnsureImmutabilityWithOptions(v interface{}, options Options) func() {
	return ensureImmutability(v, options)
}

// CheckImmutabilityOnFinalization captures checksum of v and sets finalizer on v
// to check if it was mutated during its lifetime.
// If mutation is detected finalizer will log details and panic which will stop the process.
// If you don't want to exit on detected mutation use
// immcheck.CheckImmutabilityOnFinalizationWithOptions and override default flags.
func CheckImmutabilityOnFinalization(v interface{}) {
	checkImmutabilityOnFinalization(v, Options{})
}

// CheckImmutabilityOnFinalizationWithOptions captures checksum of v and sets finalizer on v
// to check if it was mutated during its lifetime.
// If mutation is detected finalizer will log details and panic which will stop the process.
// If you don't want to exit on detected mutation override default flags.
func CheckImmutabilityOnFinalizationWithOptions(v interface{}, options Options) {
	checkImmutabilityOnFinalization(v, options)
}

//nolint:gochecknoglobals // tempSnapshotsPool is global to maximise snapshot objects re-use
var tempSnapshotsPool = &sync.Pool{
	New: func() interface{} {
		return newValueSnapshot()
	},
}

func checkImmutabilityOnFinalization(v interface{}, options Options) {
	if v == nil {
		panic(fmt.Errorf("%w. target value can't be nil", UnsupportedTypeError))
	}
	originalSnapshot := tempSnapshotsPool.Get().(*ValueSnapshot) // finalizer returns this snapshot to the pool
	skipThreeFrames := 3
	originalSnapshot = initValueSnapshot(originalSnapshot, options, skipThreeFrames)
	originalSnapshot = captureChecksumMap(originalSnapshot, reflect.ValueOf(v), options)

	runtime.SetFinalizer(v, func(v interface{}) {
		runInPool(func() {
			newSnapshot := tempSnapshotsPool.Get().(*ValueSnapshot)
			defer tempSnapshotsPool.Put(newSnapshot)
			defer tempSnapshotsPool.Put(originalSnapshot)

			funcWillBeInvokedByAsyncPoolSoSkipOneFrame := 1
			newSnapshot = initValueSnapshot(newSnapshot, options, funcWillBeInvokedByAsyncPoolSoSkipOneFrame)
			newSnapshot = captureChecksumMap(newSnapshot, reflect.ValueOf(v), options)
			checkErr := originalSnapshot.CheckImmutabilityAgainst(newSnapshot)
			if checkErr != nil {
				reportError(v, checkErr, options)
			}
		})
	})
}

func ensureImmutability(v interface{}, options Options) func() {
	if v == nil {
		panic(fmt.Errorf("%w. target value can't be nil", UnsupportedTypeError))
	}
	originalSnapshot := tempSnapshotsPool.Get().(*ValueSnapshot) // callback returns this snapshot to the pool
	skipThreeFrames := 3
	originalSnapshot = initValueSnapshot(originalSnapshot, options, skipThreeFrames)
	targetValue := reflect.ValueOf(v)
	originalSnapshot = captureChecksumMap(originalSnapshot, targetValue, options)

	return func() {
		newSnapshot := tempSnapshotsPool.Get().(*ValueSnapshot)
		defer tempSnapshotsPool.Put(newSnapshot)
		defer tempSnapshotsPool.Put(originalSnapshot)

		thisFuncWillBeInvokedByClientCodeSoSkipOnlyTwoFrames := 2
		newSnapshot = initValueSnapshot(newSnapshot, options, thisFuncWillBeInvokedByClientCodeSoSkipOnlyTwoFrames)
		newSnapshot = captureChecksumMap(newSnapshot, targetValue, options)
		checkErr := originalSnapshot.CheckImmutabilityAgainst(newSnapshot)
		if checkErr != nil {
			reportError(v, checkErr, options)
		}
	}
}

func reportError(v interface{}, checkErr error, options Options) {
	if options.Flags&SkipLoggingOnMutation == 0 {
		var logDestination io.Writer = os.Stderr
		if options.LogWriter != nil {
			logDestination = options.LogWriter
		}
		_, _ = fmt.Fprintf(
			logDestination,
			"[ERROR] runtime mutation detected. value: `%#v`; error: %v\n",
			v, checkErr,
		)
	}
	if options.Flags&SkipPanicOnDetectedMutation == 0 {
		panic(checkErr)
	}
}

func newValueSnapshot() *ValueSnapshot {
	oneBucketCapacity := 8
	return &ValueSnapshot{
		captureOriginFile: &bytes.Buffer{},
		captureOriginLine: 0,
		checksums:         make(map[uint32]uint32, oneBucketCapacity),
	}
}

func initValueSnapshot(
	dst *ValueSnapshot,
	options Options, framesToSkip int) *ValueSnapshot {
	dst.Reset()
	if options.Flags&SkipOriginCapturing == 0 {
		skipCallerFramesAndShowOnlyUsersCode := framesToSkip
		_, file, line, ok := runtime.Caller(skipCallerFramesAndShowOnlyUsersCode)
		if !ok {
			panic("can't capture stack trace")
		}
		dst.captureOriginFile.WriteString(file)
		dst.captureOriginLine = line
	}
	return dst
}

func captureChecksumMap(snapshot *ValueSnapshot, value reflect.Value, options Options) *ValueSnapshot {
	valueKind := value.Kind()
	switch valueKind {
	case reflect.UnsafePointer, reflect.Func, reflect.Chan:
		if options.Flags&AllowInherentlyUnsafeTypes == 0 {
			panic(fmt.Errorf("%w. UnsafePointer, Func, and Chan types are not supported, "+
				"since there is no way for us to fully verify immutability for these types. "+
				"If you still want to proceed and ignore fields of such type "+
				"use Flags.AllowInherentlyUnsafeTypes option. "+
				"Unsupported type kind: %v", UnsupportedTypeError, valueKind.String()))
		}
		return capturePointer(snapshot, unsafe.Pointer(value.Pointer()), valueKind)
	case reflect.Ptr, reflect.Interface:
		valuePointer := pointerOfValue(value)
		if value.IsNil() {
			return capturePointer(snapshot, valuePointer, valueKind)
		}
		// detect ref loop and skip
		if _, ok := snapshot.checksums[evalKey(uintptr(valuePointer), valueKind)]; ok {
			return snapshot
		}
		snapshot = capturePointer(snapshot, valuePointer, valueKind)
		snapshot = captureChecksumMap(snapshot, value.Elem(), options)
		return snapshot
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		valueBytes := convertValueTypeToBytesSlice(value)
		snapshot = captureRawBytesLevelChecksum(snapshot, valueBytes, valueKind)
		return snapshot
	case reflect.Struct:
		valueBytes := convertValueTypeToBytesSlice(value)
		snapshot = captureRawBytesLevelChecksum(snapshot, valueBytes, valueKind)
		snapshot = perFieldSnapshot(snapshot, value, options)
		return snapshot
	case reflect.Array, reflect.Slice, reflect.String:
		valueBytes := convertSliceBasedTypeToByteSlice(value)
		snapshot = captureRawBytesLevelChecksum(snapshot, valueBytes, valueKind)
		snapshot = perItemSnapshot(snapshot, value, options)
		return snapshot
	case reflect.Map:
		valuePointer := pointerOfValue(value)
		if value.IsNil() || value.IsZero() {
			return capturePointer(snapshot, valuePointer, valueKind)
		}
		snapshot.checksums[evalKey(uintptr(valuePointer), valueKind)] = uint32(value.Len())
		snapshot = perEntrySnapshot(snapshot, value, options)
		return snapshot
	case reflect.Invalid:
		panic(fmt.Errorf("%w, unsupported type kind: %v", UnsupportedTypeError, valueKind.String()))
	}
	return snapshot
}

//go:nosplit
func evalKey32(valuePointer uint32, kind reflect.Kind) uint32 {
	return valuePointer ^ uint32(kind)
}

//go:nosplit
func evalKey(valuePointer uintptr, kind reflect.Kind) uint32 {
	return uint32(valuePointer) ^ uint32(kind)
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

func perEntrySnapshot(snapshot *ValueSnapshot, value reflect.Value, options Options) *ValueSnapshot {
	mapRange := value.MapRange()
	for mapRange.Next() {
		k := mapRange.Key()
		v := mapRange.Value()
		snapshot = captureChecksumMap(snapshot, k, options)
		snapshot = captureChecksumMap(snapshot, v, options)
	}
	return snapshot
}

func perFieldSnapshot(snapshot *ValueSnapshot, value reflect.Value, options Options) *ValueSnapshot {
	if valueIsPrimitive(value) {
		return snapshot
	}
	numField := value.NumField()
	for i := 0; i < numField; i++ {
		if !valueIsPrimitive(value.Field(i)) {
			snapshot = captureChecksumMap(snapshot, value.Field(i), options)
		}
	}
	return snapshot
}

func perItemSnapshot(snapshot *ValueSnapshot, value reflect.Value, options Options) *ValueSnapshot {
	iterableLen := value.Len()
	if iterableLen == 0 || valueIsPrimitive(value.Index(0)) {
		return snapshot
	}
	for i := 0; i < iterableLen; i++ {
		snapshot = captureChecksumMap(snapshot, value.Index(i), options)
	}
	return snapshot
}

//go:nosplit
func capturePointer(snapshot *ValueSnapshot, valuePointer unsafe.Pointer, valueKind reflect.Kind) *ValueSnapshot {
	snapshot.checksums[evalKey(uintptr(valuePointer), valueKind)] = uint32(uintptr(valuePointer))
	return snapshot
}

//go:nosplit
func captureRawBytesLevelChecksum(
	snapshot *ValueSnapshot,
	valueBytes []byte, valueKind reflect.Kind,
) *ValueSnapshot {
	var hashSum uint32
	if len(valueBytes) > 64 {
		hashSum = crc32.ChecksumIEEE(valueBytes)
	} else {
		hashSum = uint32(xxhash.Sum64(valueBytes))
	}
	snapshot.checksums[evalKey32(hashSum, valueKind)] = hashSum
	return snapshot
}

//go:nosplit
func convertValueTypeToBytesSlice(value reflect.Value) []byte {
	var result []byte
	targetByteSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&result))

	valuePointer := pointerOfValue(value)
	valueSizeInBytes := int(value.Type().Size())

	targetByteSliceHeader.Data = uintptr(valuePointer)
	targetByteSliceHeader.Len = valueSizeInBytes
	targetByteSliceHeader.Cap = valueSizeInBytes
	return result
}

//go:nosplit
func convertSliceBasedTypeToByteSlice(value reflect.Value) []byte {
	var result []byte
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

//go:nosplit
func pointerOfValue(value reflect.Value) unsafe.Pointer {
	//nolint:exhaustive
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return unsafe.Pointer(value.Pointer())
	case reflect.String:
		return fetchDataPointerFromString(value)
	}
	if value.CanAddr() {
		return unsafe.Pointer(value.Addr().Pointer())
	}
	if value.CanInterface() {
		return fetchPointerFromValueInterface(value)
	}
	panic(fmt.Sprintf("can't get pointer to value. kind: %#v; value: %#v", value.Kind().String(), value))
}

//go:nosplit
func fetchDataPointerFromString(value reflect.Value) unsafe.Pointer {
	stringValue := value.String()
	return unsafe.Pointer(((*reflect.StringHeader)(unsafe.Pointer(&stringValue))).Data)
}

//go:nocheckptr
func fetchPointerFromValueInterface(value reflect.Value) unsafe.Pointer {
	vI := value.Interface()
	return unsafe.Pointer((*[2]uintptr)(unsafe.Pointer(&vI))[1])
}

type mutationDetectionError string

func (m mutationDetectionError) Error() string {
	return string(m)
}

func checksumEquals(newChecksum map[uint32]uint32, originalChecksum map[uint32]uint32) bool {
	if len(newChecksum) != len(originalChecksum) {
		return false
	}
	for newSnapshotKey, newSnapshotValue := range newChecksum {
		originalSnapshotValue, ok := originalChecksum[newSnapshotKey]
		if !ok {
			return false
		}
		if newSnapshotValue != originalSnapshotValue {
			return false
		}
	}
	return true
}

//nolint:gochecknoglobals // taskQueue is global to maximise goroutine pool utilization
var taskQueue = make(chan func())

//go:nosplit
func runInPool(task func()) {
	select {
	case taskQueue <- task:
		// submitted, everything is ok
	default:
		go func() {
			// do the given task
			task()

			const cleanupDuration = 10 * time.Second
			cleanupTicker := time.NewTicker(cleanupDuration)
			defer cleanupTicker.Stop()

			for {
				select {
				case t := <-taskQueue:
					t()
					cleanupTicker.Reset(cleanupDuration)
				case <-cleanupTicker.C:
					return
				}
			}
		}()
	}
}

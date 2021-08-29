//go:build !arm64
// +build !arm64

package immcheck

import (
	"hash/crc32"

	"github.com/cespare/xxhash/v2"
)

//go:nosplit
func hashSum(valueBytes []byte) uint32 {
	var hashSum uint32
	biggerSliceThreshold := 256
	if len(valueBytes) > biggerSliceThreshold {
		// crc32 measured to be more effective for values bigger values
		hashSum = crc32.ChecksumIEEE(valueBytes)
	} else {
		hashSum = uint32(xxhash.Sum64(valueBytes))
	}
	return hashSum
}

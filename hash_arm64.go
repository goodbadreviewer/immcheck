package immcheck

import (
	"hash/crc32"

	"github.com/cespare/xxhash/v2"
)

//go:nosplit
func hashSum(valueBytes []byte) uint32 {
	valueSize := len(valueBytes)
	if valueSize >= 16 && valueSize <= 256 {
		// on arm64 CPUs crc32 measured to be more effective in this size range
		return crc32.ChecksumIEEE(valueBytes)
	}
	return uint32(xxhash.Sum64(valueBytes))
}

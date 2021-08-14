package immcheck_test

import (
	"fmt"
	"github.com/goodbadreviewer/immcheck"
	"math/rand"
	"testing"
)

var settings = []immcheck.ImutabilityCheckOptions{
	{
		SkipOriginCapturing:         false,
		SkipStringSnapshotCapturing: false,
		AllowInherintlyUnsafeTypes:  false,
	},
	{
		SkipOriginCapturing:         true,
		SkipStringSnapshotCapturing: false,
		AllowInherintlyUnsafeTypes:  false,
	},
	{
		SkipOriginCapturing:         false,
		SkipStringSnapshotCapturing: true,
		AllowInherintlyUnsafeTypes:  false,
	},
	{
		SkipOriginCapturing:         true,
		SkipStringSnapshotCapturing: true,
		AllowInherintlyUnsafeTypes:  false,
	},
}

var sizeOfSlice = []int{
	8, 16, 32, 64, 4 * 1024, 16 * 1024,
}

var percentOfMutations = []int{
	0, // 1, 99,
}

var count = 0

func BenchmarkImmcheckBytes(b *testing.B) {
	for _, options := range settings {
		for _, targetSize := range sizeOfSlice {
			for _, mutationPercent := range percentOfMutations {
				benchName := fmt.Sprintf("[%v]byte;muts(%v%%)", targetSize, mutationPercent)
				if options.SkipStringSnapshotCapturing {
					benchName += ";NoSnap"
				}
				if options.SkipOriginCapturing {
					benchName += ";NoOrig"
				}
				b.Run(benchName, func(b *testing.B) {
					localRand := rand.New(rand.NewSource(rand.Int63()))
					count = 0

					targetObjects := make([][]byte, b.N)
					for i := 0; i < b.N; i++ {
						targetObjects[i] = make([]byte, targetSize)
						localRand.Read(targetObjects[i])
					}

					b.ResetTimer()
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						snapshot := immcheck.NewValueSnapshotWithOptions(&targetObjects[i], options)
						rndValue := rand.Intn(100)
						if rndValue < mutationPercent {
							targetObjects[i][0] = byte(rndValue)
						}
						otherSnapshot := immcheck.NewValueSnapshotWithOptions(&targetObjects[i], options)
						err := snapshot.CheckImmutabilityAgainst(otherSnapshot)
						if err != nil {
							count += 1
						}
					}
					b.ReportMetric(float64(count), "muts")
				})
			}
		}
	}
}

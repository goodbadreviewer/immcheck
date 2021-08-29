//go:build !race && !immcheck
// +build !race,!immcheck

package immcheck

// ImmcheckRaceEnabled can be used in test to verify if mutability should be detected or not.
const ImmcheckRaceEnabled = false

func noop() {}

// RaceEnsureImmutability same as immcheck.EnsureImmutability
// but works only under `race` or `immcheck` build flags.
func RaceEnsureImmutability(v interface{}) func() {
	return noop
}

// RaceEnsureImmutabilityWithOptions same as immcheck.EnsureImmutabilityWithOptions
// but works only under `race` or `immcheck` build flags.
func RaceEnsureImmutabilityWithOptions(v interface{}, options Options) func() {
	return noop
}

// RaceCheckImmutabilityOnFinalization same as immcheck.CheckImmutabilityOnFinalization
// but works only under `race` or `immcheck` build flags.
func RaceCheckImmutabilityOnFinalization(v interface{}) {
}

// RaceCheckImmutabilityOnFinalizationWithOptions same as immcheck.CheckImmutabilityOnFinalizationWithOptions
// but works only under `race` or `immcheck` build flags.
func RaceCheckImmutabilityOnFinalizationWithOptions(v interface{}, options Options) {
}

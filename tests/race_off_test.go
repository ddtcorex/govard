//go:build !race

package tests

// raceDetectorEnabled reports whether the binary was built with the race
// detector. See race_on_test.go for why this is a build tag.
const raceDetectorEnabled = false

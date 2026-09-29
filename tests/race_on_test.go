//go:build race

package tests

// raceDetectorEnabled reports whether the binary was built with the race
// detector. It is a build tag rather than a runtime check because Go exposes no
// runtime API for it -- the `race` tag is set automatically by `-race`.
const raceDetectorEnabled = true

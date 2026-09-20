//go:build race

package calc

// raceEnabled reports that the package was built with the race detector, which slows
// evaluation by an order of magnitude and makes timing guards meaningless.
const raceEnabled = true

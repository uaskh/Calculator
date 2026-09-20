//go:build !race

package calc

// raceEnabled reports that the package was built without the race detector, so timing
// guards measure the production code path.
const raceEnabled = false

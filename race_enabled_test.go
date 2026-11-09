//go:build race

package openai_test

// raceEnabled: the race detector drops pooled buffers at random, so
// allocation bounds do not hold.
const raceEnabled = true

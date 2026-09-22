//go:build race

package main

// raceEnabled skips the two exhaustive single-goroutine enumerations under the
// race detector, where they would run for tens of minutes without exercising any
// concurrency; they run in the ordinary focused pass.
const raceEnabled = true

//go:build !unix

package ui

// cellAspect is the usual shape of a cell, where there's no asking the
// terminal — which is also where there are no pictures to fit.
func cellAspect() float64 { return 2 }

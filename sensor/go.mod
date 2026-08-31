module github.com/andreacoppini/crossbreeder/sensor

go 1.25.0

// The engine is not published as its own module; it lives beside this one in
// the same repository and is built from source.
replace github.com/andreacoppini/crossbreeder/engine => ../engine

require (
	golang.org/x/net v0.58.0
	golang.org/x/sys v0.47.0
)

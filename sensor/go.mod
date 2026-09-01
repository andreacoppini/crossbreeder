module github.com/andreacoppini/crossbreeder/sensor

go 1.25.0

// The engine is not published as its own module; it lives beside this one in
// the same repository and is built from source.
replace github.com/andreacoppini/crossbreeder/engine => ../engine

require (
	golang.org/x/net v0.58.0
	golang.org/x/sys v0.47.0
)

require (
	github.com/andreacoppini/crossbreeder/engine v0.0.0-20260828135009-d66e0d8bb446 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/term v0.45.0 // indirect
)

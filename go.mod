module github.com/skillian/expr

go 1.23.0

toolchain go1.24.2

replace github.com/skillian/errors => ../errors

replace github.com/skillian/ctxutil => ../ctxutil

replace github.com/skillian/logging => ../logging

replace github.com/skillian/unsafereflect => ../unsafereflect

replace github.com/skillian/unsafereflecttest => ../unsafereflecttest

require (
	github.com/skillian/ctxutil v0.0.0-20250730192020-6306317f0ca2
	github.com/skillian/errors v0.0.0-20250827231644-5bd01b1010e4
	github.com/skillian/logging v0.0.0-20250730194804-db602bc2b029
	github.com/skillian/unsafereflect v0.0.0-20250902232225-c9ebf4419ac6
)

require golang.org/x/sys v0.34.0 // indirect

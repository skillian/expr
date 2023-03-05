module github.com/skillian/expr

go 1.18

replace github.com/skillian/errors => ../errors
replace github.com/skillian/errutil => ../errutil

replace github.com/skillian/ctxutil => ../ctxutil

replace github.com/skillian/logging => ../logging

require (
	github.com/skillian/ctxutil v0.0.0-00010101000000-000000000000
	github.com/skillian/errors v0.0.0-20220412220440-9e3e39f14923
	github.com/skillian/errutil v0.0.0-00010101000000-000000000000
)

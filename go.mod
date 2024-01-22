module github.com/skillian/expr

go 1.17

replace github.com/skillian/errors => ../errors

replace github.com/skillian/ctxutil => ../ctxutil

replace github.com/skillian/logging => ../logging

require (
	github.com/skillian/ctxutil v0.0.0-00010101000000-000000000000
	github.com/skillian/errors v0.0.0-20220412220440-9e3e39f14923
	github.com/skillian/logging v0.0.0-20220617155357-42fdd303775d
)

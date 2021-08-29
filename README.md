# immcheck - Runtime immutability checks library 
[![Go Reference](https://pkg.go.dev/badge/github.com/goodbadreviewer/immcheck.svg)](https://pkg.go.dev/github.com/goodbadreviewer/immcheck) ![Build](https://github.com/goodbadreviewer/immcheck/actions/workflows/build.yml/badge.svg) ![Linters](https://github.com/goodbadreviewer/immcheck/actions/workflows/golangci-lint.yml/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/goodbadreviewer/immcheck)](https://goreportcard.com/report/github.com/goodbadreviewer/immcheck) [![Coverage Status](https://coveralls.io/repos/github/goodbadreviewer/immcheck/badge.svg)](https://coveralls.io/github/goodbadreviewer/immcheck) 

Example: 

```go
m := map[string]string{
    "k1": "v1",
}
m["k2"] = "v2"
// returns function that you can call to verify that m didn't change
checkFunction := immcheck.EnsureImmutability(&m)
checkFunction() // no mutations are fine

func() {
    defer func() {
        mutationPanic := recover()
        fmt.Println(mutationPanic)
    }()

    // you can also use defer for such checks
    // now when we mutate m, we will get panic at the end of the function
    defer immcheck.EnsureImmutability(&m)()

    // it is also possible to set a finalizer that can check
    // if object remained immutable from this point till garbage collection
    immcheck.CheckImmutabilityOnFinalization(&m)

    delete(m, "k1")
}()
```


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
    
    // this function works only with `-race` or `-tags immcheck` build flags
    defer immcheck.RaceEnsureImmutability(&m)()

    // this function works only with `-race` or `-tags immcheck` build flags as well
    immcheck.RaceCheckImmutabilityOnFinalization(&m)

    delete(m, "k1")
}()
```

### Brief description of how it works internally and how it affects the performance of your program

The library uses reflection to walk the tree of all reachable pointers, starting from the pointer you provided, and stores checksums of every encountered value into a `map[uint32]uint32`. When it is time to check immutability, it walks the same structure, collects the same map of checksums, and verifies that new map is equal to the previous one. From the performance standpoint of view, the library does a lot of tricks and optimizations to make the overhead as low as possible.
For example:
 - it uses memory pooling for these maps of checksums and some internal buffers
 - it avoids allocations everywhere where possible, though some reflection API calls require allocations (with 1.18 we will be able to get rid of those that remain right now)
 - it treats slices of pointerless structures as just one contiguous value, so it hashes such slices efficiently and uses only one item in the checksums map to store its hash

In general, performance overhead will depend on what kind of structures you're declaring as immutable and how deeply nested they are. For most applications, the overhead should be non-noticeable or at least bearable. If performance is a concern though: you can use `RaceEnsureImmutability` methods that will have 0 overhead in normal builds and will perform checks only when race detector is enabled or if you build your program with `-tags immcheck` build flag

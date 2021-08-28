# immcheck - Runtime immutability checks library

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

    delete(m, "k1")
}()
```


---
---

Implement `data.Generator`:

```go
type Generator interface {
    Format() (uint, string)
    Copy() Generator
    Values(RunCount) []interface{}
    Scan(any interface{}) error
    Name() string
}
```

Your generator does _not_ have to handle data scope.
When it's called, Finch expects new values.

Implement `data.Factory` to create your data generator:

```go
type Factory interface {
    Make(name, dataKey string, params map[string]string) (Generator, error)
}
```

Register your data generator and its factory by calling `data.Register(name string, f Factory) error`.

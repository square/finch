---
---

```go
type Reporter interface {
    Report(from []Instance)
    Stop()
}

type ReporterFactory interface {
    Make(name string, opts map[string]string) (Reporter, error)
}
```

Implement and register by calling `stats.Register(name string, f ReporterFactory) error`.

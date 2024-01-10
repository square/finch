package mock

import (
	"fmt"

	"github.com/square/finch/data"
)

type DataGeneratorFactory struct {
	MakeFunc func(name, dataKey string, params map[string]string) (data.Generator, error)
}

func (f DataGeneratorFactory) Make(name, dataKey string, params map[string]string) (data.Generator, error) {
	if f.MakeFunc != nil {
		return f.MakeFunc(name, dataKey, params)
	}
	return nil, fmt.Errorf("MakeFunc not set in mock.DataGeneratorFactory")
}

var _ data.Factory = DataGeneratorFactory{}

type DataGenerator struct {
	FormatFunc func() (uint, string)
	CopyFunc   func() data.Generator
	ValuesFunc func(data.RunCount) []interface{}
	ScanFunc   func(any interface{}) error
	NameFunc   func() string
}

var _ data.Generator = DataGenerator{}

func (g DataGenerator) Format() (uint, string) {
	if g.FormatFunc != nil {
		return g.FormatFunc()
	}
	return 0, ""
}

func (g DataGenerator) Copy() data.Generator {
	if g.CopyFunc != nil {
		return g.CopyFunc()
	}
	return g
}

func (g DataGenerator) Values(rc data.RunCount) []interface{} {
	if g.ValuesFunc != nil {
		return g.ValuesFunc(rc)
	}
	return nil
}

func (g DataGenerator) Scan(any interface{}) error {
	if g.ScanFunc != nil {
		return g.ScanFunc(any)
	}
	return nil
}

func (g DataGenerator) Name() string {
	if g.NameFunc != nil {
		return g.NameFunc()
	}
	return "mock"
}

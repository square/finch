// Copyright 2022 Block, Inc.

package data

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"github.com/square/finch"
)

type IntNotNull struct {
	Max int
}

var _ Generator = IntNotNull{}

func (g IntNotNull) Id() Id {
	return Id{Type: "int-not-null"}
}

func (g IntNotNull) Scope() string {
	return finch.SCOPE_STATEMENT
}

func (g IntNotNull) Format() string {
	return "%d"
}

func (g IntNotNull) Copy(clientNo int) Generator {
	return g
}

func (g IntNotNull) Values() []interface{} {
	return []interface{}{rand.Intn(g.Max)}
}

func (g IntNotNull) Scan(any interface{}) error {
	return nil
}

// --------------------------------------------------------------------------

type IntRange struct {
	id     Id
	size   int64
	min    int64
	max    int64
	v      []int64
	params map[string]string
}

var _ Generator = &IntRange{}

func NewIntRange(id Id, params map[string]string) (*IntRange, error) {
	g := &IntRange{
		id:     id,
		size:   100,   // 100 values between
		min:    1,     // 1 and
		max:    10000, // 100,000 by default
		v:      []int64{0, 0},
		params: params,
	}
	if err := int64From(params, "size", &g.size, false); err != nil {
		return nil, err
	}
	if err := int64From(params, "min", &g.min, false); err != nil {
		return nil, err
	}
	if err := int64From(params, "max", &g.max, false); err != nil {
		return nil, err
	}
	if g.min >= g.max {
		return nil, fmt.Errorf("invalid int range: max not greater than min")
	}
	if g.size > (g.max - g.min) {
		return nil, fmt.Errorf("invalid int range: size > max - min")
	}
	finch.Debug("%s: %d values between %d and %d", g.id, g.size, g.min, g.max)
	return g, nil
}

func (g *IntRange) Id() Id { return g.id }

func (g *IntRange) Scope() string { return finch.SCOPE_STATEMENT }

func (g *IntRange) Format() string { return "%d" }

func (g *IntRange) Copy(clientNo int) Generator {
	gCopy, _ := NewIntRange(g.id.Copy(clientNo), g.params)
	return gCopy
}

func (g *IntRange) Values() []interface{} {
	// MySQL BETWEEN is closed interval [min, max], so if random min (lower)
	// is 10 and size is 3, then 10+3=13 but that's 4 values: 10, 11, 12, 13.
	// So we -1 to make BETWEEEN 10 AND 12, which is 3 values.
	lower := g.min + rand.Int63n(g.max-g.min)
	upper := lower + g.size - 1
	if upper > g.max {
		upper = g.max
	}
	return []interface{}{lower, upper}
}

func (g *IntRange) Scan(any interface{}) error {
	return nil
}

// --------------------------------------------------------------------------

type ProjectInt struct {
	id           Id
	input_range  int
	output_start float64
	slope        float64
}

func NewProjectInt(id Id, params map[string]string) (ProjectInt, error) {
	// https://stackoverflow.com/questions/5731863/mapping-a-numeric-range-onto-another

	inputMax, ok := params["input-max"]
	if !ok {
		return ProjectInt{}, fmt.Errorf("input-max required")
	}
	input_end, err := strconv.ParseInt(inputMax, 10, 32)
	if err != nil {
		return ProjectInt{}, fmt.Errorf("invalid input-max: %s: %s", inputMax, err)
	}

	outputRange, ok := params["output-range"]
	if !ok {
		return ProjectInt{}, fmt.Errorf("output-range required")
	}
	p := strings.Split(outputRange, "..")
	if len(p) != 2 {
		return ProjectInt{}, fmt.Errorf("invalid output-range: %s: got %d values, expected 2; format: min..max", outputRange, len(p))
	}

	output_start, err := strconv.ParseFloat(p[0], 64)
	if err != nil {
		return ProjectInt{}, fmt.Errorf("invalid output-max: %s: %s", p[0], err)
	}
	output_end, err := strconv.ParseFloat(p[1], 64)
	if err != nil {
		return ProjectInt{}, fmt.Errorf("invalid output-max: %s: %s", p[1], err)
	}
	finch.Debug("%s: 1..%d -> %d..%d", id, input_end, int(output_start), int(output_end))

	g := ProjectInt{
		id:           id,
		input_range:  int(input_end) - 1,
		output_start: output_start,
		slope:        (output_end - output_start) / float64(input_end-1),
	}
	return g, nil
}

var _ Generator = ProjectInt{}

func (g ProjectInt) Id() Id { return g.id }

func (g ProjectInt) Scope() string { return finch.SCOPE_STATEMENT }

func (g ProjectInt) Format() string { return "%d" }

func (g ProjectInt) Copy(clientNo int) Generator { return g }

func (g ProjectInt) Scan(any interface{}) error { return nil }

func (g ProjectInt) Values() []interface{} {
	input := rand.Intn(g.input_range)
	return []interface{}{int(g.output_start + math.Floor((g.slope*float64(input))+0.5))}
}

// --------------------------------------------------------------------------

type IntSequence struct {
	id     Id
	min    int64
	max    int64
	size   int64
	n      int64
	params map[string]string
	*sync.Mutex
}

var _ Generator = &IntSequence{}

func NewIntSequence(id Id, params map[string]string) (*IntSequence, error) {
	g := &IntSequence{
		id:     id,
		min:    1,
		max:    10000, // 100,000 by default
		size:   10,
		n:      1,
		params: params,
		Mutex:  &sync.Mutex{},
	}
	if err := int64From(params, "size", &g.size, false); err != nil {
		return nil, err
	}
	if err := int64From(params, "min", &g.min, false); err != nil {
		return nil, err
	}
	g.n = g.min
	if err := int64From(params, "max", &g.max, false); err != nil {
		return nil, err
	}
	if g.min >= g.max {
		return nil, fmt.Errorf("invalid int sequence: max not greater than min")
	}
	if g.size > (g.max - g.min) {
		return nil, fmt.Errorf("invalid int sequence: size > max - min")
	}
	finch.Debug("%s: %d values from %d to %d", g.id, g.size, g.min, g.max)
	return g, nil
}

func (g *IntSequence) Id() Id { return g.id }

func (g *IntSequence) Scope() string { return finch.SCOPE_STATEMENT }

func (g *IntSequence) Format() string { return "%d" }

func (g *IntSequence) Copy(clientNo int) Generator {
	gCopy, _ := NewIntSequence(g.id.Copy(clientNo), g.params)
	return gCopy
}

func (g *IntSequence) Values() []interface{} {
	g.Lock()
	lower := g.n
	upper := g.n + g.size
	if upper <= g.max {
		g.n += g.size // next chunk
	} else {
		upper = g.max
		finch.Debug("%s: reached %d, resetting to %d", g.id, g.max, g.min)
		g.n = g.min // reset
	}
	g.Unlock()
	return []interface{}{lower, upper}
}

func (g *IntSequence) Scan(any interface{}) error {
	return nil
}

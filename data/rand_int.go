// Copyright 2022 Block, Inc.

package data

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"

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
	id  Id
	r   int64
	max int64
	v   []int64
}

var _ Generator = &IntRange{}

func NewIntRange(id Id, max, r int64) *IntRange {
	return &IntRange{
		id:  id,
		r:   r,
		max: max,
		v:   []int64{0, 0},
	}
}

func (g *IntRange) Id() Id {
	return g.id
}

func (g *IntRange) Scope() string {
	return finch.SCOPE_STATEMENT
}

func (g *IntRange) Format() string {
	return "%d"
}

func (g *IntRange) Copy(clientNo int) Generator {
	return NewIntRange(g.id.Copy(clientNo), g.max, g.r)
}

func (g *IntRange) Values() []interface{} {
	lower := rand.Int63n(g.max)
	upper := lower + g.r - 1
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

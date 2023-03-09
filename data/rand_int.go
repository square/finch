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

type RandInt struct {
	id     Id
	params map[string]string
	min    int64
	max    int64
	dist   byte    // normal|uniform
	mean   float64 // dist=normal
	stddev float64 // dist=normal
}

var _ Generator = &RandInt{}

const (
	dist_uniform byte = iota
	dist_normal
)

func NewRandInt(id Id, params map[string]string) (*RandInt, error) {
	g := &RandInt{
		id:     id,
		min:    1,
		max:    1000000,
		dist:   dist_uniform,
		params: params,
	}

	if err := int64From(params, "min", &g.min, false); err != nil {
		return nil, err
	}
	if err := int64From(params, "max", &g.max, false); err != nil {
		return nil, err
	}

	switch strings.ToLower(params["dist"]) {
	case "normal":
		g.dist = dist_normal
		var mean int64
		if err := int64From(params, "mean", &mean, false); err != nil {
			return nil, err
		}
		if mean == 0 {
			mean = (g.max - g.min + 1) / 2
		}
		g.mean = float64(mean)

		s, ok := params["stddev"]
		if ok {
			var err error
			g.stddev, err = strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, err
			}
		} else {
			g.stddev = (float64(g.max) - float64(g.min)) / 8.0
		}
	case "uniform":
		g.dist = dist_uniform
	default:
		g.dist = dist_uniform
	}

	return g, nil
}

func (g *RandInt) Id() Id                     { return g.id }
func (g *RandInt) Format() string             { return "%d" }
func (g *RandInt) Scan(any interface{}) error { return nil }

func (g *RandInt) Copy(r finch.RunLevel) Generator {
	c, _ := NewRandInt(g.id.Copy(r), g.params)
	return c
}

func (g *RandInt) Values(_ finch.ExecCount) []interface{} {
	switch g.dist {
	case dist_normal:
		v := int64(math.Floor(rand.NormFloat64()*g.stddev + g.mean))
		if v < g.min || v > g.max {
			v = int64(math.Floor(rand.NormFloat64()*g.stddev + g.mean))
			if v < g.min || v > g.max {
				return []interface{}{int64(g.mean)}
			}
		}
		return []interface{}{v}
	default: // uniform
		return []interface{}{rand.Int63n(g.max)}
	}
}

// --------------------------------------------------------------------------

type IntRange struct {
	id     Id
	params map[string]string
	size   int64
	min    int64
	max    int64
	v      []int64
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
	return g, nil
}

func (g *IntRange) Id() Id                     { return g.id }
func (g *IntRange) Format() string             { return "%d" }
func (g *IntRange) Scan(any interface{}) error { return nil }

func (g *IntRange) Copy(r finch.RunLevel) Generator {
	gCopy, _ := NewIntRange(g.id.Copy(r), g.params)
	return gCopy
}

func (g *IntRange) Values(_ finch.ExecCount) []interface{} {
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

// --------------------------------------------------------------------------

type ProjectInt struct {
	id           Id
	params       map[string]string
	input_range  int
	output_start float64
	slope        float64
}

var _ Generator = ProjectInt{}

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
		params:       params,
		input_range:  int(input_end) - 1,
		output_start: output_start,
		slope:        (output_end - output_start) / float64(input_end-1),
	}
	return g, nil
}

func (g ProjectInt) Id() Id                     { return g.id }
func (g ProjectInt) Format() string             { return "%d" }
func (g ProjectInt) Scan(any interface{}) error { return nil }

func (g ProjectInt) Copy(r finch.RunLevel) Generator {
	c, _ := NewProjectInt(g.id.Copy(r), g.params)
	return c
}

func (g ProjectInt) Values(_ finch.ExecCount) []interface{} {
	input := rand.Intn(g.input_range)
	return []interface{}{int(g.output_start + math.Floor((g.slope*float64(input))+0.5))}
}

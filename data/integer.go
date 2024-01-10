// Copyright 2024 Block, Inc.

package data

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/square/finch"
)

// Int implements the int data generator.
type Int struct {
	min    int64
	max    int64
	dist   byte    // normal|uniform
	mean   float64 // dist=normal
	stddev float64 // dist=normal
}

var _ Generator = &Int{}

const (
	dist_uniform byte = iota
	dist_normal
)

func NewInt(params map[string]string) (*Int, error) {
	g := &Int{
		min:  1,
		max:  finch.ROWS,
		dist: dist_uniform,
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
	finch.Debug("rand int [%d, %d] dist %d (uni %d, norm %d)", g.min, g.max, g.dist, dist_uniform, dist_normal)
	return g, nil
}

func (g *Int) Name() string               { return "int" }
func (g *Int) Format() (uint, string)     { return 1, "%d" }
func (g *Int) Scan(any interface{}) error { return nil }

func (g *Int) Copy() Generator {
	c := *g
	return &c
}

func (g *Int) Values(_ RunCount) []interface{} {
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
		v := rand.Int63n(g.max)
		if v < g.min {
			v = g.min
		}
		return []interface{}{v}
	}
}

// --------------------------------------------------------------------------

// IntGaps implements the int-gaps data generator.
type IntGaps struct {
	params       map[string]string
	input_max    int64
	output_start float64
	slope        float64
}

var _ Generator = &IntGaps{}

func NewIntGaps(params map[string]string) (*IntGaps, error) {
	// https://stackoverflow.com/questions/5731863/mapping-a-numeric-range-onto-another
	min := int64(1)
	if err := int64From(params, "min", &min, false); err != nil {
		return nil, err
	}
	max := int64(finch.ROWS)
	if err := int64From(params, "max", &max, false); err != nil {
		return nil, err
	}
	size := max - min + 1
	if size <= 0 {
		return nil, fmt.Errorf("invalid int-gaps: max - min must be > 0")
	}

	p := int64(20)
	if err := int64From(params, "p", &p, false); err != nil {
		return nil, err
	}
	if p < 1 || p > 100 {
		return nil, fmt.Errorf("invalid int-gaps p: %d, must be between 1 to 100 (inclusive)", p)
	}
	input_max := int64(float64(size) * (float64(p) / 100.0))

	g := &IntGaps{
		params:       params,
		input_max:    input_max,
		output_start: float64(min),
		slope:        float64(max-min) / float64(input_max-1),
	}
	finch.Debug("1..%d -> %d..%d (%d%% of %d) gap: %d records", input_max, min, max, p, size, int(g.slope))
	return g, nil
}

func (g *IntGaps) Name() string               { return "int-gaps" }
func (g *IntGaps) Format() (uint, string)     { return 1, "%d" }
func (g *IntGaps) Scan(any interface{}) error { return nil }

func (g *IntGaps) Copy() Generator {
	c, _ := NewIntGaps(g.params)
	return c
}

func (g *IntGaps) Values(_ RunCount) []interface{} {
	return []interface{}{int64(g.output_start + float64(rand.Int63n(g.input_max))*g.slope)}
}

// --------------------------------------------------------------------------

// IntRange implements the int-range data generator.
type IntRange struct {
	params map[string]string
	size   int64
	min    int64
	max    int64
	v      []int64
}

var _ Generator = &IntRange{}

func NewIntRange(params map[string]string) (*IntRange, error) {
	g := &IntRange{
		min:    1,
		max:    finch.ROWS,
		size:   100,
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
		return nil, fmt.Errorf("invalid int range: min %d >= max %d", g.min, g.max)
	}
	if g.size > (g.max - g.min) {
		return nil, fmt.Errorf("invalid int range: size %d > (max %d - min %d)", g.size, g.max, g.min)
	}
	return g, nil
}

func (g *IntRange) Name() string               { return "int-range" }
func (g *IntRange) Format() (uint, string)     { return 2, "%d" }
func (g *IntRange) Scan(any interface{}) error { return nil }

func (g *IntRange) Copy() Generator {
	gCopy, _ := NewIntRange(g.params)
	return gCopy
}

func (g *IntRange) Values(_ RunCount) []interface{} {
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

// IntRangeSeq implements the int-range-seq data generator.
type IntRangeSeq struct {
	begin  int64
	end    int64
	size   int64
	n      int64
	params map[string]string
	*sync.Mutex
}

var _ Generator = &IntRangeSeq{}

func NewIntRangeSeq(params map[string]string) (*IntRangeSeq, error) {
	g := &IntRangeSeq{
		begin:  1,
		end:    finch.ROWS,
		size:   100,
		n:      1,
		params: params,
		Mutex:  &sync.Mutex{},
	}
	if err := int64From(params, "size", &g.size, false); err != nil {
		return nil, err
	}
	if err := int64From(params, "begin", &g.begin, false); err != nil {
		return nil, err
	}
	g.n = g.begin
	if err := int64From(params, "end", &g.end, false); err != nil {
		return nil, err
	}
	if g.begin > g.end {
		return nil, fmt.Errorf("invalid int-range-seq: begin (%d) > end (%d)", g.begin, g.end)
	}
	if g.size > (g.end - g.begin) {
		return nil, fmt.Errorf("invalid int-range-seq: size (%d) > end (%d) - begin (%d)", g.size, g.end, g.begin)
	}
	return g, nil
}

func (g *IntRangeSeq) Name() string               { return "int-range-seq" }
func (g *IntRangeSeq) Format() (uint, string)     { return 2, "%d" }
func (g *IntRangeSeq) Scan(any interface{}) error { return nil }

func (g *IntRangeSeq) Copy() Generator {
	c, _ := NewIntRangeSeq(g.params)
	return c
}

func (g *IntRangeSeq) Values(_ RunCount) []interface{} {
	g.Lock()
	if g.n > g.end {
		g.n = g.begin // reset  [begin, m]
	}
	n, m := g.n, g.n+g.size-1 // next chunk [n, m]
	g.n += g.size
	if m > g.end {
		m = g.end // short chunk [n, end]
	}
	g.Unlock()
	return []interface{}{n, m}
}

// --------------------------------------------------------------------------

// AutoInc implements the auto-inc data generator.
type AutoInc struct {
	i    uint64
	step uint64
}

var _ Generator = &AutoInc{}

func NewAutoInc(params map[string]string) (*AutoInc, error) {
	g := &AutoInc{
		i:    0,
		step: 1,
	}
	s, ok := params["start"]
	if ok {
		i, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid start=%s: %s", s, err)
		}
		g.i = i
	}
	s, ok = params["step"]
	if ok {
		i, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid step=%s: %s", s, err)
		}
		g.step = i
	}
	return g, nil
}

func (g *AutoInc) Name() string               { return "auto-inc" }
func (g *AutoInc) Format() (uint, string)     { return 1, "%d" }
func (g *AutoInc) Scan(any interface{}) error { return nil }

func (g *AutoInc) Copy() Generator {
	return &AutoInc{
		i:    g.i,
		step: g.step,
	}
}

func (g *AutoInc) Values(_ RunCount) []interface{} {
	return []interface{}{atomic.AddUint64(&g.i, g.step)}
}

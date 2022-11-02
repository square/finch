// Copyright 2022 Block, Inc.

package data

import (
	"bytes"
	"database/sql"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/square/finch"
)

func init() {
	Register("int-not-null", f)
	Register("int-range", f)
	Register("str-not-null", f)
	Register("column", f)
	Register("xid", f)
}

type Generator interface {
	Format() string
	Copy(clientNo int) Generator
	Values() []interface{}
	Scan(any interface{}) error
	Id() Id
	Scope() string
}

type Id struct {
	DataName string // @col
	Type     string // str-not-null
	ClientNo int    // 1
	CopyNo   int    // 1
	copied   int
}

func (id Id) String() string {
	return fmt.Sprintf("%s/%s/%d/%d", id.DataName, id.Type, id.ClientNo, id.CopyNo)
}

func (id *Id) Copy(clientNo int) Id {
	id.copied++
	cp := *id
	cp.ClientNo = clientNo
	cp.CopyNo = id.copied
	return cp
}

type Factory interface {
	Make(name, dataName string, params map[string]string) (Generator, error)
}

type factory struct{}

var f = &factory{}

func (f factory) Make(name, dataName string, params map[string]string) (Generator, error) {
	id := Id{
		Type:     name,
		DataName: dataName,
	}
	switch name {
	case "int-not-null":
		max := 2147483647
		if s, ok := params["max"]; ok {
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, err
			}
			max = n
		}
		return IntNotNull{Max: max}, nil
	case "int-range":
		max := int64(2147483647)
		r := int64(100)
		if s, ok := params["max"]; ok {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, err
			}
			max = n
		}
		if s, ok := params["range"]; ok {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, err
			}
			r = n
		}
		return NewIntRange(id, max, r), nil
	case "str-not-null":
		s, ok := params["len"]
		if !ok {
			return nil, fmt.Errorf("str-not-null requires param len")
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		return NewStrNotNull(id, n), nil
	case "column":
		return NewColumn(id, params["col"], params["type"]), nil
	case "xid":
		n := 1
		s, ok := params["rows"]
		if ok {
			var err error
			n, err = strconv.Atoi(s)
			if err != nil {
				return nil, err
			}
		}
		return NewXid(id, n), nil
	}
	return nil, fmt.Errorf("built-in data factory cannot make %s data generator", name)
}

var r = &repo{
	f: map[string]Factory{},
}

type repo struct {
	f map[string]Factory
}

func Register(name string, f Factory) error {
	_, have := r.f[name]
	if have {
		return fmt.Errorf("data.Generator %s already registered", name)
	}
	r.f[name] = f
	return nil
}

// Make makes a data generator by name with the given generator-specific params.
func Make(name, dataName string, params map[string]string) (Generator, error) {
	f, have := r.f[name]
	if !have {
		return nil, fmt.Errorf("data.Generator %s not registered", name)
	}
	return f.Make(name, dataName, params)
}

// --------------------------------------------------------------------------

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

// https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

type StrNotNull struct {
	id  Id
	n   int
	src rand.Source
}

func NewStrNotNull(id Id, n int) *StrNotNull {
	return &StrNotNull{
		id:  id,
		n:   n,
		src: rand.NewSource(time.Now().UnixNano()),
	}
}

var _ Generator = &StrNotNull{}

func (g *StrNotNull) Id() Id {
	return g.id
}

func (g *StrNotNull) Scope() string {
	return finch.SCOPE_STATEMENT
}

func (g *StrNotNull) Format() string {
	return "'%s'"
}

func (g *StrNotNull) Copy(clientNo int) Generator {
	return NewStrNotNull(g.id.Copy(clientNo), g.n)
}

func (g *StrNotNull) Values() []interface{} {
	sb := strings.Builder{}
	sb.Grow(g.n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := g.n-1, g.src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = g.src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	return []interface{}{sb.String()}
}

func (g StrNotNull) Scan(any interface{}) error {
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

type Column struct {
	id      Id
	col     string
	colType string
	r       bool
	v       interface{}
	b       *bytes.Buffer
}

var _ Generator = &Column{}
var _ sql.Scanner = &Column{}

func NewColumn(id Id, col, colType string) *Column {
	return &Column{
		id:      id,
		col:     col,
		colType: colType,
	}
}

func (g *Column) Id() Id {
	return g.id
}

func (g *Column) Scope() string {
	return finch.SCOPE_TRANSACTION
}

func (g *Column) Format() string {
	if g.colType == "n" {
		return "%v"
	}
	return "'%v'"
}

func (g *Column) Copy(clientNo int) Generator {
	return NewColumn(g.id.Copy(clientNo), g.col, g.colType)
}

func (g *Column) Values() []interface{} {
	if g.r {
		return []interface{}{g.b.String()}
	}
	return []interface{}{g.v}
}

func (g *Column) Scan(any interface{}) error {
	switch any.(type) {
	case []byte:
		g.r = true // is reference; copy bytes
		if g.b == nil {
			g.b = bytes.NewBuffer(make([]byte, len(any.([]byte))))
		}
		g.b.Reset()
		g.b.Write(any.([]byte))
	default:
		g.r = false // not reference; copy value
		g.v = any
	}
	return nil
}

// --------------------------------------------------------------------------

var Noop = noop{}

type noop struct{}

var _ Generator = noop{}
var _ sql.Scanner = noop{}

func (g noop) Id() Id {
	return Id{}
}

func (g noop) Scope() string {
	return finch.SCOPE_GLOBAL
}

func (g noop) Format() string {
	return ""
}

func (g noop) Copy(clientNo int) Generator {
	return g
}

func (g noop) Values() []interface{} {
	return nil
}

func (g noop) Scan(any interface{}) error {
	return nil
}

// --------------------------------------------------------------------------

type Xid struct {
	rows int
	vals []string
	i    int
	id   Id
}

var _ Generator = &Xid{}

func NewXid(id Id, rows int) *Xid {
	if rows <= 1 {
		return &Xid{id: id, rows: 1}
	}
	return &Xid{
		rows: rows,
		i:    rows,
		vals: make([]string, rows),
		id:   id,
	}
}

func (g *Xid) Id() Id {
	return g.id
}

func (g *Xid) Scope() string {
	if g.rows == 1 {
		return finch.SCOPE_GLOBAL
	}
	return finch.SCOPE_TRANSACTION
}

func (g *Xid) Format() string {
	return "'%s'"
}

func (g *Xid) Copy(clientNo int) Generator {
	return NewXid(g.id.Copy(clientNo), g.rows)
}

func (g *Xid) Values() []interface{} {
	if g.rows == 1 {
		return []interface{}{xid.New().String()}
	}
	if g.i == g.rows {
		s := xid.New().String()
		for i := 0; i < g.rows; i++ {
			g.vals[i] = s
		}
		g.i = 1
	} else {
		g.i++
	}
	return []interface{}{g.vals[g.i-1]}
}

func (g *Xid) Scan(any interface{}) error {
	return nil
}

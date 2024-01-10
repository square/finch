// Copyright 2024 Block, Inc.

package data

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// StrFillAz implemnts the str-fill-az data generator.
type StrFillAz struct {
	len int64
	src rand.Source
}

var _ Generator = &StrFillAz{}

// https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func NewStrFillAz(params map[string]string) (*StrFillAz, error) {
	g := &StrFillAz{
		len: 100,
		src: rand.NewSource(time.Now().UnixNano()),
	}
	if err := int64From(params, "len", &g.len, false); err != nil {
		return nil, err
	}
	if g.len <= 0 {
		return nil, fmt.Errorf("stra-az param len must be >= 1")
	}
	return g, nil
}

func (g *StrFillAz) Name() string               { return "str-fill-az" }
func (g *StrFillAz) Format() (uint, string)     { return 1, "'%s'" }
func (g *StrFillAz) Scan(any interface{}) error { return nil }

func (g *StrFillAz) Copy() Generator {
	return &StrFillAz{
		len: g.len,
		src: rand.NewSource(time.Now().UnixNano()),
	}
}

func (g *StrFillAz) Values(_ RunCount) []interface{} {
	sb := strings.Builder{}
	sb.Grow(int(g.len))
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := g.len-1, g.src.Int63(), letterIdxMax; i >= 0; {
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

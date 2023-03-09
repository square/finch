// Copyright 2022 Block, Inc.

package data

import (
	"math/rand"
	"strings"
	"time"

	"github.com/square/finch"
)

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

func (g *StrNotNull) Format() string {
	return "'%s'"
}

func (g *StrNotNull) Copy(r finch.RunLevel) Generator {
	return NewStrNotNull(g.id.Copy(r), g.n)
}

func (g *StrNotNull) Values(_ finch.ExecCount) []interface{} {
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

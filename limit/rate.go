// Copyright 2022 Block, Inc.

package limit

import (
	"context"

	gorate "golang.org/x/time/rate"

	"github.com/square/finch"
)

type Rate interface {
	Allow() <-chan bool
}

type rate struct {
	c  chan bool
	n  uint64
	rl *gorate.Limiter
}

var _ Rate = &rate{}

func NewRate(perSecond uint64) Rate {
	if perSecond == 0 {
		return nil
	}
	finch.Debug("new rate: %d/s", perSecond)
	lm := &rate{
		rl: gorate.NewLimiter(gorate.Limit(perSecond), 1),
		c:  make(chan bool, 1),
	}
	go lm.run()
	return lm
}

func (lm *rate) Allow() <-chan bool {
	return lm.c
}

func (lm *rate) run() {
	var err error
	for {
		err = lm.rl.Wait(context.Background())
		if err != nil {
			// burst limit exceeded?
			continue
		}
		select {
		case lm.c <- true:
		default:
			// dropped
		}
	}
}

// --------------------------------------------------------------------------

type and struct {
	c chan bool
	n uint64
	a Rate
	b Rate
}

var _ Rate = and{}

// And makes a Rate limiter that allows execution when both a and b allow it.
// This is used to combine QPS and TPS rate limits to keep clients at or below
// both rates.
func And(a, b Rate) Rate {
	if a == nil && b == nil {
		return nil
	}
	if a == nil && b != nil {
		return b
	}
	if a != nil && b == nil {
		return a
	}
	lm := and{
		a: a,
		b: b,
		c: make(chan bool, 1),
	}
	go lm.run()
	return lm
}

func (lm and) Allow() <-chan bool {
	return lm.c
}

func (lm and) N(_ uint64) {
}

func (lm and) run() {
	a := false
	b := false
	for {
		select {
		case <-lm.a.Allow():
			a = true
		case <-lm.b.Allow():
			b = true
		}
		if a && b {
			select {
			case lm.c <- true:
			default:
				// dropped
			}
			a = false
			b = false
		}
	}
}

// Copyright 2022 Block, Inc.

package limit

import (
	"context"
	"fmt"

	gorate "golang.org/x/time/rate"

	"github.com/square/finch"
)

type Rate interface {
	Adjust(byte)
	Current() (byte, string)
	Allow() <-chan bool
	Stop()
}

type rate struct {
	c        chan bool
	n        uint
	rl       *gorate.Limiter
	stopChan chan struct{}
}

var _ Rate = &rate{}

func NewRate(perSecond uint) Rate {
	if perSecond == 0 {
		return nil
	}
	finch.Debug("new rate: %d/s", perSecond)
	lm := &rate{
		rl:       gorate.NewLimiter(gorate.Limit(perSecond), 1),
		c:        make(chan bool, 1),
		stopChan: make(chan struct{}),
	}
	go lm.run()
	return lm
}

func (lm *rate) Adjust(p byte) {
}

func (lm *rate) Current() (p byte, s string) {
	return 0, ""
}

func (lm *rate) Stop() {
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
		case <-lm.stopChan:
			return
		default:
			// dropped
		}
	}
}

// --------------------------------------------------------------------------

type and struct {
	c chan bool
	n uint
	a Rate
	b Rate
}

var _ Rate = &and{}

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
	lm := &and{
		a: a,
		b: b,
		c: make(chan bool, 1),
	}
	go lm.run()
	return lm
}

func (lm *and) Allow() <-chan bool {
	return lm.c
}

func (lm *and) N(_ uint) {
}

func (lm *and) Adjust(p byte) {
	lm.a.Adjust(p)
	lm.b.Adjust(p)
}

func (lm *and) Current() (p byte, s string) {
	p1, s1 := lm.a.Current()
	p2, s2 := lm.a.Current()
	if p1 != p2 {
		panic(fmt.Sprintf("lm.A %d != lm.B %d", p1, p2))
	}
	return p1, s1 + " and " + s2
}

func (lm *and) Stop() {
	lm.a.Stop()
	lm.b.Stop()
}

func (lm *and) run() {
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

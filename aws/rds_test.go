// Copyright 2024 Block, Inc.

package aws

import (
	"testing"
)

func TestRDSCert(t *testing.T) {
	var err any
	f := func() {
		defer func() {
			err = recover()
		}()
		RegisterRDSCA() // panics on any error
	}
	f()
	if err != nil {
		t.Errorf("RegisterRDSCA paniced: %v", err)
	}
}

package config_test

import (
	"testing"

	"github.com/square/finch/config"
)

func TestValidate_Stage(t *testing.T) {
	c := config.Stage{}
	err := c.Validate("test")
	if err == nil {
		t.Error("stage with disable=false and zero trx returned err=nil, expected validation error")
	}

	c.Disable = true
	err = c.Validate("test")
	if err != nil {
		t.Errorf("stage with disable=true and zero trx returned an error, experted err=nil: %v", err)
	}
}

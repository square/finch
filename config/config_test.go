package config_test

import (
	"os"
	"testing"

	"github.com/square/finch/config"
)

func TestValidate_Stage(t *testing.T) {
	c := config.Stage{}
	err := c.Validate()
	if err == nil {
		t.Error("stage with disable=false and zero trx returned err=nil, expected validation error")
	}

	c.Disable = true
	err = c.Validate()
	if err != nil {
		t.Errorf("stage with disable=true and zero trx returned an error, experted err=nil: %v", err)
	}
}

func TestVars(t *testing.T) {
	params := map[string]string{
		"foo": "bar",
		"n":   "100",
		"a-b": "val",
	}

	home := os.Getenv("HOME")

	var tests = []struct {
		s, expect string
	}{
		{"rows: 5", "rows: 5"},
		{"rows: $params.n", "rows: 100"},
		{"rows: ${params.n}", "rows: 100"},
		{`p: "${params.foo}"`, `p: "bar"`},
		{`p: _${params.foo}_`, `p: _bar_`},
		{`r: $params.a-b`, `r: val`},
		{"key: $params.n $params.foo", "key: 100 bar"},
		{"home: $HOME", "home: " + home}, // env var
		{"rows: 1K", "rows: 1000"},
		{"rows: 1,000", "rows: 1000"},
		{"size: 1GiB", "size: 1073741824"},
		{"(1, 2, 'foo')", "(1, 2, 'foo')"},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got, err := config.Vars(tt.s, params)
			if err != nil {
				t.Errorf("got an error, expected nil: %v", err)
			}
			if got != tt.expect {
				t.Errorf("got '%s', expected '%s'", got, tt.expect)
			}
		})
	}
}

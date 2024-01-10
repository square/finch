package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-test/deep"

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
		numbers   bool
	}{
		// numbers=true (humanize numbers: 1k -> 1000)
		{"rows: 5", "rows: 5", true},
		{"rows: $params.n", "rows: 100", true},
		{"rows: ${params.n}", "rows: 100", true},
		{`p: "${params.foo}"`, `p: "bar"`, true},
		{`p: _${params.foo}_`, `p: _bar_`, true},
		{`r: $params.a-b`, `r: val`, true},
		{"key: $params.n $params.foo", "key: 100 bar", true},
		{"home: $HOME", "home: " + home, true}, // env var
		{"rows: 1K", "rows: 1000", true},
		{"rows: 1,000", "rows: 1000", true},
		{"size: 1GiB", "size: 1073741824", true},
		{"(1, 2, 'foo')", "(1, 2, 'foo')", true},
		// numbers=false
		{"db.abd6b.us-east-1.rds.amazonaws.com", "db.abd6b.us-east-1.rds.amazonaws.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got, err := config.Vars(tt.s, params, tt.numbers)
			if err != nil {
				t.Errorf("got an error, expected nil: %v", err)
			}
			if got != tt.expect {
				t.Errorf("got '%s', expected '%s'", got, tt.expect)
			}
		})
	}
}

func TestLoadWithBase(t *testing.T) {
	stages, err := config.Load([]string{"../test/config/b1/stage.yaml"}, nil, "", "")
	if err != nil {
		t.Error(err)
	}
	if len(stages) == 0 {
		t.Fatalf("got 0 stages, expected 1")
	}
	fileName, _ := filepath.Abs("../test/config/b1/stage.yaml")
	expect := config.Stage{
		N:    1,
		Name: "test",
		File: fileName,
		Compute: config.Compute{
			Instances: "1",
		},
		Params: map[string]string{
			"foo": "test",
		},
		Stats: config.Stats{
			Freq: "0s",
			Report: map[string]map[string]string{
				"stdout": map[string]string{
					"each-instance": "true",
				},
			},
		},
		Trx: []config.Trx{
			{
				Name: "trx.sql",
				File: "trx.sql",
			},
		},
	}
	if diff := deep.Equal(stages[0], expect); diff != nil {
		t.Error(diff)
	}
}

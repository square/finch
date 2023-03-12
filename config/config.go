// Copyright 2022 Block, Inc.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

func Load(filePath string) (File, error) {
	file, err := filepath.Abs(filePath)
	if err != nil {
		return File{}, err
	}

	if _, err := os.Stat(file); err != nil {
		return File{}, fmt.Errorf("config file %s does not exist", filePath)
	}

	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		// err includes file name, e.g. "read config file: open <file>: no such file or directory"
		return File{}, fmt.Errorf("cannot read config file: %s", err)
	}

	var cfg File
	if err := yaml.UnmarshalStrict(bytes, &cfg); err != nil {
		return cfg, fmt.Errorf("cannot decode YAML in %s: %s", file, err)
	}

	return cfg, nil
}

// ValidFreq validates the freq value for the given config section and returns
// nil if valid, else returns an error.
func ValidFreq(freq, config string) error {
	if freq == "" {
		return nil
	}
	if freq == "0" {
		return fmt.Errorf("invalid config.%s: 0: must be greater than zero", config)
	}
	d, err := time.ParseDuration(freq)
	if err != nil {
		return fmt.Errorf("invalid config.%s: %s: %s", config, freq, err)
	}
	if d <= 0 {
		return fmt.Errorf("invalid config.%s: %s (%d): value <= 0; must be greater than zero", config, freq, d)
	}
	return nil
}

// True returns true if b is non-nil and true.
// This is convenience function related to *bool files in config structs,
// which is required for knowing when a bool config is explicitily set
// or not. If set, it's not changed; if not, it's set to the default value.
// That makes a good config experience but a less than ideal code experience
// because !*b will panic if b is nil, hence the need for this func.
func True(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func FileExists(filePath string) bool {
	file, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	_, err = os.Stat(file)
	return err == nil
}

var envvar = regexp.MustCompile(`\${([\w_.-]+)(?:(\:\-)([\w_.-]*))?}`)

// interpolateEnv changes ${FOO} to the value of environment variable FOO.
// It also changes ${FOO:-bar} to "bar" if env var FOO is an empty string.
// Only strict matching, else the original string is returned.
func interpolateEnv(v string) string {
	if !strings.Contains(v, "${") {
		return v
	}
	m := envvar.FindStringSubmatch(v)
	if len(m) != 4 {
		return v // strict match only
	}
	v2 := os.Getenv(m[1])
	if v2 == "" && m[2] != "" {
		return m[3]
	}
	return envvar.ReplaceAllLiteralString(v, v2)
}

// setBool sets c to the value of b if c is nil (not set). Pointers are required
// because var b bool is false by default whether set or not, so we can't tell if
// it's explicitly set in config file. But var b *bool is only false if explicitly
// set b=false, else it's nil, so no we can tell if the var is set or not.
func setBool(c *bool, b *bool) *bool {
	if c == nil && b != nil {
		c = new(bool)
		*c = *b
	}
	return c
}

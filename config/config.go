// Copyright 2023 Block, Inc.

package config

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	human "github.com/dustin/go-humanize"
	"gopkg.in/yaml.v2"

	"github.com/square/finch"
)

// stageFile represents the stage file layout:
//
//	stage:
//	  name: read-only
//	  runtime: 60s
//	  workload:
//	    - clients: 1
//
// This is purely "decorative" because I like the explicit "stage" at top
// because it makes it clear that what follows is a stage config.
type stageFile struct {
	Stage Stage `yaml:"stage"`
}

func Load(stageFiles []string, kvparams []string, dsn, db string) ([]Stage, error) {
	var err error
	base := map[string]Base{}
	stages := []Stage{}

	// Get and restore current working dir because we chdir to validate stage stageFiles
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	finch.Debug("cwd %s", cwd)
	defer func() {
		os.Chdir(cwd)
	}()

	params := map[string]string{}
	for _, kv := range kvparams {
		f := strings.SplitN(kv, "=", 2)
		if len(f) != 2 {
			log.Printf("Ignoring invalid --param %s: split into %d fields, expected 2\n", kv, len(f))
			continue
		}
		params[f[0]] = f[1]
	}

	for n, fileName := range stageFiles {
		// Load base file (_all.yaml) once for the dir, if it exists
		dir := filepath.Dir(fileName)
		b, ok := base[dir]
		if !ok {
			baseFile := filepath.Join(dir, "_all.yaml")
			if FileExists(baseFile) {
				finch.Debug("base: %s", baseFile)
				bytes, err := read(baseFile)
				if err != nil {
					return nil, err
				} else {
					var newb Base
					if err := yaml.UnmarshalStrict(bytes, &newb); err != nil {
						return nil, fmt.Errorf("cannot decode YAML in %s: %s", fileName, err)
					}
					base[dir] = newb
					b = newb
					finch.Debug("base: %+v", b)
				}
			} else {
				finch.Debug("base: none in %s", dir)
			}

			// --param foo=bar on command line overrides .params in stage files
			if len(b.Params) > 0 {
				if b.Params == nil {
					b.Params = map[string]string{}
				}
				for k, v := range params {
					b.Params[k] = v
				}
			}

			base[dir] = b
		}

		// Load stage file, which includes and overwrite the optional base config (b)
		bytes, err := read(fileName)
		if err != nil {
			return nil, err
		}
		absFile, err := filepath.Abs(fileName)
		if err != nil {
			return nil, err
		}
		f := &stageFile{Stage: Stage{
			File: absFile,
			N:    uint(n + 1),
		}}
		if err := yaml.UnmarshalStrict(bytes, f); err != nil {
			return nil, fmt.Errorf("cannot decode YAML in %s: %s", fileName, err)
		}

		// Set stage with defaults (base)
		f.Stage.With(b)

		// --dsn and --database on command line override config files
		if dsn != "" {
			f.Stage.MySQL.DSN = dsn
		}
		if db != "" {
			f.Stage.MySQL.Db = db
		}

		// interpolate $vars -> values (see Vars func below)
		if err := f.Stage.Vars(); err != nil {
			return nil, fmt.Errorf("in %s: %s", fileName, err)
		}

		// Chdir to confit file so relative trx file paths in the config work,
		// e.g. "trx.file: trx/foo.sql" where trx/ is relative to the dir where
		// the config file is located.
		os.Chdir(filepath.Dir(fileName))

		// Validate config now that it's final (interpolated vars and chdir)
		if err := f.Stage.Validate(); err != nil {
			return nil, fmt.Errorf("%s invalid: %s", fileName, err)
		}
		stages = append(stages, f.Stage)
		finch.Debug("%+v", f.Stage)

		os.Chdir(cwd)
	}
	return stages, nil
}

func read(filePath string) ([]byte, error) {
	finch.Debug("read %s", filePath)
	file, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(file); err != nil {
		return nil, err
	}
	return ioutil.ReadFile(file)
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

var varRE = []*regexp.Regexp{
	regexp.MustCompile(`\${([^}]+)}`),  // ${param.foo} for "hello${param.foo}bar"
	regexp.MustCompile(`\$([^\s"']+)`), // $param.foo for standalone value
}
var reHumanNumber = regexp.MustCompile(`([\d,]*\d+(?i:[MKGBI]*))`) // 1M or 1,000,000 -> 1000000
var reAllDigits = regexp.MustCompile(`^\d+$`)

// Vars changes $params.foo and $FOO to param values and environment variable
// values, respectively, and human numbers to integers (1k -> 1000).
// "${var}" is also valid but YAML requires string quotes around {}.
func Vars(s string, params map[string]string, numbers bool) (string, error) {
	for _, r := range varRE {
		m := r.FindAllStringSubmatch(s, -1)
		if len(m) == 0 {
			continue
		}
		rep := make([]string, 0, len(m)*2)
		for i := range m {
			v := m[i]
			//p := strings.Trim(v[1], "{}")
			p := v[1]
			switch {
			case strings.HasPrefix(p, "params."):
				k := strings.TrimPrefix(p, "params.")
				val, ok := params[k]
				if !ok {
					return "", fmt.Errorf("%s not defined (is it spelled correctly?)", p)
				}
				rep = append(rep, v[0], val)
				finch.Debug("param: %s -> %v (user-defined)", s, rep)
			case strings.HasPrefix(p, "sys."):
				k := strings.TrimPrefix(p, "sys.")
				val, ok := finch.SystemParams[k]
				if !ok {
					return "", fmt.Errorf("%s not defined (is it spelled correctly?)", p)
				}
				rep = append(rep, v[0], val)
				finch.Debug("param: %s -> %v (built-in)", s, rep)
			default:
				val, ok := os.LookupEnv(p)
				if !ok {
					return "", fmt.Errorf("environment variable %s not set (is it spelled correctly?)", p)
				}
				rep = append(rep, v[0], val)
				finch.Debug("param: %s -> %v (env var)", s, rep)
			}
		}
		r := strings.NewReplacer(rep...)
		s = r.Replace(s)
	}

	if !numbers {
		return s, nil
	}

	// Look for human numbers like 1k and 1,000
	m := reHumanNumber.FindAllStringSubmatch(s, -1)
	if len(m) == 0 {
		return s, nil // no human numbers
	}
	rep := []string{}
	for i := range m {
		// To keep the regex simple, reHumanNumber also matches ints like 1000,
		// but don't waste time parsing something that's already machine number.
		if reAllDigits.MatchString(m[i][0]) {
			continue
		}
		d, err := human.ParseBytes(m[i][1])
		if err != nil {
			return "", fmt.Errorf("invalid human-readable number: %s: %s", m[i][1], err)
		}
		rep = append(rep, m[i][0], strconv.FormatUint(d, 10))
	}
	if len(rep) == 0 { // all machine numbers (see comment above)
		return s, nil
	}
	finch.Debug("var: %s -> %v", s, rep)
	r := strings.NewReplacer(rep...)
	return r.Replace(s), nil
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

func parseInt(s string) error {
	if s == "" {
		return nil
	}
	_, err := strconv.ParseUint(s, 10, 32)
	return err
}

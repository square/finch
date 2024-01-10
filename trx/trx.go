// Copyright 2024 Block, Inc.

package trx

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/limit"
)

const (
	STMT  = byte(0x0)
	BEGIN = byte(0x1)
	END   = byte(0x2)
)

const EXPLICIT_CALL_SUFFIX = "()"

var DataKeyPattern = regexp.MustCompile(`@[\w_-]+(?:\(\))?`)
var ExplicitCallPattern = regexp.MustCompile(`@[\w_-]+\(\)`)

// Set is the complete set of transactions (and statements) for a stage.
type Set struct {
	Order      []string                // trx names in config order
	Statements map[string][]*Statement // keyed on trx name
	Meta       map[string]Meta         // keyed on trx name
	Data       *data.Scope             // keyed on data key (@d)
}

// Statement is one query in a transaction and all its read-only metadata.
type Statement struct {
	Trx          string
	Query        string
	ResultSet    bool
	Prepare      bool
	PrepareMulti int
	Begin        bool
	Commit       bool
	Write        bool
	DDL          bool
	Idle         time.Duration
	Inputs       []string // data keys (number of values)
	Outputs      []string // data keys save-results|columns and save-insert-id
	InsertId     string   // data key (special output)
	Limit        limit.Data
	Calls        []byte
}

type Meta struct {
	DDL bool
}

// Load loads all trx files and returns a Set representing all parsed trx.
// This is called from stage.Prepare since a stage comprises all trx.
// The given data scope comes from compute.Server to handle globally scoped
// data keys. Params are user-defined from the stage file: stage.params.
// The stage uses the returned Set for workload allocation based on however
// stage.workload mixes and matches trx to exec/client groups.
func Load(trxFiles []config.Trx, scope *data.Scope, params map[string]string) (*Set, error) {
	set := &Set{
		Order:      make([]string, 0, len(trxFiles)),
		Statements: map[string][]*Statement{},
		Data:       scope,
		Meta:       map[string]Meta{},
	}
	for i := range trxFiles {
		if err := NewFile(trxFiles[i], set, params).Load(); err != nil {
			return nil, err
		}
	}
	return set, nil
}

var ErrEOF = fmt.Errorf("EOF")

type lineBuf struct {
	n      uint
	str    string
	mods   []string
	copyNo uint
}

// File represents and loads one trx file. File.Load is called by the pkg func,
// trx.Load, which is called by stage.Prepare. Do not call File.Load directly
// except for testing.
type File struct {
	cfg    config.Trx        // stage.trx[]
	set    *Set              // trx set for the stage, what File.Load fills in
	params map[string]string // stage.params: user-defined value interpolation
	// --
	lb      lineBuf        // save lines until a complete statement is read
	colRefs map[string]int // column ref counts to detect unused ones
	stmtNo  uint           // 1-indexed in file (not a line number; not an index into stmt)
	stmts   []*Statement   // all statements in this file
	hasDDL  bool           // true if any statement is DDL
}

func NewFile(cfg config.Trx, set *Set, params map[string]string) *File {
	return &File{
		cfg:     cfg,
		set:     set,
		params:  params,
		colRefs: map[string]int{},
		lb:      lineBuf{mods: []string{}},
		stmts:   []*Statement{},
		stmtNo:  0,
	}
}

func (f *File) Load() error {
	finch.Debug("loading %s", f.cfg.File)
	file, err := os.Open(f.cfg.File)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		err = f.line(strings.TrimSpace(scanner.Text()))
		if err != nil {
			if err == ErrEOF {
				break
			}
			return err
		}
	}
	err = f.line("") // last line
	if err != nil {
		return err
	}

	if len(f.stmts) == 0 {
		return fmt.Errorf("trx file %s has no statements; at least 1 is required", f.cfg.File)
	}

	noRefs := []string{}
	for col, refs := range f.colRefs {
		if refs > 0 {
			continue
		}
		noRefs = append(noRefs, col)
	}
	if len(noRefs) > 0 {
		return fmt.Errorf("saved columns not referenced: %s", strings.Join(noRefs, ", "))
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err) // shouldn't happen
	}

	f.set.Order = append(f.set.Order, f.cfg.Name)
	f.set.Statements[f.cfg.Name] = f.stmts
	f.set.Meta[f.cfg.Name] = Meta{
		DDL: f.hasDDL,
	}

	return nil
}

func (f *File) line(line string) error {
	f.lb.n++

	// More lines in statement
	if line != "" {
		finch.Debug("line %d: %s\n", f.lb.n, line)
		if strings.HasPrefix(line, "-- ") {
			if line == "-- EOF" {
				return ErrEOF
			}
			mod, err := config.Vars(strings.TrimSpace(strings.TrimPrefix(line, "--")), f.params, true)
			if err != nil {
				return fmt.Errorf("parsing modifier '%s' on line %d: %s", line, f.lb.n, err)
			}
			f.lb.mods = append(f.lb.mods, mod)
		} else {
			f.lb.str += line + " "
		}
		return nil
	}

	// Empty lines between statements
	if f.lb.str == "" {
		finch.Debug("line %d: space", f.lb.n)
		return nil
	}

	// End of statement
	finch.Debug("line %d: end prev", f.lb.n)
	s, err := f.statements()
	if err != nil {
		return fmt.Errorf("error parsing %s at line %d: %s", f.cfg.File, f.lb.n-1, err)
	}
	for i := range s {
		finch.Debug("stmt: %+v", s[i])
	}
	f.stmts = append(f.stmts, s...)

	f.lb.str = ""
	f.lb.mods = []string{}

	return nil
}

var reKeyVal = regexp.MustCompile(`([\w_-]+)(?:\:\s*(\w+))?`)
var reCSV = regexp.MustCompile(`\/\*\!csv\s+(\d+)\s+(.+)\*\/`)
var reFirstWord = regexp.MustCompile(`^(\w+)`)

func (f *File) statements() ([]*Statement, error) {
	f.stmtNo++
	s := &Statement{
		Trx: f.cfg.Name, // trx name (trx.name or base(trx.file)
	}

	query := strings.TrimSpace(f.lb.str)
	finch.Debug("query raw: %s", query)

	// ----------------------------------------------------------------------
	// Switches
	// ----------------------------------------------------------------------

	// @todo regexp to extract first word
	com := strings.ToUpper(reFirstWord.FindString(query))
	switch com {
	case "SELECT":
		s.ResultSet = true
	case "BEGIN", "START":
		s.Begin = true // used to rate limit trx per second (TPS) in client/client.go
	case "COMMIT":
		s.Commit = true // used to measure TPS rate in client/client.go
	case "INSERT", "UPDATE", "DELETE", "REPLACE":
		s.Write = true
	case "ALTER", "CREATE", "DROP", "RENAME", "TRUNCATE":
		finch.Debug("DDL")
		s.DDL = true    // statement is DDL
		f.hasDDL = true // trx has DDL
	}

	// ----------------------------------------------------------------------
	// Modifiers: --prepare, --table-size, etc.
	// ----------------------------------------------------------------------

	for _, mod := range f.lb.mods {
		m := strings.Fields(mod)
		finch.Debug("mod: '%v' %#v", mod, m)
		if len(m) < 1 {
			return nil, fmt.Errorf("invalid modifier: '%s': does not match key: value (pattern match < 2)", mod)
		}
		m[0] = strings.Trim(m[0], ":")
		switch m[0] {
		case "prepare", "prepared":
			s.Prepare = true
		case "idle":
			d, err := time.ParseDuration(m[1])
			if err != nil {
				return nil, fmt.Errorf("invalid idle modifier: '%s': %s", mod, err)
			}
			s.Idle = d
		case "rows":
			max, err := strconv.ParseUint(m[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid rows limit: %s: %s", m[1], err)
			}
			var offset uint64
			if len(m) == 3 {
				offset, err = strconv.ParseUint(m[2], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid rows offset: %s: %s", m[2], err)
				}
			}
			finch.Debug("write limit: %d rows (offset %d)", max, offset)
			s.Limit = limit.Or(s.Limit, limit.NewRows(int64(max), int64(offset)))
		case "table-size", "database-size":
			if len(m) != 3 {
				return nil, fmt.Errorf("invalid %s modifier: split %d fields, expected 3: %s", m[0], len(m), mod)
			}
			max, err := humanize.ParseBytes(m[2])
			if err != nil {
				return nil, err
			}
			var lm limit.Data
			if m[0] == "table-size" {
				lm = limit.NewSize(max, m[2], "", m[1])
			} else { // database-size
				lm = limit.NewSize(max, m[2], m[1], "")
			}
			s.Limit = limit.Or(s.Limit, lm)
		case "save-insert-id":
			// @todo check len(m)
			if s.ResultSet {
				return nil, fmt.Errorf("save-insert-id not allowed on SELECT")
			}
			finch.Debug("save-insert-id")
			dataKey, err := f.column(0, m[1])
			if err != nil {
				return nil, err
			}
			s.InsertId = dataKey
			s.Outputs = append(s.Outputs, dataKey)
		case "save-columns":
			// @todo check len(m)
			for i, col := range m[1:] {
				// @todo split csv (handle "col1,col2" instead of "col1, col2")
				dataKey, err := f.column(i, col)
				if err != nil {
					return nil, err
				}
				s.Outputs = append(s.Outputs, dataKey)
			}
		case "copies":
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, fmt.Errorf("copies: %s invalid: %s", m[1], err)
			}
			if n < 0 {
				return nil, fmt.Errorf("copies: %s invalid: must be >= 0", m[1])
			}
			if n == 0 {
				return nil, nil
			}
			if n == 1 {
				continue
			}
			prepareMulti := false
			mods := make([]string, 0, len(f.lb.mods)-1)
			for _, mod := range f.lb.mods {
				if strings.HasPrefix(mod, "copies") {
					continue
				}
				if strings.HasPrefix(mod, "prepare") && !strings.Contains(query, finch.COPY_NUMBER) {
					prepareMulti = true
				}
				mods = append(mods, mod)
			}
			f.lb.mods = mods
			f.stmtNo--
			multi := make([]*Statement, n)
			for i := 0; i < n; i++ {
				finch.Debug("copy %d of %d", i+1, n)
				f.lb.copyNo = uint(i + 1)
				ms, err := f.statements() // recurse
				if err != nil {
					return nil, fmt.Errorf("during copy recurse: %s", err)
				}
				multi[i] = ms[0]
			}
			if prepareMulti {
				multi[0].PrepareMulti = n
			}
			f.lb.copyNo = 0
			return multi, nil
		default:
			return nil, fmt.Errorf("unknown modifier: %s: '%s'", m[0], mod)
		}
	}

	// ----------------------------------------------------------------------
	// Replace /*!copy-number*/
	// ----------------------------------------------------------------------
	query = strings.ReplaceAll(query, finch.COPY_NUMBER, fmt.Sprintf("%d", f.lb.copyNo))

	// ----------------------------------------------------------------------
	// Expand CSV /*!csv N template*/
	// ----------------------------------------------------------------------
	csvTemplate := ""
	m := reCSV.FindStringSubmatch(query)
	if len(m) > 0 {
		n, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid number of CSV values in %s: %s", m[0], err)
		}
		vals := make([]string, n)
		csvTemplate = strings.TrimSpace(m[2])

		keys := map[string]bool{}
		for _, name := range DataKeyPattern.FindAllString(csvTemplate, -1) {
			// Trim to look up data key in config because @ is not valid YAML.
			// The @ will be put back later because all other code expects it.
			name = cfgKey(name)
			dataCfg, ok := f.cfg.Data[name] // config.stage.trx[].data
			if !ok {
				return nil, fmt.Errorf("%s not configured: trx file uses %s but this data key is not configured in the stage file", name, name)
			}

			// @d in a CSV template defaults to row scope
			if dataCfg.Scope == "" {
				dataCfg.Scope = finch.SCOPE_ROW
				f.cfg.Data[name] = dataCfg
			}

			// Save row scoped @d in CSV template, ignore other scopes
			if dataCfg.Scope == finch.SCOPE_ROW {
				keys["@"+name] = true
			}
		}

		// Change first row scoped @d -> @d() so it generates new values per row
		csvTemplateScoped := RowScope(keys, csvTemplate)
		finch.Debug("csv %d %s -> %s", n, csvTemplate, csvTemplateScoped)

		// Expand template, e.g. 3 (@d) -> (@d), (@d), (@d)
		for i := int64(0); i < n; i++ {
			vals[i] = csvTemplateScoped
		}
		csv := strings.Join(vals, ", ")
		query = reCSV.ReplaceAllLiteralString(query, csv)
	}

	// ----------------------------------------------------------------------
	// Data keys: @d -> data.Generator
	// ----------------------------------------------------------------------
	dataKeys := DataKeyPattern.FindAllString(query, -1)
	finch.Debug("data keys: %v", dataKeys)
	if len(dataKeys) == 0 {
		s.Query = query
		return []*Statement{s}, nil // no data key, return early
	}
	s.Inputs = dataKeys

	s.Calls = Calls(s.Inputs)
	query = ExplicitCallPattern.ReplaceAllStringFunc(query, func(s string) string {
		return strings.TrimSuffix(s, EXPLICIT_CALL_SUFFIX)
	})

	dataFormats := map[string]string{} // keyed on data name
	for i, name := range s.Inputs {
		// Remove () from @d()
		name = strings.TrimSuffix(name, EXPLICIT_CALL_SUFFIX)
		s.Inputs[i] = name

		var g data.Generator
		var err error

		if k, ok := f.set.Data.Keys[name]; ok && k.Column >= 0 {
			f.colRefs[name]++
			g = k.Generator
		} else if name == "@PREV" {
			if i == 0 {
				return nil, fmt.Errorf("no @PREV data generator")
			}
			for p := i - 1; p >= 0; p-- {
				finch.Debug("%s <- %s", dataKeys[p], dataKeys[i])
				if dataKeys[p] == "@PREV" {
					continue
				}
				g = f.set.Data.Keys[dataKeys[p]].Generator
				break
			}
		} else {
			if k, ok = f.set.Data.Keys[name]; ok {
				g = k.Generator
			} else {
				dataCfg, ok := f.cfg.Data[cfgKey(name)] // config.stage.trx[].data
				if !ok {
					return nil, fmt.Errorf("%s not configured: trx file uses %s but this data key is not configured in the stage file", name, name)
				}
				finch.Debug("make data generator: %s %s scope: %s", dataCfg.Generator, name, dataCfg.Scope)

				if dataCfg.Scope == "" {
					dataCfg.Scope = finch.SCOPE_STATEMENT
					f.cfg.Data[name] = dataCfg
				}

				g, err = data.Make(
					dataCfg.Generator, // e.g. "auto-inc"
					name,              // @d
					dataCfg.Params,    // trx[].data.params, generator-specific
				)
				if err != nil {
					return nil, err
				}
				f.set.Data.Keys[name] = data.Key{
					Name:      name,
					Trx:       f.cfg.Name,
					Line:      f.lb.n - 1,
					Statement: f.stmtNo,
					Column:    -1,
					Scope:     dataCfg.Scope,
					Generator: g,
				}
				finch.Debug("%#v", k)
			}
		}

		if s.Prepare {
			dataFormats[name] = "?"
		} else {
			_, dataFormats[name] = g.Format()
		}
	}

	replacements := make([]string, len(dataFormats)*2) // *2 because key + value
	i := 0
	for k, v := range dataFormats {
		replacements[i] = k
		replacements[i+1] = v
		i += 2
	}
	finch.Debug("replacements: %v", replacements)
	r := strings.NewReplacer(replacements...)
	s.Query = r.Replace(query)

	// Caller debug prints full Statement
	return []*Statement{s}, nil
}

func (f *File) column(colNo int, col string) (string, error) {
	col = strings.TrimSpace(strings.TrimSuffix(col, ","))
	finch.Debug("col %s %d", col, colNo)

	// If no-op column "_"?
	if col == finch.NOOP_COLUMN {
		if _, ok := f.set.Data.Keys[finch.NOOP_COLUMN]; !ok {
			f.set.Data.Keys[finch.NOOP_COLUMN] = data.Key{
				Name:      finch.NOOP_COLUMN,
				Trx:       f.cfg.Name,
				Line:      f.lb.n - 1,
				Statement: f.stmtNo,
				Column:    colNo,
				Scope:     finch.SCOPE_GLOBAL,
				Generator: data.Noop,
			}
			finch.Debug("%#v", f.set.Data.Keys[finch.NOOP_COLUMN])
		}
		finch.Debug("saved no-op col %s @ %d", col, colNo)
		return finch.NOOP_COLUMN, nil
	}

	if k, ok := f.set.Data.Keys[col]; ok {
		return "", fmt.Errorf("duplicated saved column: %s (first use: %s)", col, k)
	}

	dataCfg, ok := f.cfg.Data[cfgKey(col)] // config.stage.trx.*.data
	if !ok {
		dataCfg = config.Data{
			Name:      col,
			Generator: "column",
			Scope:     finch.SCOPE_TRX,
		}
		fmt.Printf("No data params for column %s (%s line %d), default to non-quoted value\n", col, f.cfg.Name, f.lb.n-1)
	}

	g, err := data.Make("column", col, dataCfg.Params)
	if err != nil {
		return "", err
	}
	f.colRefs[col] = 0
	f.set.Data.Keys[col] = data.Key{
		Name:      col,
		Trx:       f.cfg.Name,
		Line:      f.lb.n - 1,
		Statement: f.stmtNo,
		Column:    colNo,
		Scope:     dataCfg.Scope,
		Generator: g,
	}
	finch.Debug("%#v", f.set.Data.Keys[col])
	return col, nil
}

func Calls(dataKeys []string) []byte {
	calls := make([]byte, len(dataKeys))
	for i, name := range dataKeys {
		if strings.HasSuffix(name, EXPLICIT_CALL_SUFFIX) {
			calls[i] = 1
		}
	}
	finch.Debug("calls: %v", calls)
	return calls
}

// RowScope changes every first occurrence of the keys from @d to @d()
// in csvTemplate. So "(@d, @d)" -> "(@d(), @d)". The explicit call @d()
// makes @d row scoped because each row will call @d again. This is called
// when the /*!csv N template */ is being processed (see reCSV).
func RowScope(keys map[string]bool, csvTemplate string) string {
	csvDataKeys := DataKeyPattern.FindAllString(csvTemplate, -1)
KEY:
	for dataKey := range keys { // row scoped keys
		for _, k := range csvDataKeys { // all keys in csvTemplate
			if !strings.HasPrefix(k, dataKey) {
				continue // not the row scoped key we're looking for
			}
			// This is first occurrence of row scoped key in csvTemplate.
			// Add () suffix if not already set.
			if !strings.HasSuffix(k, EXPLICIT_CALL_SUFFIX) {
				csvTemplate = strings.Replace(csvTemplate, k, k+EXPLICIT_CALL_SUFFIX, 1) // 1=only first occurrence
			}
			continue KEY // only check/change first occurrence, so this row scoped key is done
		}
	}
	return csvTemplate
}

func cfgKey(s string) string {
	return strings.Trim(s, "@"+EXPLICIT_CALL_SUFFIX)
}

// Copyright 2023 Block, Inc.

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

// Set is the complete set of transactions (and statments) for a stage.
type Set struct {
	Order      []string                // trx names in config order
	Statements map[string][]*Statement // keyed on trx name
	Meta       map[string]Meta
	Data       *data.Scope
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
	Idle         time.Duration
	Inputs       []string // data keys (number of values)
	Outputs      []string // data keys save-results|columns and save-insert-id
	InsertId     string   // data key (special output)
	Limit        limit.Data
}

type Meta struct {
	DDL bool
}

func Load(trxList []config.Trx, scope *data.Scope, params map[string]string) (*Set, error) {
	set := &Set{
		Order:      make([]string, 0, len(trxList)),
		Statements: map[string][]*Statement{},
		Data:       scope,
		Meta:       map[string]Meta{},
	}
	for i := range trxList {
		if err := NewFile(trxList[i], set, params).Load(); err != nil {
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

type File struct {
	lb      lineBuf
	set     *Set
	params  map[string]string
	cfg     config.Trx
	colRefs map[string]int
	dg      map[string]data.Generator
	stmtNo  uint // 1-indexed in file (not a line number; not an index into stmt)
	stmt    []*Statement
	hasDDL  bool
}

func NewFile(cfg config.Trx, set *Set, params map[string]string) *File {
	return &File{
		cfg:     cfg,
		set:     set,
		params:  params,
		colRefs: map[string]int{},
		dg:      map[string]data.Generator{},
		lb:      lineBuf{mods: []string{}},
		stmt:    []*Statement{},
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

	if len(f.stmt) == 0 {
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
	f.set.Statements[f.cfg.Name] = f.stmt
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
	s, err := f.statement()
	if err != nil {
		return err
	}
	for i := range s {
		finch.Debug("stmt: %+v", s[i])
	}
	f.stmt = append(f.stmt, s...)

	f.lb.str = ""
	f.lb.mods = []string{}

	return nil
}

var reKeyVal = regexp.MustCompile(`([\w_-]+)(?:\:\s*(\w+))?`)
var reData = regexp.MustCompile(`@[\w_-]+`)
var reCSV = regexp.MustCompile(`\/\*\!csv\s+(\d+)\s+(.+)\*\/`)
var reFirstWord = regexp.MustCompile(`^(\w+)`)

func (f *File) statement() ([]*Statement, error) {
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
		f.hasDDL = true
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
		case "save-result", "save-results":
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
				ms, err := f.statement() // recurse
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
	// Expand CSV /*!csv N val*/
	// ----------------------------------------------------------------------
	m := reCSV.FindStringSubmatch(query)
	if len(m) > 0 {
		n, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid number of CSV values in %s: %s", m[0], err)
		}
		vals := make([]string, n)
		val := strings.TrimSpace(m[2])
		finch.Debug("%dx %s", n, val)
		for i := int64(0); i < n; i++ {
			vals[i] = val
		}
		csv := strings.Join(vals, ", ")
		query = reCSV.ReplaceAllLiteralString(query, csv)
	}

	// ----------------------------------------------------------------------
	// Data keys: @d -> data.Generator
	// ----------------------------------------------------------------------
	dataKeys := reData.FindAllString(query, -1)
	finch.Debug("data keys: %v", dataKeys)
	if len(dataKeys) == 0 {
		s.Query = query
		return []*Statement{s}, nil // no data key, return early
	}
	s.Inputs = dataKeys
	dataFormats := map[string]string{} // keyed on data name
	for i, name := range s.Inputs {

		var g data.Generator
		var err error
		if k, ok := f.set.Data.Keys[name]; ok && k.Column >= 0 {
			f.colRefs[name]++
			g = k.Generator
		} else if name == "@PREV" {
			if i == 0 {
				return nil, fmt.Errorf("no @PREV data generator")
			}
			g = f.set.Data.Keys[dataKeys[i-1]].Generator
			finch.Debug("@PREV = %s: %s", dataKeys[i-1], g.Id())
		} else {
			if k, ok = f.set.Data.Keys[name]; ok {
				g = k.Generator
			} else {
				dataCfg, ok := f.cfg.Data[strings.TrimPrefix(name, "@")] // config.stage.trx.*.data
				if !ok {
					return nil, fmt.Errorf("no params for data name %s", name)
				}
				finch.Debug("make data generator %s for %s", dataCfg.Generator, name)
				g, err = data.Make(dataCfg.Generator, name, dataCfg.Scope, dataCfg.Params)
				if err != nil {
					return nil, err
				}
				f.set.Data.Keys[name] = data.Key{
					Name:      name,
					Trx:       f.cfg.Name,
					Line:      f.lb.n - 1,
					Statement: f.stmtNo,
					Column:    -1,
					Generator: g,
				}
				finch.Debug("%#v", k)
			}
		}

		if s.Prepare {
			dataFormats[name] = "?"
		} else {
			dataFormats[name] = g.Format()
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

	dataCfg, ok := f.cfg.Data[strings.TrimPrefix(col, "@")] // config.stage.trx.*.data
	if !ok {
		dataCfg = config.Data{
			Name:      col,
			Generator: "column",
			DataType:  "string",
		}
		fmt.Printf("No data params for %s (%s line %d), defaulting to string column\n", col, f.cfg.Name, f.lb.n-1)
	}

	// [...].data.col.data-type
	colType := ""
	switch dataCfg.DataType {
	case "numeric":
		colType = "n"
	case "string":
		colType = "s"
	default:
		return "", fmt.Errorf("invalid column type: %s: valid types: numeric, string", dataCfg.DataType)
	}
	g, err := data.Make("column", col, dataCfg.Scope, map[string]string{
		"name": col,
		"type": colType,
	})
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
		Generator: g,
	}
	finch.Debug("%#v", f.set.Data.Keys[col])
	return col, nil
}

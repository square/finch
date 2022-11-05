// Copyright 2022 Block, Inc.

package workload

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

type Statement struct {
	Query     string
	ResultSet bool
	Prepare   bool
	Defer     bool
	Begin     bool
	Idle      time.Duration
	Data      []data.Generator
	Values    int
	Limit     limit.Data
	Columns   []interface{}
	InsertId  data.Generator
}

type Trx struct {
	Name       string
	Statements []Statement
	Data       map[string]data.Generator
	Column     map[string]Column
}

type Column struct {
	Statement int
	Column    int
	Data      data.Generator
}

var ErrEOF = fmt.Errorf("EOF")

func (src Trx) ClientCopy(clientNo int) []Statement {

	dstTrxData := make(map[string]data.Generator, len(src.Data))
	for k, g := range src.Data {
		dstTrxData[k] = g.Copy(clientNo)
	}
	s := make([]Statement, len(src.Statements))

	for i := range src.Statements {
		dst := src.Statements[i]

		// New column copy
		if len(src.Statements[i].Columns) > 0 {
			dst.Columns = make([]interface{}, len(src.Statements[i].Columns))
			for n, srcCol := range src.Statements[i].Columns {
				if srcCol != data.Noop {
					continue
				}
				dst.Columns[n] = srcCol
				finch.Debug("                     stmt %2d col %2d is noop", i+1, n+1)
			}
		}

		dst.Data = make([]data.Generator, len(src.Statements[i].Data))
		for j, g := range src.Statements[i].Data {

			if g.Scope() == finch.SCOPE_TRANSACTION {
				dst.Data[j] = dstTrxData[g.Id().DataName] // first and only copy for trx
			} else {
				dst.Data[j] = g.Copy(clientNo) // new copy for this statement
			}

			// Is this data gen a pointer to an original data.Column, then
			// it's also in Columns in a previous statement, so go back and
			// update that previous ref.
			c, ok := src.Column[g.Id().DataName]
			if !ok {
				continue
			}
			finch.Debug("stmt %2d data %2d from stmt %2d col %2d: %s\n", i+1, j+1, c.Statement+1, c.Column+1, dst.Data[j].Id())
			s[c.Statement].Columns[c.Column] = dst.Data[j]

			// If statement saves insert ID, then update data gen for that too
			if s[c.Statement].InsertId != nil {
				s[c.Statement].InsertId = dst.Data[j]
			}
		}
		s[i] = dst
		// @todo Clone Limit
	}
	return s
}

func Transactions(w []config.Trx) ([]string, map[string]Trx, error) {
	names := []string{}     // workload order
	all := map[string]Trx{} // workload => statements
	for i := range w {
		trx, err := NewFile(w[i]).Load()
		if err != nil {
			return nil, nil, err
		}
		names = append(names, trx.Name)
		all[trx.Name] = trx
		finch.Debug("%d statements in trx %s", len(trx.Statements), w[i].Name)
	}
	return names, all, nil
}

type lineBuf struct {
	n    uint
	str  string
	mods []string
}

type File struct {
	lb      lineBuf
	trx     Trx
	cfg     config.Trx
	colRefs map[string]int
	dg      map[string]data.Generator
	stmtNo  int
}

func NewFile(cfg config.Trx) *File {
	return &File{
		cfg: cfg,
		trx: Trx{
			Name:       cfg.Name,
			Statements: []Statement{},
			Data:       map[string]data.Generator{},
			Column:     map[string]Column{},
		},
		colRefs: map[string]int{},
		dg:      map[string]data.Generator{},
		lb:      lineBuf{mods: []string{}},
		stmtNo:  -1,
	}
}

func (f *File) Load() (Trx, error) {
	finch.Debug("loading %s", f.cfg.File)
	file, err := os.Open(f.cfg.File)
	if err != nil {
		return Trx{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		err = f.line(strings.TrimSpace(scanner.Text()))
		if err != nil {
			if err == ErrEOF {
				break
			}
			return Trx{}, err
		}
	}
	err = f.line("") // last line
	if err != nil {
		return Trx{}, err
	}

	noRefs := []string{}
	for col, refs := range f.colRefs {
		if refs > 0 {
			continue
		}
		noRefs = append(noRefs, col)
	}
	if len(noRefs) > 0 {
		return Trx{}, fmt.Errorf("saved columns not referenced: %s", strings.Join(noRefs, ", "))
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err) // shouldn't happen
	}

	return f.trx, nil
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
			f.lb.mods = append(f.lb.mods, strings.TrimPrefix(line, "--"))
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
	finch.Debug("stmt: %+v", s)
	f.trx.Statements = append(f.trx.Statements, s)

	f.lb.str = ""
	f.lb.mods = []string{}

	return nil
}

var reKeyVal = regexp.MustCompile(`([\w_-]+)(?:\:\s*(\w+))?`)
var reData = regexp.MustCompile(`@[\w_-]+`)
var reCSV = regexp.MustCompile(`\/\*\!csv\s+(\d+)\s+(.+)\*\/`)

func (f *File) statement() (Statement, error) {
	f.stmtNo++
	s := Statement{}

	query := strings.TrimSpace(f.lb.str)
	finch.Debug("query raw: %s", query)

	// @todo regexp to extract first word
	if strings.HasPrefix(strings.ToUpper(query), "SELECT") {
		s.ResultSet = true
	}
	if strings.HasPrefix(strings.ToUpper(query), "BEGIN") && strings.HasPrefix(strings.ToUpper(query), "START") {
		s.Begin = true // used to measure trx per second (TPS)
	}

	for _, mod := range f.lb.mods {
		m := strings.Fields(mod)
		finch.Debug("mod: '%v' %#v", mod, m)
		if len(m) < 1 {
			return Statement{}, fmt.Errorf("invalid modifier: '%s': does not match key: value (pattern match < 2)", mod)
		}
		m[0] = strings.Trim(m[0], ":")
		switch m[0] {
		case "prepare", "prepared":
			s.Prepare = true
			if len(m) == 2 && strings.ToLower(m[1]) == "defer" {
				s.Defer = true
			}
		case "idle":
			d, err := time.ParseDuration(m[1])
			if err != nil {
				return Statement{}, fmt.Errorf("invalid idle modifier: '%s': %s", mod, err)
			}
			s.Idle = d
		case "rows":
			max, err := strconv.ParseUint(m[1], 10, 64)
			if err != nil {
				return Statement{}, fmt.Errorf("invalid rows limit: %s: %s", m[1], err)
			}
			var offset uint64
			if len(m) == 3 {
				offset, err = strconv.ParseUint(m[2], 10, 64)
				if err != nil {
					return Statement{}, fmt.Errorf("invalid rows offset: %s: %s", m[2], err)
				}
			}
			finch.Debug("write limit: %d rows (offset %d)", max, offset)
			s.Limit = limit.Or(s.Limit, limit.NewRows(int64(max), int64(offset)))
		case "table-size", "database-size":
			if len(m) != 3 {
				return Statement{}, fmt.Errorf("invalid %s modifier: split %d fields, expected 3: %s", m[0], len(m), mod)
			}
			max, err := humanize.ParseBytes(m[2])
			if err != nil {
				return Statement{}, err
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
				return Statement{}, fmt.Errorf("save-insert-id not allowed on SELECT")
			}
			finch.Debug("save-insert-id")
			g, err := f.column(0, m[1])
			if err != nil {
				return Statement{}, err
			}
			s.InsertId = g
			s.Columns = []interface{}{g}
		case "save-result", "save-results":
			// @todo check len(m)
			s.Columns = make([]interface{}, len(m[1:]))
			for i, colAndType := range m[1:] {
				// @todo split csv (handle "col1,col2" instead of "col1, col2")
				g, err := f.column(i, colAndType)
				if err != nil {
					return Statement{}, err
				}
				s.Columns[i] = g
			}
		default:
			return s, fmt.Errorf("unknown modifier: %s: '%s'", m[0], mod)
		}
	}

	// Expand CSV /*!csv N val*/
	m := reCSV.FindStringSubmatch(query)
	if len(m) > 0 {
		n, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil {
			return s, fmt.Errorf("invalid number of CSV values in %s: %s", m[0], err)
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

	// Data geneartors (@x in "WHERE col = @x")
	dataNames := reData.FindAllString(query, -1)
	finch.Debug("data names: %v", dataNames)
	if len(dataNames) == 0 {
		s.Query = query
		return s, nil
	}

	s.Values = len(dataNames)          // a value for every @data
	s.Data = []data.Generator{}        // len(s.Data) can be < s.Values due to @PREV
	dataFormats := map[string]string{} // keyed on data name
	for i, name := range dataNames {
		var g data.Generator
		var err error
		if c, ok := f.trx.Column[name]; ok {
			finch.Debug("stmt %d %s uses saved col %s from stmt %d col %d", f.stmtNo+1, name, c.Data.Id(), c.Statement+1, c.Column+1)
			f.colRefs[name]++
			g = c.Data
			s.Data = append(s.Data, g)
		} else if name == "@PREV" {
			if i == 0 {
				return s, fmt.Errorf("no @PREV data generator")
			}
			g = f.dg[dataNames[i-1]]
			finch.Debug("@PREV = %s: %s", dataNames[i-1], g.Id())
		} else {
			g, ok = f.dg[name]
			if !ok {
				d, ok := f.cfg.Data[strings.TrimPrefix(name, "@")] // config.stage.workload.*.data
				if !ok {
					return s, fmt.Errorf("no params for data name %s", name)
				}
				finch.Debug("make data generator %s for %s", d.Generator, name)
				g, err = data.Make(d.Generator, name, d.Params)
				if err != nil {
					return s, err
				}
				f.dg[name] = g
				if g.Scope() == finch.SCOPE_TRANSACTION {
					finch.Debug("%s trx scoped", g.Id())
					f.trx.Data[g.Id().DataName] = g
				}
			}
			s.Data = append(s.Data, g)
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
	return s, nil
}

func (f *File) column(colNo int, colAndType string) (data.Generator, error) {
	colAndType = strings.TrimSpace(strings.TrimSuffix(colAndType, ","))
	if colAndType == "_" {
		return data.Noop, nil
	}
	p := strings.Split(colAndType, ":") // col:type
	col := p[0]
	if _, ok := f.trx.Column[col]; ok {
		return nil, fmt.Errorf("duplicated saved column: %s", col)
	}
	t := "n" // default: numeric column, format %d
	if len(p) == 2 {
		t = p[1]
	}
	var colFmt string // column format: %d or '%s'
	colType := strings.TrimSuffix(t, ",")
	switch colType {
	case "n":
	case "s":
	default:
		return nil, fmt.Errorf("invalid column type: %s: valid types: n, s", t)
	}
	g, err := data.Make("column", col, map[string]string{"col": col, "type": colType})
	if err != nil {
		return nil, err
	}
	f.colRefs[col] = 0
	f.trx.Column[col] = Column{
		Statement: f.stmtNo,
		Column:    colNo,
		Data:      g,
	}
	f.trx.Data[g.Id().DataName] = g // data.Column are trx scoped
	finch.Debug("saved col %s %s", col, colFmt)
	return g, nil
}

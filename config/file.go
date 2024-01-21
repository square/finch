// Copyright 2024 Block, Inc.

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/square/finch"
)

// Base represents a base config file: _all.yaml. If it exists, it applies to
// all stage config files in the directory.
type Base struct {
	MySQL  MySQL             `yaml:"mysql,omitempty"`
	Params map[string]string `yaml:"params,omitempty"`
	Stats  Stats             `yaml:"stats,omitempty"`
}

func (c *Base) Validate() error {
	if err := c.MySQL.Validate(); err != nil {
		return err
	}
	if err := c.Stats.Validate(); err != nil {
		return err
	}
	return nil
}

// Stage represents one stage config file. The stage config overwrites any base
// config (_all.yaml).
type Stage struct {
	Compute  Compute           `yaml:"compute,omitempty"`
	Disable  bool              `yaml:"disable"`
	File     string            `yaml:"-"`
	Id       string            `yaml:"-"`
	Name     string            `yaml:"name"`
	MySQL    MySQL             `yaml:"mysql,omitempty"`
	N        uint              `yaml:"-"`
	Params   map[string]string `yaml:"params,omitempty"`
	QPS      string            `yaml:"qps,omitempty"` // uint
	Runtime  string            `yaml:"runtime,omitempty"`
	Stats    Stats             `yaml:"stats,omitempty"`
	TPS      string            `yaml:"tps,omitempty"` // uint
	Test     bool              `yaml:"-"`
	Trx      []Trx             `yaml:"trx,omitempty"`
	Workload []ClientGroup     `yaml:"workload,omitempty"`
}

func (c *Stage) With(b Base) {
	// Apply base config to stage before reading stage config file. This means
	// any base config values are overwritten if set in the stage config file.
	if len(b.Params) > 0 {
		if c.Params == nil {
			c.Params = map[string]string{}
		}
		for k, v := range b.Params {
			if _, ok := c.Params[k]; !ok {
				c.Params[k] = v
			}
		}
	}

	// Shallow copy MySQL because it has no maps or slices, but setBool to create
	// new *bool pointers
	c.MySQL.With(b.MySQL)
	/*
		c.MySQL = b.MySQL
		c.MySQL.DisableAutoTLS = setBool(c.MySQL.DisableAutoTLS, b.MySQL.DisableAutoTLS)
		c.MySQL.TLS.SkipVerify = setBool(c.MySQL.TLS.SkipVerify, b.MySQL.TLS.SkipVerify)
		c.MySQL.TLS.Disable = setBool(c.MySQL.TLS.Disable, b.MySQL.TLS.Disable)

		if b.MySQL.Db != "" {
			for i := range c.Workload {
				c.Workload[i].Db = b.MySQL.Db
			}
		}
	*/

	// Stats has a map, so copy in all fields manually
	c.Stats.Disable = setBool(c.Stats.Disable, b.Stats.Disable)
	c.Stats.Freq = b.Stats.Freq
	if len(b.Stats.Report) > 0 {
		c.Stats.Report = map[string]map[string]string{}
		for r := range b.Stats.Report {
			c.Stats.Report[r] = map[string]string{}
			for k, v := range b.Stats.Report[r] {
				c.Stats.Report[r][k] = v
			}
		}
	}

}

func (c *Stage) Vars() error {
	var err error

	// First interpolate the params before they're used in other config vars
	for k, v := range c.Params {
		c.Params[k], err = Vars(v, c.Params, true)
		if err != nil {
			return fmt.Errorf("in params: %s", err)
		}
	}

	// Interpolate $params.var in other config vars
	c.Name, err = Vars(c.Name, c.Params, false)
	if err != nil {
		return err
	}
	c.Runtime, err = Vars(c.Runtime, c.Params, false)
	if err != nil {
		return err
	}
	c.QPS, err = Vars(c.QPS, c.Params, true)
	if err != nil {
		return err
	}
	c.TPS, err = Vars(c.TPS, c.Params, true)
	if err != nil {
		return err
	}
	if err := c.Compute.Vars(c.Params); err != nil {
		return fmt.Errorf("in compute: %s", err)
	}
	if err := c.MySQL.Vars(c.Params); err != nil {
		return fmt.Errorf("in mysql: %s", err)
	}
	if err := c.Stats.Vars(c.Params); err != nil {
		return fmt.Errorf("in stats: %s", err)
	}
	for i := range c.Trx {
		if err := c.Trx[i].Vars(c.Params); err != nil {
			return fmt.Errorf("in trx: %s", err)
		}
	}
	for i := range c.Workload {
		if err := c.Workload[i].Vars(c.Params); err != nil {
			return fmt.Errorf("in workload: %s", err)
		}
	}
	return nil
}

func (c *Stage) Validate() error {
	if c.Disable {
		return nil
	}

	if c.Name == "" {
		c.Name = filepath.Base(c.File)
	}

	if len(c.Trx) == 0 {
		return fmt.Errorf("stage %s has zero trx files and is not disabled; specify at least 1 trx file or %s.disable = true", c.Name, c.Name)
	}

	// Trx list: must validate before Workload because Workload reference trx by name
	seen := map[string]string{}
	for i := range c.Trx {
		if c.Trx[i].File == "" {
			return fmt.Errorf("no file specified for trx %d", i+1)
		}
		if !FileExists(c.Trx[i].File) {
			return fmt.Errorf("trx %d file %s does not exist", i+1, c.Trx[i].File)
		}
		if c.Trx[i].Name == "" {
			c.Trx[i].Name = filepath.Base(c.Trx[i].File)
		}

		for dataKey, data := range c.Trx[i].Data {
			if data.Generator == "" {
				return fmt.Errorf("trx[%d].data[%s].generator not set; see https://square.github.io/finch/syntax/stage-file/#dgenerator", i, dataKey)
			}
			scope := data.Scope
			switch scope {
			case
				"",
				finch.SCOPE_GLOBAL,
				finch.SCOPE_STAGE,
				finch.SCOPE_WORKLOAD,
				finch.SCOPE_EXEC_GROUP,
				finch.SCOPE_CLIENT_GROUP,
				finch.SCOPE_CLIENT,
				finch.SCOPE_ITER,
				finch.SCOPE_TRX,
				finch.SCOPE_STATEMENT,
				finch.SCOPE_ROW,
				finch.SCOPE_VALUE:
				// ok
			default:
				return fmt.Errorf("invalid data scope: trx[%d].data[%s].scope: %s; see https://square.github.io/finch/syntax/stage-file/#dscope", i, dataKey, scope)
			}

			if prevScope, ok := seen[dataKey]; !ok {
				seen[dataKey] = scope
			} else if prevScope != scope {
				return fmt.Errorf("scope mismatch: %s: %s then %s", dataKey, prevScope, scope)
			}
		}
	}

	// Workload
	names := map[string]int{}
	withTrx := map[int]int{}
	withoutTrx := map[int]int{}
	for i := range c.Workload {
		if err := c.Workload[i].Validate(c.Trx); err != nil {
			return err
		}

		if c.Workload[i].Group != "" {
			if last, ok := names[c.Workload[i].Group]; !ok {
				names[c.Workload[i].Group] = i
			} else {
				if last != i-1 {
					return fmt.Errorf("duplicate or non-consecutive execution group name: %s: first at %s.workload[%d], then at %s.workoad[%d]; unique group names must consecutive", c.Workload[i].Group, c.Name, last, c.Name, i)
				}
			}
		}

		if len(c.Workload[i].Trx) > 0 {
			withTrx[i] = i
		TRX:
			for j, trxName := range c.Workload[i].Trx {
				for k := range c.Trx {
					if trxName == c.Trx[k].Name {
						continue TRX
					}
				}
				return fmt.Errorf("%s.workload[%d].trx[%d]: '%s' not defined in %s.trx", c.Name, i, j, trxName, c.Name)
			}
		} else {
			withoutTrx[i] = i
		}
	}

	if len(withTrx) > 0 && len(withoutTrx) > 0 {
		return fmt.Errorf("%s.workload has mixed trx assignments", c.Name)
	}

	// Runtime
	if err := ValidFreq(c.Runtime, "workload"); err != nil {
		return err
	}

	if err := parseInt(c.QPS); err != nil {
		return fmt.Errorf("tps: '%s' is not an integer: %s", c.QPS, err)
	}
	if err := parseInt(c.TPS); err != nil {
		return fmt.Errorf("tps: '%s' is not an integer: %s", c.TPS, err)
	}

	if err := c.MySQL.Validate(); err != nil {
		return err
	}

	if err := c.Compute.Validate(); err != nil {
		return err
	}

	if err := c.Stats.Validate(); err != nil {
		return err
	}

	return nil
}

// --------------------------------------------------------------------------

type Compute struct {
	DisableLocal bool   `yaml:"disable-local,omitempty"`
	Instances    string `yaml:"instances,omitempty"` // uint
}

func (c *Compute) Vars(params map[string]string) error {
	var err error
	c.Instances, err = Vars(c.Instances, params, true)
	if err != nil {
		return err
	}
	return nil
}

func (c *Compute) Validate() error {
	if err := parseInt(c.Instances); err != nil {
		return fmt.Errorf("instances: '%s' is not an integer: %s", c.Instances, err)
	}
	if c.Instances == "" {
		c.Instances = "1"
	}
	return nil
}

// --------------------------------------------------------------------------

type Trx struct {
	Name string
	File string
	Data map[string]Data
}

func (c *Trx) Vars(params map[string]string) error {
	var err error
	c.Name, err = Vars(c.Name, params, false)
	if err != nil {
		return err
	}
	c.File, err = Vars(c.File, params, false)
	if err != nil {
		return err
	}
	for k := range c.Data {
		d := c.Data[k]
		if err := d.Vars(params); err != nil {
			return err
		}
		c.Data[k] = d
	}
	return nil
}

type Data struct {
	Name      string            `yaml:"name"`      // @id
	Generator string            `yaml:"generator"` // data.Generator type
	Scope     string            `yaml:"scope"`
	Params    map[string]string `yaml:"params"` // Generator-specific params
}

func (c *Data) Vars(params map[string]string) error {
	var err error
	c.Name, err = Vars(c.Name, params, false)
	if err != nil {
		return err
	}
	c.Generator, err = Vars(c.Generator, params, false)
	if err != nil {
		return err
	}
	c.Scope, err = Vars(c.Scope, params, false)
	if err != nil {
		return err
	}
	for k, v := range c.Params {
		c.Params[k], err = Vars(v, params, true)
		if err != nil {
			return err
		}
	}
	return nil
}

// --------------------------------------------------------------------------

type ClientGroup struct {
	Clients       string   `yaml:"clients,omitempty"` // uint
	Db            string   `yaml:"db,omitempty"`
	DisableStats  bool     `yaml:"disable-stats,omitempty"`
	Iter          string   `yaml:"iter,omitempty"`            // uint
	IterClients   string   `yaml:"iter-clients,omitempty"`    // uint
	IterExecGroup string   `yaml:"iter-exec-group,omitempty"` // uint
	Group         string   `yaml:"group,omitempty"`
	QPS           string   `yaml:"qps,omitempty"`            // uint
	QPSClients    string   `yaml:"qps-clients,omitempty"`    // uint
	QPSExecGroup  string   `yaml:"qps-exec-group,omitempty"` // uint
	Runtime       string   `yaml:"runtime,omitempty"`
	TPS           string   `yaml:"tps,omitempty"`
	TPSClients    string   `yaml:"tps-clients,omitempty"`
	TPSExecGroup  string   `yaml:"tps-exec-group,omitempty"`
	Trx           []string `yaml:"trx,omitempty"`
}

func (c *ClientGroup) Validate(w []Trx) error {
	if err := parseInt(c.Clients); err != nil {
		return fmt.Errorf("client: '%s' is not an integer: %s", c.Clients, err)
	}
	if c.Clients == "" {
		c.Clients = "1"
	}

	if err := parseInt(c.Iter); err != nil {
		return fmt.Errorf("iter: '%s' is not an integer: %s", c.Iter, err)
	}
	if err := parseInt(c.IterClients); err != nil {
		return fmt.Errorf("iter-clients: '%s' is not an integer: %s", c.IterClients, err)
	}
	if err := parseInt(c.IterExecGroup); err != nil {
		return fmt.Errorf("iter-exec-group: '%s' is not an integer: %s", c.IterExecGroup, err)
	}

	if err := parseInt(c.QPS); err != nil {
		return fmt.Errorf("iter: '%s' is not an integer: %s", c.QPS, err)
	}
	if err := parseInt(c.QPSClients); err != nil {
		return fmt.Errorf("iter-clients: '%s' is not an integer: %s", c.QPSClients, err)
	}
	if err := parseInt(c.QPSExecGroup); err != nil {
		return fmt.Errorf("iter-exec-group: '%s' is not an integer: %s", c.QPSExecGroup, err)
	}

	if err := parseInt(c.TPS); err != nil {
		return fmt.Errorf("tps: '%s' is not an integer: %s", c.TPS, err)
	}
	if err := parseInt(c.TPSClients); err != nil {
		return fmt.Errorf("tps-clients: '%s' is not an integer: %s", c.TPSClients, err)
	}
	if err := parseInt(c.TPSExecGroup); err != nil {
		return fmt.Errorf("tps-exec-group: '%s' is not an integer: %s", c.TPSExecGroup, err)
	}

	if err := ValidFreq(c.Runtime, "workload.runtime"); err != nil {
		return err
	}
	return nil
}

func (c *ClientGroup) Vars(params map[string]string) error {
	var err error
	c.Db, err = Vars(c.Db, params, false)
	if err != nil {
		return err
	}
	c.Clients, err = Vars(c.Clients, params, true)
	if err != nil {
		return err
	}
	c.QPS, err = Vars(c.QPS, params, true)
	if err != nil {
		return err
	}
	c.QPSClients, err = Vars(c.QPSClients, params, true)
	if err != nil {
		return err
	}
	c.QPSExecGroup, err = Vars(c.QPSExecGroup, params, true)
	if err != nil {
		return err
	}
	c.TPS, err = Vars(c.TPS, params, true)
	if err != nil {
		return err
	}
	c.TPSClients, err = Vars(c.TPSClients, params, true)
	if err != nil {
		return err
	}
	c.TPSExecGroup, err = Vars(c.TPSExecGroup, params, true)
	if err != nil {
		return err
	}
	c.Runtime, err = Vars(c.Runtime, params, false)
	if err != nil {
		return err
	}
	c.Group, err = Vars(c.Group, params, false)
	if err != nil {
		return err
	}
	for i := range c.Trx {
		c.Trx[i], err = Vars(c.Trx[i], params, false)
		if err != nil {
			return err
		}
	}
	return nil
}

// --------------------------------------------------------------------------

type MySQL struct {
	Db             string `yaml:"db,omitempty"`
	DSN            string `yaml:"dsn,omitempty"`
	Hostname       string `yaml:"hostname,omitempty"`
	MyCnf          string `yaml:"mycnf,omitempty"`
	Password       string `yaml:"password,omitempty"`
	PasswordFile   string `yaml:"password-file,omitempty"`
	Socket         string `yaml:"socket,omitempty"`
	TimeoutConnect string `yaml:"timeout-connect,omitempty"`
	TLS            TLS    `yaml:"tls,omitempty"`
	Username       string `yaml:"username,omitempty"`

	DisableAutoTLS *bool `yaml:"disable-auto-tls,omitempty"`
}

// With returns the MySQL config c with defaults from def. It's called in
// dbconn/factory.setDSN to apply any defaults from MySQL.MyCnf (a my.cnf
// defaults file), which mimics how MySQL works.
func (c *MySQL) With(def MySQL) {
	if c.Db == "" {
		c.Db = def.Db
	}
	if c.DSN == "" {
		c.DSN = def.DSN
	}
	if c.Hostname == "" {
		c.Hostname = def.Hostname
	}
	if c.MyCnf == "" && def.MyCnf != "" {
		c.MyCnf = def.MyCnf
	}
	if c.Password == "" && def.Password != "" {
		c.Password = def.Password
	}
	if c.PasswordFile == "" && def.PasswordFile != "" {
		c.PasswordFile = def.PasswordFile
	}
	if c.Socket == "" {
		c.Socket = def.Socket
	}
	if c.TimeoutConnect == "" && def.TimeoutConnect != "" {
		c.TimeoutConnect = def.TimeoutConnect
	}
	if c.Username == "" && def.Username != "" {
		c.Username = def.Username
	}
	c.DisableAutoTLS = setBool(c.DisableAutoTLS, def.DisableAutoTLS)
	c.TLS.With(def.TLS)
}

func (c *MySQL) Vars(params map[string]string) error {
	var err error
	c.Db, err = Vars(c.Db, params, false)
	if err != nil {
		return err
	}
	c.DSN, err = Vars(c.DSN, params, false)
	if err != nil {
		return err
	}
	c.MyCnf, err = Vars(c.MyCnf, params, false)
	if err != nil {
		return err
	}
	c.Username, err = Vars(c.Username, params, false)
	if err != nil {
		return err
	}
	c.Password, err = Vars(c.Password, params, false)
	if err != nil {
		return err
	}
	c.PasswordFile, err = Vars(c.PasswordFile, params, false)
	if err != nil {
		return err
	}
	c.TimeoutConnect, err = Vars(c.TimeoutConnect, params, false)
	if err != nil {
		return err
	}
	if err := c.TLS.Vars(params); err != nil {
		return err
	}
	return nil
}

func (c *MySQL) Validate() error {
	return nil
}

func (c MySQL) Redacted() string {
	if c.Password != "" {
		c.Password = "..."
	}
	return fmt.Sprintf("%+v", c)
}

// --------------------------------------------------------------------------

type TLS struct {
	CA         string `yaml:"ca,omitempty"`   // ssl-ca
	Cert       string `yaml:"cert,omitempty"` // ssl-cert
	Key        string `yaml:"key,omitempty"`  // ssl-key
	SkipVerify *bool  `yaml:"skip-verify,omitempty"`
	Disable    *bool  `yaml:"disable,omitempty"`

	// ssl-mode from a my.cnf (see dbconn.ParseMyCnf)
	MySQLMode string `yaml:"-"`
}

func (c *TLS) With(def TLS) {
	if c.Cert == "" {
		c.Cert = def.Cert
	}
	if c.Key == "" {
		c.Key = def.Key
	}
	if c.CA == "" {
		c.CA = def.CA
	}
	if c.MySQLMode == "" {
		c.MySQLMode = def.MySQLMode
	}
	c.SkipVerify = setBool(c.SkipVerify, def.SkipVerify)
	c.Disable = setBool(c.Disable, def.Disable)
}

func (c *TLS) Validate() error {
	if True(c.Disable) || (c.Cert == "" && c.Key == "" && c.CA == "") {
		return nil // no TLS
	}

	// Any files specified must exist
	if c.CA != "" && !FileExists(c.CA) {
		return fmt.Errorf("config.tls.ca: %s: file does not exist", c.CA)
	}
	if c.Cert != "" && !FileExists(c.Cert) {
		return fmt.Errorf("config.tls.cert: %s: file does not exist", c.Cert)
	}
	if c.Key != "" && !FileExists(c.Key) {
		return fmt.Errorf("config.tls.key: %s: file does not exist", c.Key)
	}

	// The three valid combination of files:
	if c.CA != "" && (c.Cert == "" && c.Key == "") {
		return nil // ca (only) e.g. Amazon RDS CA
	}
	if c.Cert != "" && c.Key != "" {
		return nil // cert + key (using system CA)
	}
	if c.CA != "" && c.Cert != "" && c.Key != "" {
		return nil // ca + cert + key (private CA)
	}

	if c.Cert == "" && c.Key != "" {
		return fmt.Errorf("config.tls: missing cert (cert and key are mutually dependent)")
	}
	if c.Cert != "" && c.Key == "" {
		return fmt.Errorf("config.tls: missing key (cert and key are mutually dependent)")
	}

	return fmt.Errorf("config.tls: invalid combination of files: %+v; valid combinations are: ca; cert and key; ca, cert, and key", c)
}

func (c *TLS) Vars(params map[string]string) error {
	var err error
	c.Cert, err = Vars(c.Cert, params, false)
	if err != nil {
		return err
	}
	c.Key, err = Vars(c.Key, params, false)
	if err != nil {
		return err
	}
	c.CA, err = Vars(c.CA, params, false)
	if err != nil {
		return err
	}
	return nil
}

// Set return true if TLS is not disabled and at least one file is specified.
// If not set, Blip ignores the TLS config. If set, Blip validates, loads, and
// registers the TLS config.
func (c TLS) Set() bool {
	return !True(c.Disable) && c.MySQLMode != "DISABLED" && (c.CA != "" || c.Cert != "" || c.Key != "")
}

func (c TLS) LoadTLS(server string) (*tls.Config, error) {
	//  WARNING: Validate must be called first!
	if !c.Set() {
		return nil, nil
	}

	// Either ServerName or InsecureSkipVerify is required else Go will
	// return an error saying that. If both are set, Go seems to ignore
	// ServerName.
	tlsConfig := &tls.Config{
		ServerName:         server,
		InsecureSkipVerify: True(c.SkipVerify),
	}

	// Root CA (optional)
	if c.CA != "" {
		caCert, err := ioutil.ReadFile(c.CA)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	// Cert and key
	if c.Cert != "" && c.Key != "" {
		cert, err := tls.LoadX509KeyPair(c.Cert, c.Key)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		tlsConfig.BuildNameToCertificate()
	}

	return tlsConfig, nil
}

// --------------------------------------------------------------------------

type Stats struct {
	Disable *bool                        `yaml:"disable"`
	Freq    string                       `yaml:"freq,omitempty"`
	Report  map[string]map[string]string `yaml:"report,omitempty"`
}

func (c *Stats) Validate() error {
	if c.Freq == "" {
		c.Freq = "0s" // one report for the entire runtime
	} else {
		if err := ValidFreq(c.Freq, "stats.freq"); err != nil {
			return err
		}
	}
	if len(c.Report) == 0 {
		c.Report = map[string]map[string]string{
			"stdout": {"each-instance": "true"},
		}
	} else {
		// Prevent "panic: assignment to entry in nil map", so reporters can trust
		// that their opts arg is a map, not nil
		for k, v := range c.Report {
			if v != nil {
				continue
			}
			c.Report[k] = map[string]string{}
		}
	}
	return nil
}

func (c *Stats) Vars(params map[string]string) error {
	var err error
	c.Freq, err = Vars(c.Freq, params, false)
	if err != nil {
		return err
	}
	for _, r := range c.Report {
		for k, v := range r {
			r[k], err = Vars(v, params, false)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

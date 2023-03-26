// Copyright 2022 Block, Inc.

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// File is the config file format including all sub-sections.
type File struct {
	MySQL   MySQL   `yaml:"mysql,omitempty"`
	Compute Compute `yaml:"compute,omitempty"`
	Stats   Stats   `yaml:"stats,omitempty"`

	Setup     Stage `yaml:"setup,omitempty"`
	Warmup    Stage `yaml:"warmup,omitempty"`
	Benchmark Stage `yaml:"benchmark,omitempty"`
	Cleanup   Stage `yaml:"cleanup,omitempty"`
}

func (c *File) Validate() error {
	if err := c.MySQL.Validate(); err != nil {
		return err
	}
	if err := c.Compute.Validate(); err != nil {
		return err
	}
	if err := c.Stats.Validate(); err != nil {
		return err
	}

	// Stages
	if err := c.Setup.Validate("setup"); err != nil {
		return err
	}
	if err := c.Warmup.Validate("warmup"); err != nil {
		return err
	}
	if err := c.Benchmark.Validate("benchmark"); err != nil {
		return err
	}
	if err := c.Cleanup.Validate("cleanup"); err != nil {
		return err
	}
	return nil
}

// --------------------------------------------------------------------------

type Stage struct {
	Name    string
	Runtime string `yaml:"runtime,omitempty"`
	Script  string `yaml:"script,omitempty"`
	QPS     uint   `yaml:"qps,omitempty"`
	TPS     uint   `yaml:"tps,omitempty"`

	Exec     string      `yaml:"exec,omitempty"`
	Workload []ExecGroup `yaml:"workload,omitempty"`
	Trx      []Trx       `yaml:"trx,omitempty"`

	Disable               bool `yaml:"disable"`
	DisableAutoAllocation bool `yaml:"disable-auto-allocation,omitempty"`
}

type Trx struct {
	Name string
	File string
	Data map[string]Data
}

func (c *Stage) Validate(name string) error {
	c.Name = name // used for logging
	if c.Disable {
		return nil
	}

	if len(c.Trx) == 0 && len(c.Workload) == 0 {
		c.Disable = true
		return nil
	}

	if len(c.Trx) == 0 {
		return fmt.Errorf("stage %s has zero trx files and is not disabled; specify at least 1 trx file or %s.disable = true", name, name)
	}

	// Trx list: must validate before Workload because Workload reference trx by name
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
	}

	// Workload
	for i := range c.Workload {
		if err := c.Workload[i].Validate(c.Trx); err != nil {
			return err
		}
	TRX:
		for _, name := range c.Workload[i].Trx {
			for i := range c.Trx {
				if c.Trx[i].Name == name {
					continue TRX
				}
			}
			return fmt.Errorf("client group %d specifies nonexistent workload: %s", i, name)
		}
	}

	// Runtime
	if err := ValidFreq(c.Runtime, "workload"); err != nil {
		return err
	}

	// Exec
	switch c.Exec {
	case "sequential":
	case "concurrenct":
	case "": // Auto-set
		if c.Name == "setup" || c.Name == "cleanup" {
			c.Exec = "sequential"
		} else {
			c.Exec = "concurrent"
		}
	default:
		return fmt.Errorf("invalid exec: %s (valid values: sequential, concurrent)", c.Exec)
	}

	// Script
	// @todo

	return nil
}

type Data struct {
	Name      string            `yaml:"name"`      // @id
	Generator string            `yaml:"generator"` // data.Generator type
	Scope     string            `yaml:"scope"`
	DataType  string            `yaml:"data-type"`
	Params    map[string]string `yaml:"params"` // Generator-specific params
}

type ExecGroup struct {
	Clients       uint     `yaml:"clients,omitempty"`
	Db            string   `yaml:"db,omitempty"`
	Iter          uint     `yaml:"iter,omitempty"`
	IterClients   uint32   `yaml:"iter-clients,omitempty"`
	IterExecGroup uint32   `yaml:"iter-exec-group,omitempty"`
	Name          string   `yaml:"name,omitempty"`
	QPS           uint     `yaml:"qps,omitempty"`
	QPSClients    uint     `yaml:"qps-clients,omitempty"`
	QPSExecGroup  uint     `yaml:"qps-exec-group,omitempty"`
	Runtime       string   `yaml:"runtime,omitempty"`
	TPS           uint     `yaml:"tps,omitempty"`
	TPSClients    uint     `yaml:"tps-clients,omitempty"`
	TPSExecGroup  uint     `yaml:"tps-exec-group,omitempty"`
	Trx           []string `yaml:"trx,omitempty"`
}

func (c *ExecGroup) Validate(w []Trx) error {
	if c.Clients == 0 {
		c.Clients = 1
	}
	if err := ValidFreq(c.Runtime, "workload.runtime"); err != nil {
		return err
	}
	return nil
}

// --------------------------------------------------------------------------

type MySQL struct {
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

func (c *MySQL) Validate() error {
	return nil
}

func (c *MySQL) ApplyDefaults(d MySQL) {
	if c.Socket == "" {
		c.Socket = d.Socket
	}
	if c.Hostname == "" {
		c.Hostname = d.Hostname
	}
	if c.MyCnf == "" {
		c.MyCnf = d.MyCnf
	}
	if c.Username == "" {
		c.Username = d.Username
	}
	if c.Password == "" {
		c.Password = d.Password
	}
	if c.PasswordFile == "" {
		c.PasswordFile = d.Password
	}
	if c.TimeoutConnect == "" {
		c.TimeoutConnect = d.TimeoutConnect
	}

	if c.TLS.Cert == "" {
		c.TLS.Cert = d.TLS.Cert
	}
	if c.TLS.Key == "" {
		c.TLS.Key = d.TLS.Key
	}
	if c.TLS.CA == "" {
		c.TLS.CA = d.TLS.CA
	}
}

func (c *MySQL) InterpolateEnvVars() {
	c.MyCnf = interpolateEnv(c.MyCnf)
	c.Username = interpolateEnv(c.Username)
	c.Password = interpolateEnv(c.Password)
	c.PasswordFile = interpolateEnv(c.PasswordFile)
	c.TimeoutConnect = interpolateEnv(c.TimeoutConnect)
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

func (c *TLS) InterpolateEnvVars() {
	c.Cert = interpolateEnv(c.Cert)
	c.Key = interpolateEnv(c.Key)
	c.CA = interpolateEnv(c.CA)
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

const DEFAULT_BIND = ":33075"

type Compute struct {
	Name string `yaml:"name,omitempty"`

	// Local coordinator
	Bind                string `yaml:"bind,omitempty"`
	DisableLocal        *bool  `yaml:"disable-local,omitempty"`
	DisableFileTransfer *bool  `yaml:"disable-file-transfer,omitempty"`
	Instances           uint   `yaml:"instances,omitempty"`

	// Remote client
	Server string `yaml:"server,omitempty"`
}

func (c *Compute) Validate() error {
	if c.Name == "" {
		c.Name, _ = os.Hostname()
	}
	if c.Server != "" {
		// Remote client
		if c.Bind != "" {
			return fmt.Errorf("compute.server and compute.bind are mutually exclusive: compute.server enables a remote instance; compute.bind sets the address of a local coordinator")
		}
		if True(c.DisableLocal) {
			return fmt.Errorf("compute.server and compute.disable-local = true are mutually exclusive because it equals zero instances")
		}
		if c.Instances > 0 {
			return fmt.Errorf("compute.server and compute.instances > 0 are mutually exclusive: compute.server enables a remote instance; compute.instances enables a local coordinator")
		}
		if !strings.HasPrefix(c.Server, "http") {
			c.Server = "http://" + c.Server
		}
	} else {
		// Local instance
		if c.Instances == 0 {
			c.Instances = 1
		}
		/*
			if c.Instances <= 1 && True(c.DisableLocal) {
				return fmt.Errorf("compute.instances <= 1 and compute.disable-local = true are mutually exclusive because it equals zero instances")
			}
		*/
		if c.Bind == "" {
			c.Bind = DEFAULT_BIND
		}
	}
	return nil
}

// --------------------------------------------------------------------------

type Stats struct {
	Freq   string                       `yaml:"freq,omitempty"`
	Report map[string]map[string]string `yaml:"report,omitempty"`
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
			"stdout": {}, // defaults in stats/stdout.go
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

func (c *Stats) InterpolateEnvVars() {
	c.Freq = interpolateEnv(c.Freq)
}

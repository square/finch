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
	Name     string
	Clients  []ClientGroup `yaml:"clients,omitempty"`
	Runtime  string        `yaml:"runtime,omitempty"`
	Sequence string        `yaml:"sequence,omitempty"`
	Script   string        `yaml:"script,omitempty"`
	Workload []Trx         `yaml:"workload,omitempty"`
	QPS      uint64        `yaml:"qps,omitempty"`
	TPS      uint64        `yaml:"tps,omitempty"`

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

	// Workload: must validate before Clients because Clients reference trx sets by nameo
	for i := range c.Workload {
		if c.Workload[i].File == "" {
			return fmt.Errorf("no file specified for workload transaction set %d", i)
		}
		if !FileExists(c.Workload[i].File) {
			return fmt.Errorf("workload transaction set %d file %s does not exist", i, c.Workload[i].File)
		}
		if c.Workload[i].Name == "" {
			c.Workload[i].Name = filepath.Base(c.Workload[i].File)
		}
	}

	// Clients
	if len(c.Clients) > 0 {
		for i := range c.Clients {
			if err := c.Clients[i].Validate(c.Workload); err != nil {
				return err
			}
		CLIENT_TRX:
			for _, name := range c.Clients[i].Transactions {
				for i := range c.Workload {
					if c.Workload[i].Name == name {
						continue CLIENT_TRX
					}
				}
				return fmt.Errorf("client group %d specifies nonexistent workload: %s", i, name)
			}
		}
	}

	// Runtime
	if err := ValidFreq(c.Runtime, "workload"); err != nil {
		return err
	}

	// Sequence
	switch c.Sequence {
	case "client": // alt value
		c.Sequence = "clients" // normalize
	case "clients":
	case "workload":
	case "":
		// Auto-set
		if c.Name == "setup" || c.Name == "cleanup" {
			c.Sequence = "workload"
		}
	default:
		return fmt.Errorf("invalid sequence: %s (valid values: clients, workload)", c.Sequence)
	}

	// Script
	// @todo

	return nil
}

type Data struct {
	Name      string            // @id
	Generator string            // data.Generator type
	Params    map[string]string // Generator-specific params
}

type ClientGroup struct {
	N            uint     `yaml:"n,omitempty"`
	Transactions []string `yaml:"transactions,omitempty"`
	Runtime      string   `yaml:"runtime,omitempty"`
	Iterations   uint64   `yaml:"iterations,omitempty"`
	QPS          uint64   `yaml:"qps,omitempty"`
	TPS          uint64   `yaml:"tps,omitempty"`
}

func (c *ClientGroup) Validate(w []Trx) error {
	if c.N == 0 {
		c.N = 1
	}
	if err := ValidFreq(c.Runtime, "workload.clients"); err != nil {
		return err
	}
	if len(c.Transactions) == 0 {
		names := make([]string, len(w))
		for i := range w {
			names[i] = w[i].Name
		}
		c.Transactions = names
	}
	return nil
}

func (cg ClientGroup) Name() string {
	return strings.Join(cg.Transactions, ",")
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
			"stdout": {
				"percentiles": "99,99.9",
			},
		}
	}
	return nil
}

func (c *Stats) InterpolateEnvVars() {
	c.Freq = interpolateEnv(c.Freq)
}

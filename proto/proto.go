// Copyright 2022 Block, Inc.

package proto

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/square/finch"
)

var RetryWait = 500 * time.Millisecond
var MaxTries = 100

type Client struct {
	addr   string
	name   string
	client *http.Client
}

func NewClient(client, server string) Client {
	return Client{
		name:   client,
		addr:   server,
		client: finch.MakeHTTPClient(),
	}
}

func (c Client) Send(data interface{}, endpoint string) error {
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(data)
	req, _ := http.NewRequest("POST", c.URL(endpoint, nil), buf)
	_, err := c.client.Do(req)
	return err
}

func (c Client) URL(path string, params [][]string) string {
	// Every request requires 'name=...' to tell server this client's name.
	// It's not a hostname, just a user-defined name for the remote compute instance.
	u := c.addr + path + "?name=" + url.QueryEscape(c.name)
	if len(params) > 0 {
		escaped := make([]string, len(params))
		for i := range params {
			escaped[i] = params[i][0] + "=" + url.QueryEscape(params[i][1])
		}
		u += strings.Join(escaped, "&")
	}
	finch.Debug("%s", u)
	return u
}

func (c Client) Error(err error) {
	errString := bytes.NewBufferString(err.Error())
	printErr := true
	for i := 0; i < MaxTries; i++ {
		_, err := c.client.Post(c.URL("/error", nil), "text/plain", errString)
		if err == nil {
			return // success
		}
		if printErr {
			log.Println(err)
			printErr = false // don't spam output
		}
		time.Sleep(RetryWait)
	}
}

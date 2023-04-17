// Copyright 2023 Block, Inc.

package proto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/square/finch"
)

var DefaultRetryWait = 500 * time.Millisecond
var DefaultMaxTries = 100

var ErrFailed = errors.New("request failed after attempts, or context cancelled")

type Client struct {
	addr      string
	name      string
	client    *http.Client
	RetryWait time.Duration
	MaxTries  int
	errNo     int
}

func NewClient(client, server string) *Client {
	return &Client{
		name:      client,
		addr:      server,
		client:    finch.MakeHTTPClient(),
		RetryWait: DefaultRetryWait,
		MaxTries:  DefaultMaxTries,
	}
}

func (c *Client) reset() {
	c.errNo = 0
}

func (c *Client) wait(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if c.errNo%100 == 0 {
		log.Println(err)
	}
	c.errNo++
	time.Sleep(c.RetryWait)
	return true
}

func (c *Client) Get(ctx context.Context, endpoint string, params [][]string) (*http.Response, []byte, error) {
	defer c.reset()
	for i := 0; i < c.MaxTries; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", c.URL(endpoint, params), nil)
		resp, err := c.client.Do(req)
		if err != nil {
			if c.wait(err) {
				continue
			}
			return nil, nil, ErrFailed
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}
		if resp.StatusCode >= 500 {
			err := fmt.Errorf("server error: %s: %s", resp.Status, string(body))
			if c.wait(err) {
				continue
			}
			return nil, nil, ErrFailed
		}
		if resp.StatusCode >= 400 {
			return nil, nil, fmt.Errorf("client error: %s: %s", resp.Status, string(body))
		}
		return resp, body, nil // success
	}
	return nil, nil, ErrFailed
}

func (c *Client) Send(ctx context.Context, endpoint string, data interface{}) error {
	defer c.reset()
	buf := new(bytes.Buffer)
	if data != nil {
		json.NewEncoder(buf).Encode(data)
	}
	for i := 0; i < c.MaxTries; i++ {
		req, _ := http.NewRequestWithContext(ctx, "POST", c.URL(endpoint, nil), buf)
		resp, err := c.client.Do(req)
		if err != nil {
			if c.wait(err) {
				continue
			}
			return ErrFailed
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 500 {
			err := fmt.Errorf("server error: %s: %s", resp.Status, string(body))
			if c.wait(err) {
				continue
			}
			return ErrFailed
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("client error: %s: %s", resp.Status, string(body))
		}
		return nil // success
	}
	return ErrFailed
}

func (c *Client) Error(err error) {
	defer c.reset()
	errString := bytes.NewBufferString(err.Error())
	for i := 0; i < c.MaxTries; i++ {
		_, err := c.client.Post(c.URL("/error", nil), "text/plain", errString)
		if err != nil {
			if c.wait(err) {
				continue
			}
		}
		return
	}
}

func (c *Client) URL(path string, params [][]string) string {
	// Every request requires 'name=...' to tell server this client's name.
	// It's not a hostname, just a user-defined name for the remote compute instance.
	u := c.addr + path + "?name=" + url.QueryEscape(c.name)
	if len(params) > 0 {
		escaped := make([]string, len(params))
		for i := range params {
			escaped[i] = params[i][0] + "=" + url.QueryEscape(params[i][1])
		}
		u += "&" + strings.Join(escaped, "&")
	}
	finch.Debug("%s", u)
	return u
}

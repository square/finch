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

var ErrFailed = errors.New("request failed after attempts, or context cancelled")

type Client struct {
	name       string
	serverAddr string
	// --
	client  *http.Client
	errNo   int
	StageId string
}

func NewClient(name, server string) *Client {
	return &Client{
		name:       name,
		serverAddr: server,
		// --
		client: finch.MakeHTTPClient(),
	}
}

func (c *Client) reset() {
	c.errNo = 0
}

// retry is called for all errors and returns true if the caller should continue
// try, else it returns false and the caller should return.
func (c *Client) retry(ctx context.Context, err error, wait time.Duration) bool {
	if ctx.Err() != nil {
		return false // stop trying; parent context cancelled
	}
	finch.Debug("%v", err)
	if c.errNo%20 == 0 {
		log.Printf("Request error, retrying: %v", err)
	}
	c.errNo++
	time.Sleep(wait)
	return true // keep trying
}

func (c *Client) Get(ctx context.Context, endpoint string, params [][]string, tries int, wait time.Duration) (*http.Response, []byte, error) {
	defer c.reset()
	url := c.URL(endpoint, params)
	finch.Debug("GET %s", url)
	var err error
	var body []byte
	var req *http.Request
	var resp *http.Response
	retry := true
	for i := 0; retry && (tries == -1 || i < tries); i++ {
		ctxReq, cancelReq := context.WithTimeout(ctx, 2*time.Second)
		req, _ = http.NewRequestWithContext(ctxReq, "GET", url, nil)
		resp, err = c.client.Do(req)
		cancelReq()
		if err != nil {
			goto RETRY
		}

		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, nil, err
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return resp, body, nil // success
		case http.StatusResetContent:
			return resp, nil, nil // reset
		default:
			finch.Debug("unknown http response: %v", resp)
			goto RETRY
		}

	RETRY:
		retry = c.retry(ctx, err, wait)
	}
	return nil, nil, ErrFailed
}

func (c *Client) Send(ctx context.Context, endpoint string, data interface{}, tries int, wait time.Duration) error {
	defer c.reset()
	url := c.URL(endpoint, nil)
	finch.Debug("POST %s", url)

	buf := new(bytes.Buffer)
	if data != nil {
		json.NewEncoder(buf).Encode(data)
	}

	var err error
	var body []byte
	var req *http.Request
	var resp *http.Response
	retry := true
	for i := 0; retry && (tries == -1 || i < tries); i++ {
		ctxReq, cancelReq := context.WithTimeout(ctx, 2*time.Second)
		req, _ = http.NewRequestWithContext(ctxReq, "POST", url, buf)
		resp, err = c.client.Do(req)
		cancelReq()
		if err != nil {
			goto RETRY
		}

		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if resp.StatusCode >= 500 {
			err = fmt.Errorf("server error: %s: %s", resp.Status, string(body))
			goto RETRY
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("client error: %s: %s", resp.Status, string(body))
		}
		return nil // success

	RETRY:
		retry = c.retry(ctx, err, wait)
	}
	return ErrFailed
}

func (c *Client) URL(path string, params [][]string) string {
	// Every request requires 'name=...' to tell server this client's name.
	// It's not a hostname, just a user-defined name for the remote compute instance.
	u := c.serverAddr + path + "?name=" + url.QueryEscape(c.name)
	n := len(params) + 1
	escaped := make([]string, len(params)+1)
	for i := range params {
		escaped[i] = params[i][0] + "=" + url.QueryEscape(params[i][1])
	}
	escaped[n-1] = "stage-id=" + c.StageId
	u += "&" + strings.Join(escaped, "&")
	return u
}

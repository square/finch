// Copyright 2023 Block, Inc.

package proto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/square/finch"
)

var ErrFailed = errors.New("request failed after attempts, or context cancelled")

type R struct {
	Timeout time.Duration
	Wait    time.Duration
	Tries   int
}

type Client struct {
	name       string
	serverAddr string
	// --
	client      *http.Client
	StageId     string
	PrintErrors bool
}

func NewClient(name, server string) *Client {
	return &Client{
		name:       name,
		serverAddr: server,
		// --
		client: finch.MakeHTTPClient(),
	}
}

func (c *Client) Get(ctx context.Context, endpoint string, params [][]string, r R) (*http.Response, []byte, error) {
	return c.request(ctx, "GET", endpoint, params, nil, r)
}

func (c *Client) Send(ctx context.Context, endpoint string, data interface{}, r R) error {
	_, _, err := c.request(ctx, "POST", endpoint, nil, data, r)
	return err
}

func (c *Client) request(ctx context.Context, method string, endpoint string, params [][]string, data interface{}, r R) (*http.Response, []byte, error) {
	url := c.URL(endpoint, params)
	finch.Debug("%s %s", method, url)

	buf := new(bytes.Buffer)
	if data != nil {
		json.NewEncoder(buf).Encode(data)
	}

	var err error
	var body []byte
	var req *http.Request
	var resp *http.Response
	try := 0
	for r.Tries == -1 || try < r.Tries {
		try += 1
		ctxReq, cancelReq := context.WithTimeout(ctx, r.Timeout)
		req, _ = http.NewRequestWithContext(ctxReq, method, url, buf)
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
			goto RETRY
		}

	RETRY:
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		finch.Debug("%v", err)
		if c.PrintErrors && try%20 == 0 {
			log.Printf("Request error, retrying: %v", err)
		}
		time.Sleep(r.Wait)
	}
	return nil, nil, ErrFailed
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

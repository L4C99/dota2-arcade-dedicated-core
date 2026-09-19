// Package client calls d2core's authenticated same-user local protocol.
// It never starts a manager or server and never creates a new retry key.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/localipc"
)

type Client struct{ dataDir string }
type Error struct {
	Code        string `json:"code"`
	Stage       string `json:"stage"`
	Message     string `json:"message"`
	InstanceID  string `json:"instanceId,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}

func (e *Error) Error() string { return e.Code + " (" + e.Stage + "): " + e.Message }

func New(dataDir string) (*Client, error) {
	if !filepath.IsAbs(dataDir) {
		return nil, fmt.Errorf("absolute data-dir required")
	}
	return &Client{filepath.Clean(dataDir)}, nil
}

// Call returns transport errors separately from structured *Error rejections.
// Context cancellation means the outcome may be unknown, not that a previously
// accepted operation was cancelled. Retry create with the original key/params.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	switch method {
	case "create", "list", "status", "operation", "logs", "restart", "stop":
	default:
		return nil, fmt.Errorf("unsupported local method %q", method)
	}
	if params == nil {
		params = struct{}{}
	}
	b, e := json.Marshal(map[string]any{"protocolVersion": 1, "method": method, "params": params})
	if e != nil {
		return nil, e
	}
	b, e = localipc.Call(ctx, c.dataDir, b)
	if e != nil {
		return nil, e
	}
	var r struct {
		ProtocolVersion int             `json:"protocolVersion"`
		OK              bool            `json:"ok"`
		Result          json.RawMessage `json:"result,omitempty"`
		Error           *Error          `json:"error,omitempty"`
	}
	if e = config.DecodeStrict(b, &r); e != nil {
		return nil, e
	}
	if r.ProtocolVersion != 1 {
		return nil, fmt.Errorf("unsupported response protocol version")
	}
	if !r.OK {
		if r.Error == nil || r.Error.Code == "" {
			return nil, fmt.Errorf("invalid error response")
		}
		return nil, r.Error
	}
	if r.Error != nil || len(r.Result) == 0 {
		return nil, fmt.Errorf("invalid success response")
	}
	return r.Result, nil
}

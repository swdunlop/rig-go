// Package protocol defines the wire protocol for a subset of JSON-RPC 2.0
// with the ambiguities of IDs and parameters removed.
package protocol

import (
	"encoding/json"
)

// A Request is a message sent from a client to a service.
type Request struct {
	ID     string          `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// A Response is a message sent from a service to a client in response to a
// request that had an ID.
type Response struct {
	ID     string `json:"id"`
	Result any    `json:"result"`
	Error  *Error `json:"error"`
	End    bool   `json:"end"`
}

// A Notification is a message sent from a client to a service in JSON-RPC 2.0
// without an ID.  We also support sending notifications back to the client
// which is not normally tolerated by JSON-RPC 2.0 clients.
type Notification struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

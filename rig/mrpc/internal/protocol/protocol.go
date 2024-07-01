// Package protocol defines the wire protocol for our MessagePack-based RPC system.
package protocol

import "github.com/tinylib/msgp/msgp"

//go:generate go run github.com/tinylib/msgp
//msgp:tuple Request Fail
//msgp:ignore Response

// A Request is a message sent from a client to a server.
type Request struct {
	// ID is a unique identifier for this request used to coordinate responses.
	ID string

	// Method is currently one of "call" or "start" but may be used for other purposes in the future.
	Method string

	// Function is the name of the function to call or start.  This may be an empty string if unused by other
	// methods.
	Function string

	// Input contains the result of the request, which may be nil.  The actual underlying type depends on the
	// method and function.
	Input msgp.Raw
}

// A Response is a message sent from a server to a client.
type Response struct {
	// ID is the ID of the request to which this is a response.
	ID string

	// Method is currently one of "succ", "fail" or "yield" but may be used for other purposes in the future.
	Method string

	// Output contains the result of the request, which may be nil.  The actual underlying type depends on the method.
	// For a "succ" or "yield" the type actually depends on the function.
	// For "fail" it is a Fail and for "end" it is a nil.
	Output msgp.MarshalSizer
}

// Msgsize implements msgp.MarshalSizer
func (r *Response) Msgsize() int {
	return msgp.ArrayHeaderSize +
		msgp.StringPrefixSize + len(r.Method) +
		msgp.StringPrefixSize + len(r.ID) +
		r.Output.Msgsize()
}

// MarshalMsg implements msgp.Marshaler
func (rs *Response) MarshalMsg(b []byte) ([]byte, error) {
	b = msgp.AppendArrayHeader(b, 3)
	b = msgp.AppendString(b, rs.ID)
	b = msgp.AppendString(b, rs.Method)
	if rs.Output == nil {
		b = msgp.AppendNil(b)
	} else {
		return rs.Output.MarshalMsg(b)
	}
	return b, nil
}

// A Fail is a response that indicates an error occurred.
type Fail struct {
	Code int // Status code, generally analogous to HTTP status codes.
	Msg  string
}

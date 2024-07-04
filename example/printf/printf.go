package printf

//go:generate go run github.com/tinylib/msgp

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/swdunlop/rig-go/rig/mrpc"
	"github.com/tinylib/msgp/msgp"
)

func Call(_ *mrpc.Scope, req Request) (ret Response, err error) {
	info := make([]interface{}, len(req.Info))
	for i, p := range req.Info {
		var buf bytes.Buffer
		_, err = msgp.UnmarshalAsJSON(&buf, p)
		if err != nil {
			return
		}
		err = json.Unmarshal(buf.Bytes(), &info[i])
		if err != nil {
			return
		}
	}
	ret.String = fmt.Sprintf(req.Message, info...)
	return
}

type Request struct {
	Message string     `msg:"msg"`
	Info    []msgp.Raw `msg:"info"`
}

type Response struct {
	String string `msg:"str"`
}

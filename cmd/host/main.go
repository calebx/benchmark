package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"
	_ "google.golang.org/grpc/encoding/gzip"

	"nothing.com/benchmark/vrpc"
)

const enclaveAddr = "127.0.0.1:5005"

type JsonIterSerializer struct{}

func (j *JsonIterSerializer) Serialize(c echo.Context, i interface{}, indent string) error {
	enc := jsoniter.NewEncoder(c.Response())
	if indent != "" {
		enc.SetIndent("", indent)
	}
	return enc.Encode(i)
}

func (j *JsonIterSerializer) Deserialize(c echo.Context, i interface{}) error {
	return jsoniter.NewDecoder(c.Request().Body).Decode(i)
}

type Req struct {
	XID string `json:"xid"`
}

type Resp struct {
	DIX string `json:"dix"`
}

var globalCli vrpc.Client

func MustInit() {
	ctx := context.Background()

	cid := uint32(16)
	port := uint32(50001)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cliCount := 8

	client, err := vrpc.NewClient(ctx, cid, port, addr, false, cliCount)
	if err != nil {
		log.Panic("initialize vsock rpc client failed", err)
	}
	globalCli = client
}

func Get() vrpc.Client {
	return globalCli
}

func main() {
	e := echo.New()
	e.JSONSerializer = &JsonIterSerializer{}

	MustInit()
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 60 * time.Second,
	}))

	e.POST("/echo", func(c echo.Context) error {
		req := &Req{}
		if err := c.Bind(req); err != nil {
			return c.JSON(http.StatusBadRequest, &Resp{
				DIX: "bad",
			})
		}

		ctx := c.Request().Context()
		resp, err := Invoke[Resp](ctx, "/echo", req)
		if err != nil {
			return c.JSON(http.StatusBadRequest, &Resp{
				DIX: "bad",
			})
		}
		return c.JSON(http.StatusOK, resp)
	})

	e.Logger.Fatal(e.Start(":1323"))
}

func Invoke[T any](ctx context.Context, url string, req any) (resp *T, err error) {
	buf, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	rpcResp, err := Get().Invoke(ctx, url, buf)
	if err != nil {
		return nil, err
	}
	if rpcResp.Code != 0 {
		return nil, err
	}

	resp = new(T)
	if err = json.Unmarshal(rpcResp.Payload, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

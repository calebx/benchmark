package main

import (
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/labstack/echo/v4"
	_ "google.golang.org/grpc/encoding/gzip"

	"nothing.com/benchmark/vsockcli"
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
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	e := echo.New()
	e.JSONSerializer = &JsonIterSerializer{}

	cfg := &vsockcli.Config{
		IsMocked:       true,
		CID:            16,
		Port:           5005,
		Origin:         "http://127.0.0.1:5005",
		CliCount:       4,
		MaxConnsPerCli: 16,
		Timeout:        3 * time.Second,
	}
	vsockcli.MustInit(cfg)
	cli := vsockcli.Get()

	e.POST("/echo", func(c echo.Context) error {
		req := &Req{}
		if err := c.Bind(req); err != nil {
			return c.JSON(http.StatusBadRequest, &Resp{
				Code:    400,
				Message: "Bad Request 5",
			})
		}

		ctx := c.Request().Context()
		buf, err := cli.Post(ctx, "/echo", req)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, &Resp{
				Code:    500,
				Message: err.Error(),
			})
		}
		return c.JSON(http.StatusOK, &Resp{
			Code:    200,
			Message: string(buf),
		})
	})

	e.Logger.Fatal(e.Start(":1323"))
}

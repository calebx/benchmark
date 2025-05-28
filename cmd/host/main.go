package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/labstack/echo/v4"
	_ "google.golang.org/grpc/encoding/gzip"
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
	client, err := NewEnclaveClient(context.Background(), 16, 5005)
	if err != nil {
		log.Fatalf("failed to create client pool: %v", err)
	}

	e := echo.New()
	e.JSONSerializer = &JsonIterSerializer{}

	e.POST("/echo", func(c echo.Context) error {
		req := &Req{}
		if err := c.Bind(req); err != nil {
			return c.JSON(http.StatusBadRequest, &Resp{
				Code:    400,
				Message: "Bad Request",
			})
		}

		respStr, err := client.Post("/echo", &req)
		if err != nil {
			log.Printf("stream error: %s", err)
			return c.JSON(http.StatusInternalServerError, &Resp{Code: 500, Message: err.Error()})
		}

		return c.JSON(http.StatusOK, &Resp{
			Code:    200,
			Message: string(respStr),
		})
	})

	e.Logger.Fatal(e.Start(":1323"))
}

func NewEnclaveClient(ctx context.Context, enclave_cid, enclave_vsock_port uint32) (cli *EnclaveHttpCli, err error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			log.Println("Dialing enclave at vsock port:", enclave_vsock_port)
			type dialResult struct {
				conn net.Conn
				err  error
			}
			done := make(chan dialResult, 1)

			go func() {
				conn, err := net.Dial("tcp", ":1323")
				done <- dialResult{conn, err}
			}()

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case res := <-done:
				return res.conn, res.err
			}
		},
		MaxIdleConns:          1000, // total max idle connections
		MaxIdleConnsPerHost:   100,  // each host max idle connections
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		ForceAttemptHTTP2:     false, // disable http2
	}

	return &EnclaveHttpCli{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d/", enclave_vsock_port),
		cli: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		},
	}, nil
}

type EnclaveHttpCli struct {
	baseURL string
	cli     *http.Client
}

func (api *EnclaveHttpCli) Post(queryPath string, req interface{}) ([]byte, error) {
	log.Println("EnclaveHttpCli Post request to path:", queryPath, " with request:", req)
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("fail to marshal req [%+v] due to error [%w]", req, err)
	}
	url := api.baseURL + queryPath
	cliResp, err := api.cli.Post(url, "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("fail to make POST request to url [%s] due to error [%w]", url, err)
	}
	defer cliResp.Body.Close()
	bodyBytes, err := io.ReadAll(cliResp.Body)
	if err != nil {
		return nil, fmt.Errorf("fail to read request body bytes due to error [%w]", err)
	}
	return bodyBytes, nil
}

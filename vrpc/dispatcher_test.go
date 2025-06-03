package vrpc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type TestResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type TestPointerRequest struct {
	ID    string `json:"id"`
	Value int    `json:"value"`
}

func testHandlerWithRequestResponse(ctx context.Context, req *TestRequest) (*TestResponse, error) {
	if req == nil {
		return nil, assert.AnError
	}
	if req.Name == "error" {
		return nil, assert.AnError
	}
	return &TestResponse{
		Message: fmt.Sprintf("Hello %s, age %d", req.Name, req.Age),
		Status:  "success",
	}, nil
}

func testHandlerWithPointerRequest(ctx context.Context, req *TestPointerRequest) (*TestResponse, error) {
	return &TestResponse{
		Message: fmt.Sprintf("ID: %s, Value: %d", req.ID, req.Value),
		Status:  "success",
	}, nil
}

func testHandlerContextOnly(ctx context.Context) error {
	return nil
}

func testHandlerContextOnlyWithError(ctx context.Context) error {
	return errors.New("context only error")
}

func testHandlerWithResponse(ctx context.Context) (*TestResponse, error) {
	return &TestResponse{
		Message: "no request needed",
		Status:  "ok",
	}, nil
}

func testHandlerWithBool(ctx context.Context, input string) (bool, error) {
	if len(input) > 10 {
		return false, assert.AnError
	}
	return true, nil
}

func invalidHandlerNoParams() error {
	return nil
}

func invalidHandlerWrongFirstParam(s string) error {
	return nil
}

func invalidHandlerNoReturn(ctx context.Context) {
}

func invalidHandlerWrongLastReturn(ctx context.Context) string {
	return "wrong"
}

var notAFunctionVar = "not a function"

func TestDispatcherRegister(t *testing.T) {
	tests := []struct {
		name        string
		handler     interface{}
		expectPanic bool
		panicMsg    string
	}{
		{
			name:        "valid handler with request and response",
			handler:     testHandlerWithRequestResponse,
			expectPanic: false,
		},
		{
			name:        "valid handler context only",
			handler:     testHandlerContextOnly,
			expectPanic: false,
		},
		{
			name:        "valid handler with response only",
			handler:     testHandlerWithResponse,
			expectPanic: false,
		},
		{
			name:        "nil handler",
			handler:     nil,
			expectPanic: true,
			panicMsg:    "handler cannot be nil",
		},
		{
			name:        "not a function",
			handler:     notAFunctionVar,
			expectPanic: true,
			panicMsg:    "handler must be a function",
		},
		{
			name:        "no parameters",
			handler:     invalidHandlerNoParams,
			expectPanic: true,
			panicMsg:    "handler must have at least context parameter",
		},
		{
			name:        "wrong first parameter",
			handler:     invalidHandlerWrongFirstParam,
			expectPanic: true,
			panicMsg:    "first parameter must be context.Context",
		},
		{
			name:        "no return values",
			handler:     invalidHandlerNoReturn,
			expectPanic: true,
			panicMsg:    "handler must return at least error",
		},
		{
			name:        "wrong last return type",
			handler:     invalidHandlerWrongLastReturn,
			expectPanic: true,
			panicMsg:    "last return value must be error",
		},
		{
			name:        "simple string to bool func",
			handler:     testHandlerWithBool,
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &dispatcher{
				cmdToFunc: make(map[string]any),
			}

			if tt.expectPanic {
				assert.PanicsWithValue(t, tt.panicMsg, func() {
					d.Register("test", tt.handler)
				})
			} else {
				assert.NotPanics(t, func() {
					d.Register("test", tt.handler)
				})
				assert.Contains(t, d.cmdToFunc, "test")
			}
		})
	}
}

func TestDispatcherDispatch(t *testing.T) {
	d := &dispatcher{
		cmdToFunc: make(map[string]any),
		errCode:   10005,
	}

	d.Register("test_req_resp", testHandlerWithRequestResponse)
	d.Register("test_context_only", testHandlerContextOnly)
	d.Register("test_context_error", testHandlerContextOnlyWithError)
	d.Register("test_response_only", testHandlerWithResponse)
	d.Register("test_pointer_req", testHandlerWithPointerRequest)
	d.Register("test_simple_string_to_bool", testHandlerWithBool)

	tests := []struct {
		name         string
		command      string
		payload      []byte
		expectedCode uint32
		expectedMsg  string
		expectResp   bool
	}{
		{
			name:         "successful request with response",
			command:      "test_req_resp",
			payload:      []byte(`{"name":"John","age":30}`),
			expectedCode: 0,
			expectedMsg:  "success",
			expectResp:   true,
		},
		{
			name:         "context only handler success",
			command:      "test_context_only",
			payload:      nil,
			expectedCode: 0,
			expectedMsg:  "success",
			expectResp:   false,
		},
		{
			name:         "response only handler",
			command:      "test_response_only",
			payload:      nil,
			expectedCode: 0,
			expectedMsg:  "success",
			expectResp:   true,
		},
		{
			name:         "handler returns error",
			command:      "test_req_resp",
			payload:      []byte(`{"name":"error","age":25}`),
			expectedCode: 10005, // ErrInternalError code
			expectedMsg:  "test error",
			expectResp:   false,
		},
		{
			name:         "context only handler with error",
			command:      "test_context_error",
			payload:      nil,
			expectedCode: 10005,
			expectedMsg:  "handler error: context only error",
			expectResp:   false,
		},
		{
			name:         "command not found",
			command:      "nonexistent",
			payload:      nil,
			expectedCode: 10005,
			expectedMsg:  "command [nonexistent] not found",
			expectResp:   false,
		},
		{
			name:         "invalid JSON payload",
			command:      "test_req_resp",
			payload:      []byte(`{"name":"John","age":}`),
			expectedCode: 10005,
			expectedMsg:  "failed to parse request:",
			expectResp:   false,
		},
		{
			name:         "pointer request handler",
			command:      "test_pointer_req",
			payload:      []byte(`{"id":"test123","value":42}`),
			expectedCode: 0,
			expectedMsg:  "success",
			expectResp:   true,
		},
		{
			name:         "empty payload for handler expecting request",
			command:      "test_req_resp",
			payload:      []byte{},
			expectedCode: 10005,
			expectedMsg:  "nil",
			expectResp:   false,
		},
		{
			name:         "simple string to bool",
			command:      "test_simple_string_to_bool",
			payload:      []byte{},
			expectedCode: 0,
			expectedMsg:  "success",
			expectResp:   true,
		},
		{
			name:         "simple string to bool failed",
			command:      "test_simple_string_to_bool",
			payload:      []byte(`"a very very very very long message"`),
			expectedCode: 10005,
			expectedMsg:  "input too long",
			expectResp:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, msg, payload := d.Dispatch(tt.command, tt.payload)

			assert.Equal(t, tt.expectedCode, code)
			if tt.expectedMsg != "" {
				assert.Contains(t, msg, tt.expectedMsg)
			}

			if tt.expectResp {
				assert.NotNil(t, payload)
				assert.Greater(t, len(payload), 0)
				str := string(payload)
				t.Log("resp: ", str)
			} else {
				assert.Nil(t, payload)
			}
		})
	}
}

func TestParseArg(t *testing.T) {
	d := &dispatcher{}

	t.Run("empty payload", func(t *testing.T) {
		arg, keep, err := d.parseArg(testHandlerWithRequestResponse, []byte{})
		assert.NoError(t, err)
		assert.Nil(t, arg)
		assert.True(t, keep)
	})

	t.Run("function with no second parameter", func(t *testing.T) {
		arg, _, err := d.parseArg(testHandlerContextOnly, []byte(`{"test":"data"}`))
		assert.NoError(t, err)
		assert.Nil(t, arg)
	})

	t.Run("valid JSON for pointer parameter", func(t *testing.T) {
		payload := []byte(`{"name":"John","age":30}`)
		arg, _, err := d.parseArg(testHandlerWithRequestResponse, payload)
		assert.NoError(t, err)
		assert.NotNil(t, arg)

		req, ok := arg.(*TestRequest)
		assert.True(t, ok)
		assert.Equal(t, "John", req.Name)
		assert.Equal(t, 30, req.Age)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		payload := []byte(`{"name":"John","age":}`)
		arg, _, err := d.parseArg(testHandlerWithRequestResponse, payload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal request payload err")
		assert.Nil(t, arg)
	})
}

func TestCallFunc(t *testing.T) {
	d := &dispatcher{}

	t.Run("call function with args", func(t *testing.T) {
		ctx := context.Background()
		req := &TestRequest{Name: "John", Age: 30}

		results := d.callFunc(testHandlerWithRequestResponse, ctx, req)
		assert.Len(t, results, 2)

		resp, ok := results[0].(*TestResponse)
		assert.True(t, ok)
		assert.Equal(t, "Hello John, age 30", resp.Message)
		assert.Equal(t, "success", resp.Status)

		assert.Nil(t, results[1])
	})

	t.Run("call function with nil args", func(t *testing.T) {
		ctx := context.Background()

		results := d.callFunc(testHandlerContextOnly, ctx)
		assert.Len(t, results, 1)
		assert.Nil(t, results[0])
	})

	t.Run("call function that returns error", func(t *testing.T) {
		ctx := context.Background()

		results := d.callFunc(testHandlerContextOnlyWithError, ctx)
		assert.Len(t, results, 1)
		assert.NotNil(t, results[0])

		err, ok := results[0].(error)
		assert.True(t, ok)
		assert.Equal(t, "context only error", err.Error())
	})
}

func TestInitDispatcher(t *testing.T) {
	t.Run("init dispatcher creates instance", func(t *testing.T) {
		dp := &dispatcher{
			cmdToFunc: make(map[string]any),
		}

		dp.Register("/test", testHandlerContextOnly)

		assert.Contains(t, dp.cmdToFunc, "/test")
		assert.Len(t, dp.cmdToFunc, 1)
	})
}

package vrpc

import (
	"context"
	"fmt"
	"reflect"
	"time"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Dispatcher interface {
	Dispatch(cmd string, request []byte) (uint32, string, []byte)
	Register(cmd string, handler any)
}

func NewDispatcher(defaultErrCode uint32, defaultTimout time.Duration) Dispatcher {
	return &dispatcher{
		cmdToFunc: make(map[string]any),
		errCode:   defaultErrCode,
		timeout:   defaultTimout,
	}
}

type dispatcher struct {
	cmdToFunc map[string]any
	errCode   uint32
	timeout   time.Duration
}

func (d *dispatcher) Register(cmd string, handler any) {
	// simple validate handler
	if handler == nil {
		panic("handler cannot be nil")
	}
	handlerType := reflect.TypeOf(handler)
	if handlerType.Kind() != reflect.Func {
		panic("handler must be a function")
	}

	// Handler should have at least context parameter
	if handlerType.NumIn() < 1 {
		panic("handler must have at least context parameter")
	}

	// First parameter should be context.Context
	firstParam := handlerType.In(0)
	contextInterface := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !firstParam.Implements(contextInterface) {
		panic("first parameter must be context.Context")
	}

	// Must have at least one return value
	if handlerType.NumOut() < 1 {
		panic("handler must return at least error")
	}

	// last return value should be error
	lastOut := handlerType.Out(handlerType.NumOut() - 1)
	errorInterface := reflect.TypeOf((*error)(nil)).Elem()
	if !lastOut.Implements(errorInterface) {
		panic("last return value must be error")
	}

	d.cmdToFunc[cmd] = handler
}

func (d *dispatcher) Dispatch(command string, payload []byte) (code uint32, message string, responsePayload []byte) {
	handler, ok := d.cmdToFunc[command]
	if !ok {
		return d.errCode, fmt.Sprintf("command [%s] not found", command), nil
	}

	// TODO: involve context from request
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	args := []any{ctx}

	arg, keep, err := d.parseArg(handler, payload)
	if err != nil {
		return d.errCode, fmt.Sprintf("parse request err: %v", err), nil
	}
	if keep {
		args = append(args, arg)
	}

	results := d.callFunc(handler, args...)
	if len(results) == 0 {
		return d.errCode, "handler has no results", nil
	}

	// last result should be error
	lastResult := results[len(results)-1]
	if lastResult != nil {
		if err, ok := lastResult.(error); ok && err != nil {
			return d.errCode, fmt.Sprintf("handler err: %v", err), nil
		}
	}

	// take first result as expected payload
	if len(results) > 1 && results[0] != nil {
		respBuf, err := json.Marshal(results[0])
		if err != nil {
			return d.errCode, fmt.Sprintf("marshal err: %v", err), nil
		}
		return 0, "ok", respBuf
	}
	return 0, "ok", nil
}

// parseArg parses the second argument of the function
// returns the argument value, skip it or not, and any error encountered
func (d *dispatcher) parseArg(fn any, buf []byte) (any, bool, error) {
	ft := reflect.TypeOf(fn)
	if ft.NumIn() <= 1 {
		return nil, false, nil
	}

	argType := ft.In(1)
	var reqArg any
	if argType.Kind() == reflect.Ptr {
		reqArg = reflect.New(argType.Elem()).Interface()
	} else {
		reqArg = reflect.New(argType).Interface()
	}

	// if no payload, return zero value of the argument type
	if len(buf) == 0 {
		if argType.Kind() == reflect.Ptr {
			return nil, true, nil
		}
		return reflect.ValueOf(reqArg).Elem().Interface(), true, nil
	}

	if err := json.Unmarshal(buf, reqArg); err != nil {
		return nil, true, fmt.Errorf("unmarshal request payload err: %v", err)
	}
	if argType.Kind() == reflect.Ptr {
		return reqArg, true, nil
	}
	return reflect.ValueOf(reqArg).Elem().Interface(), true, nil
}

func (d *dispatcher) callFunc(fn any, args ...any) []any {
	fv := reflect.ValueOf(fn)

	params := make([]reflect.Value, len(args))
	for i, arg := range args {
		if arg == nil {
			expectedType := fv.Type().In(i)
			params[i] = reflect.Zero(expectedType)
		} else {
			params[i] = reflect.ValueOf(arg)
		}
	}

	rs := fv.Call(params)
	result := make([]any, len(rs))
	for i, r := range rs {
		result[i] = r.Interface()
	}
	return result
}

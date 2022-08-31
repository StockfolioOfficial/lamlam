package lamlam

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/aws/aws-lambda-go/lambda"
	"reflect"
	"sync"
)

type _inputKind int

const (
	inputNothing _inputKind = iota
	inputContextOnly
	inputDataOnly
	inputBoth
)

type _outputKind int

const (
	outputNothing _outputKind = iota
	outputErrorOnly
	outputDataOnly
	outputBoth
)

var (
	dummyFuncValue                = reflect.TypeOf(func(context.Context) error { return nil })
	contextType                   = dummyFuncValue.In(0)
	errorType                     = dummyFuncValue.Out(0)
	_              lambda.Handler = (*Mux)(nil)
)

type (
	handler struct {
		originFunc     interface{}
		inputKind      _inputKind
		outputKind     _outputKind
		funcValue      reflect.Value
		funcType       reflect.Type
		funcInputType  [2]reflect.Type
		funcOutputType [2]reflect.Type
	}

	Mux struct {
		tableLock sync.RWMutex
		funcTable map[string]*handler
	}
)

func NewMux() *Mux {
	return &Mux{
		funcTable: make(map[string]*handler),
	}
}

//
//func MakeMux[T any](t T) (m *Mux) {
//	m = NewMux()
//
//	val := reflect.ValueOf(t)
//	if !val.IsValid() || val.IsNil() {
//		return
//	}
//
//	val.Type()
//	typ := reflect.TypeOf(new(T)).Elem()
//	name := typ.Name()
//	for i := 0; i < typ.NumMethod(); i++ {
//		methodTyp := typ.Method(i)
//		if !methodTyp.IsExported() {
//			continue
//		}
//
//		funcValue := val.MethodByName(methodTyp.Name)
//		m.setHandler(fmt.Sprintf("%s_%s", name, methodTyp.Name), &handler{
//			funcValue: funcValue,
//			funcType:  funcValue.Type(),
//		})
//	}
//
//	return
//}

func (m *Mux) Invoke(ctx context.Context, payload []byte) (res []byte, err error) {
	var p payloadType
	err = json.Unmarshal(payload, &p)
	if err != nil {
		return
	}

	f, ok := m.funcTable[p.FuncKey]
	if !ok {
		err = ErrNotFoundFunction
		return
	}

	return f.invoke(ctx, &p)
}

func (m *Mux) Set(funcKey string, f interface{}) error {
	funcValue := reflect.ValueOf(f)
	return m.setHandler(funcKey, &handler{
		funcValue: funcValue,
		funcType:  funcValue.Type(),
	})
}

func (m *Mux) setHandler(funcKey string, h *handler) error {
	if h.funcValue.Kind() != reflect.Func {
		return errors.New("not function")
	}

	numIn := h.funcType.NumIn()
	if numIn > 2 {
		return errors.New("func args number must be under the three")
	}

	for i := 0; i < numIn; i++ {
		h.funcInputType[i] = h.funcType.In(i)
	}

	switch numIn {
	case 0:
		h.inputKind = inputNothing
	case 1:
		if h.funcInputType[0] == contextType {
			h.inputKind = inputContextOnly
		} else {
			h.inputKind = inputDataOnly
		}
	case 2:
		if h.funcInputType[0] != contextType {
			return errors.New("if tow args, first argument must be \"context.Context\"")
		}
		h.inputKind = inputBoth
	}

	numOut := h.funcType.NumOut()
	if numOut > 2 {
		return errors.New("func returns number must be under the three")
	}

	for i := 0; i < numOut; i++ {
		h.funcOutputType[i] = h.funcType.Out(i)
	}

	switch numOut {
	case 0:
		h.outputKind = outputNothing
	case 1:
		if h.funcOutputType[0] == errorType {
			h.outputKind = outputErrorOnly
		} else {
			h.outputKind = outputDataOnly
		}
	case 2:
		if h.funcOutputType[1] != errorType {
			return errors.New("if 2 returns, second return must be \"error\"")
		}

		h.outputKind = outputBoth
	}

	m.tableLock.Lock()
	defer m.tableLock.Unlock()
	m.funcTable[funcKey] = h
	return nil
}

func (h *handler) invoke(ctx context.Context, payload *payloadType) ([]byte, error) {
	var inValues []reflect.Value

	switch h.inputKind {
	case inputNothing:
		// nothing
	case inputContextOnly:
		inValues = []reflect.Value{reflect.ValueOf(ctx)}
	case inputDataOnly:
		val, err := getValueFromPayload(h.funcInputType[0], payload)
		if err != nil {
			return nil, err
		}
		inValues = []reflect.Value{val}
	case inputBoth:
		val, err := getValueFromPayload(h.funcInputType[1], payload)
		if err != nil {
			return nil, err
		}
		inValues = []reflect.Value{reflect.ValueOf(ctx), val}
	}

	resValues := h.funcValue.Call(inValues)

	var res []byte
	var err error
	switch h.outputKind {
	case outputNothing:
		// nothing
	case outputErrorOnly:
		setErrorSafety(&err, resValues[0])
	case outputDataOnly:
		res, err = valueJsonMarshal(resValues[0])
	case outputBoth:
		res, err = valueJsonMarshal(resValues[0])
		if err != nil {
			break
		}
		setErrorSafety(&err, resValues[1])
	}

	return res, err
}

func setErrorSafety(dst *error, maybeError reflect.Value) {
	errorValue := maybeError.Interface()
	if errorValue != nil {
		*dst = errorValue.(error)
	}
}

func valueJsonMarshal(val reflect.Value) ([]byte, error) {
	intf := val.Interface()
	if intf == nil {
		return nil, nil
	}

	return json.Marshal(intf)
}

func getValueFromPayload(typ reflect.Type, payload *payloadType) (res reflect.Value, err error) {
	val := reflect.New(typ)
	err = payload.bindData(val.Interface())
	if err != nil {
		return
	}

	res = val.Elem()
	return
}

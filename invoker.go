package lamlam

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

type (
	Invoker struct {
		funcName string
		cli      *lambda.Client
	}

	invokeFunc func(context.Context, []byte) ([]byte, error)

	Handler struct {
		invoke  invokeFunc
		payload payloadType
	}

	Return struct {
		data []byte
		err  error
	}
)

func NewInvoker(cli *lambda.Client, funcName string) *Invoker {
	return &Invoker{
		funcName: funcName,
		cli:      cli,
	}
}

func (i *Invoker) Client() *lambda.Client {
	return i.cli
}

func (i *Invoker) Func(key string) *Handler {
	return newInvokeHandler(key, i.invoke)
}

func (i *Invoker) Invoke(ctx context.Context, key string, in interface{}) *Return {
	return i.Func(key).Invoke(ctx, in)
}

func (i *Invoker) invoke(ctx context.Context, payload []byte) (res []byte, err error) {
	result, err := i.cli.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: &i.funcName,
		Payload:      payload,
	})
	if err != nil {
		return
	}

	res = result.Payload
	if result.FunctionError != nil {
		err = ErrUnhandled
	}

	return
}

func newInvokeHandler(funcKey string, invoke invokeFunc) *Handler {
	return &Handler{
		invoke: invoke,
		payload: payloadType{
			FuncKey: funcKey,
		},
	}
}

func (i *Handler) Invoke(ctx context.Context, in any) *Return {
	res := &Return{}
	err := i.payload.setData(in)
	if err != nil {
		res.err = err
		return res
	}

	data, err := json.Marshal(i.payload)
	if err != nil {
		res.err = err
		return res
	}

	res.data, res.err = i.invoke(ctx, data)
	return res
}

func (r *Return) Raw() ([]byte, error) {
	return r.data, r.err
}

func (r *Return) Result(dst any) error {
	switch r.err {
	case nil:
	case ErrUnhandled:
		if len(r.data) > 0 {
			var errPayload ErrorPayload
			if err := json.Unmarshal(r.data, &errPayload); err != nil {
				return err
			}

			return errPayload.TryCastKnownError()
		} else {
			return ErrUnhandled
		}
	default:
		return r.err
	}

	if dst == nil {
		return nil
	}

	return json.Unmarshal(r.data, dst)
}

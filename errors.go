package lamlam

import (
	"errors"
	"fmt"
	"reflect"
)

type UnmarshalerErrorPayload interface {
	UnmarshalErrorPayload(ep *ErrorPayload) error
}

var _ error = (*ErrorPayload)(nil)

type ErrorPayload struct {
	ErrorMessage string `json:"errorMessage"`
	ErrorType    string `json:"errorType"`
}

func (e *ErrorPayload) Error() string {
	return fmt.Sprintf("type: %s, message: %s", e.ErrorType, e.ErrorMessage)
}

func (e *ErrorPayload) Is(err error) bool {
	return e.ErrorType == getTypeName(reflect.TypeOf(err))
}

func (e *ErrorPayload) As(target any) bool {
	if target == nil {
		return false
	}
	val := reflect.ValueOf(target)
	typ := val.Type()
	if typ.Kind() != reflect.Ptr || val.IsNil() {
		return false
	}
	targetType := typ.Elem()
	if targetType.Kind() != reflect.Interface && !targetType.Implements(errorType) {
		return false
	}
	if e.ErrorType == getTypeName(targetType) {
		targetValue := val.Elem()
		if targetType.Kind() == reflect.Ptr {
			targetValue.Set(reflect.New(targetType.Elem()))
		}

		if unmarshal, ok := targetValue.Interface().(UnmarshalerErrorPayload); ok {
			err := unmarshal.UnmarshalErrorPayload(e)
			if err != nil {
				return false
			}
		}
		return true
	}

	return false
}

func getTypeName(typ reflect.Type) string {
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	return typ.Name()
}

func (e *ErrorPayload) TryCast(err error) error {
	if e.Is(err) {
		return err
	}

	return e.TryCastKnownError()
}

func (e *ErrorPayload) TryCastKnownError() error {
	err := knownErrTable[e.ErrorType]
	if err != nil {
		return err
	}

	return e
}

func UnwrapErrorPayload(err error) (res *ErrorPayload) {
	if errors.As(err, &res) {
		return
	}

	return
}

type errNotFoundFunction struct{}

func (err *errNotFoundFunction) Error() string {
	return "not found function"
}

var (
	ErrNotFoundFunction error = &errNotFoundFunction{}
	ErrUnhandled              = errors.New("Unhandled")

	knownErrTable = map[string]error{
		"errNotFoundFunction": ErrNotFoundFunction,
	}
)

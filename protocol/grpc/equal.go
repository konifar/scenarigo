package grpc

import (
	"reflect"

	"github.com/golang/protobuf/proto" // nolint:staticcheck
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
)

func init() {
	assert.RegisterCustomEqualer(assert.EqualerFunc(equalEnum))
	assert.RegisterCustomEqualer(assert.EqualerFunc(equalMessage))
}

var protoMessage = reflect.TypeOf((*proto.Message)(nil)).Elem()

func equalEnum(expected interface{}, got interface{}) (bool, error) {
	s, ok := expected.(string)
	if !ok {
		return false, nil
	}
	enum, ok := got.(protoreflect.Enum)
	if !ok {
		return false, nil
	}
	if string(enum.Descriptor().Values().ByNumber(enum.Number()).Name()) == s {
		return true, nil
	}
	return false, nil
}

func equalMessage(expected interface{}, got interface{}) (bool, error) {
	// use the pointer to the value if the pointer type implements proto.Message
	e, ok, _ := reflectutil.ConvertInterface(protoMessage, expected)
	if ok {
		expected = e
	}
	g, ok, _ := reflectutil.ConvertInterface(protoMessage, got)
	if ok {
		got = g
	}
	em, ok := expected.(proto.Message)
	if !ok {
		return false, nil
	}
	gm, ok := got.(proto.Message)
	if !ok {
		return false, nil
	}
	if proto.Equal(em, gm) {
		return true, nil
	}
	return false, nil
}

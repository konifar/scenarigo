package schema

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/protocol"
)

type testProtocol struct {
	name            string
	request, expect interface{}
}

func (p *testProtocol) Name() string { return p.name }

func (p *testProtocol) UnmarshalRequest(b []byte) (protocol.Invoker, error) {
	if err := yaml.Unmarshal(b, &p.request); err != nil {
		return nil, err
	}
	return nil, nil
}

func (p *testProtocol) UnmarshalExpect(b []byte) (protocol.AssertionBuilder, error) {
	if b == nil {
		return &testAssertionBuilder{}, nil
	}
	if err := yaml.NewDecoder(bytes.NewBuffer(b), yaml.UseOrderedMap()).Decode(&p.expect); err != nil {
		return nil, err
	}
	return &testAssertionBuilder{}, nil
}

type testAssertionBuilder struct{}

func (*testAssertionBuilder) Build(_ *context.Context) (assert.Assertion, error) { return nil, nil }

func TestLoadScenarios(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			path                              string
			scenarios                         []*Scenario
			request, expect, assertionBuilder interface{}
		}{
			"valid": {
				path: "testdata/valid.yaml",

				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Expect: Expect{
									AssertionBuilder: &testAssertionBuilder{},
									bytes:            nil,
								},
							},
						},
						filepath: "testdata/valid.yaml",
					},
				},
				request: map[string]interface{}{
					"body": map[string]interface{}{
						"message": "{{vars.message}}",
					},
				},
				expect: yaml.MapSlice{
					{
						Key: "body",
						Value: yaml.MapSlice{
							{
								Key:   "message",
								Value: "{{request.body}}",
							},
						},
					},
				},
			},
			"anchor": {
				path: "testdata/valid-anchor.yaml",

				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Expect: Expect{
									AssertionBuilder: &testAssertionBuilder{},
									bytes:            nil,
								},
							},
						},
						filepath: "testdata/valid-anchor.yaml",
					},
				},
				request: map[string]interface{}{
					"body": map[string]interface{}{
						"message": "{{vars.message}}",
					},
				},
				expect: yaml.MapSlice{
					{
						Key: "body",
						Value: yaml.MapSlice{
							{
								Key:   "message",
								Value: "{{request.body}}",
							},
						},
					},
				},
			},
			"without protocol": {
				path: "testdata/valid-without-protocol.yaml",

				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Include: "./valid.yaml",
							},
						},
						filepath: "testdata/valid-without-protocol.yaml",
					},
				},
				request: map[interface{}]interface{}{},
				expect:  map[interface{}]interface{}{},
			},
			"without expect": {
				path: "testdata/valid-without-expect.yaml",

				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Expect: Expect{
									AssertionBuilder: &testAssertionBuilder{},
									bytes:            nil,
								},
							},
						},
						filepath: "testdata/valid-without-expect.yaml",
					},
				},
				request: map[string]interface{}{
					"body": map[string]interface{}{
						"message": "{{vars.message}}",
					},
				},
				expect: map[interface{}]interface{}{},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				p := &testProtocol{
					name:    "test",
					request: map[interface{}]interface{}{},
					expect:  map[interface{}]interface{}{},
				}
				protocol.Register(p)
				defer protocol.Unregister(p.Name())

				got, err := LoadScenarios(test.path)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.scenarios, got,
					cmp.AllowUnexported(
						Scenario{}, Request{}, Expect{},
					),
					cmp.FilterPath(func(path cmp.Path) bool {
						s := path.String()
						if s == "Steps.Request" {
							return true
						}
						if s == "Steps.Expect.bytes" {
							return true
						}
						if s == "Node" {
							return true
						}
						return false
					}, cmp.Ignore()),
				); diff != "" {
					t.Errorf("scenario differs (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(test.request, p.request); diff != "" {
					t.Errorf("request differs (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(test.expect, p.expect); diff != "" {
					t.Errorf("expect differs (-want +got):\n%s", diff)
				}
				for i, scn := range got {
					if g, e := scn.filepath, test.path; g != e {
						t.Errorf("[%d] expect %q but got %q", i, e, g)
					}
					if scn.Node == nil {
						t.Errorf("[%d] Node is nil", i)
					}
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		p := &testProtocol{
			name: "test",
		}
		protocol.Register(p)
		defer protocol.Unregister(p.Name())

		tests := map[string]struct {
			path string
		}{
			"not found": {
				path: "notfound.yaml",
			},
			"parse error": {
				path: "testdata/parse-error.yaml",
			},
			"invalid": {
				path: "testdata/invalid.yaml",
			},
			"unknown protocol": {
				path: "testdata/unknown-protocol.yaml",
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				_, err := LoadScenarios(test.path)
				if err == nil {
					t.Fatal("expected error but no error")
				}
			})
		}
	})
}

func TestLoadScenariosFromReader(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			yaml            string
			scenarios       []*Scenario
			request, expect interface{}
		}{
			"valid": {
				yaml: `
title: echo-service
description: check echo-service
vars:
  message: hello
steps:
  - title: POST /say
    description: check to respond same message
    protocol: test
    request:
      body:
        message: "{{vars.message}}"
    expect:
      body:
        message: "{{request.body}}"
`,
				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
							},
						},
					},
				},
				request: map[string]interface{}{
					"body": map[string]interface{}{
						"message": "{{vars.message}}",
					},
				},
				expect: yaml.MapSlice{
					{
						Key: "body",
						Value: yaml.MapSlice{
							{
								Key:   "message",
								Value: "{{request.body}}",
							},
						},
					},
				},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				p := &testProtocol{
					name:    "test",
					request: map[interface{}]interface{}{},
					expect:  map[interface{}]interface{}{},
				}
				protocol.Register(p)
				defer protocol.Unregister(p.Name())

				got, err := LoadScenariosFromReader(strings.NewReader(test.yaml))
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.scenarios, got,
					cmp.AllowUnexported(
						Scenario{}, Request{}, Expect{},
					),
					cmp.FilterPath(func(path cmp.Path) bool {
						s := path.String()
						if s == "Steps.Request" {
							return true
						}
						if s == "Steps.Expect" {
							return true
						}
						if s == "Node" {
							return true
						}
						return false
					}, cmp.Ignore()),
				); diff != "" {
					t.Errorf("scenario differs (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(test.request, p.request); diff != "" {
					t.Errorf("request differs (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(test.expect, p.expect); diff != "" {
					t.Errorf("expect differs (-want +got):\n%s", diff)
				}
				for i, scn := range got {
					if g, e := scn.filepath, ""; g != e {
						t.Errorf("[%d] expect %q but got %q", i, e, g)
					}
					if scn.Node == nil {
						t.Errorf("[%d] Node is nil", i)
					}
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			r io.Reader
		}{
			"failed to read": {
				r: errReader{errors.New("read error")},
			},
			"parse error": {
				r: strings.NewReader(`
a:
- b
  c: d`),
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				_, err := LoadScenariosFromReader(test.r)
				if err == nil {
					t.Fatal("expected error but no error")
				}
			})
		}
	})
}

type errReader struct {
	err error
}

func (r errReader) Read(_ []byte) (int, error) { return 0, r.err }

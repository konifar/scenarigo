// +build !race

package scenarigo

import (
	"bytes"
	gocontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zoncoen/scenarigo"
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/protocol"
	"github.com/zoncoen/scenarigo/reporter"
	"github.com/zoncoen/scenarigo/testdata/gen/pb/test"
)

type testProtocol struct {
	name    string
	invoker invoker
	builder builder
}

func (p *testProtocol) Name() string { return p.name }

func (p *testProtocol) UnmarshalRequest(_ []byte) (protocol.Invoker, error) {
	return p.invoker, nil
}

func (p *testProtocol) UnmarshalExpect(_ []byte) (protocol.AssertionBuilder, error) {
	return p.builder, nil
}

type invoker func(*context.Context) (*context.Context, interface{}, error)

func (f invoker) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	return f(ctx)
}

type builder func(*context.Context) (assert.Assertion, error)

func (f builder) Build(ctx *context.Context) (assert.Assertion, error) {
	return f(ctx)
}

type testGRPCServer struct {
	users map[string]string
}

func (s *testGRPCServer) Echo(ctx gocontext.Context, req *test.EchoRequest) (*test.EchoResponse, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	ts := md.Get("token")
	if len(ts) == 0 {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if _, ok := s.users[ts[0]]; !ok {
		sts, err := status.New(codes.Unauthenticated, "invalid token").
			WithDetails(&errdetails.LocalizedMessage{
				Locale:  "ja-JP",
				Message: "だめ",
			}, &errdetails.LocalizedMessage{
				Locale:  "en-US",
				Message: "NG",
			}, &errdetails.DebugInfo{
				Detail: "test",
			})
		if err != nil {
			return nil, err
		}
		return nil, sts.Err()
	}
	return &test.EchoResponse{
		MessageId:   req.MessageId,
		MessageBody: req.MessageBody,
		UserType:    test.UserType_CUSTOMER,
	}, nil
}

func TestRunner_Run(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			scenario string
			invoker  func(*context.Context) (*context.Context, interface{}, error)
			builder  func(*context.Context) (assert.Assertion, error)
		}{
			"simple": {
				scenario: "testdata/scenarios/simple.yaml",
				invoker:  func(ctx *context.Context) (*context.Context, interface{}, error) { return ctx, nil, nil },
				builder: func(ctx *context.Context) (assert.Assertion, error) {
					return assert.AssertionFunc(func(_ interface{}) error { return nil }), nil
				},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				var invoked, built bool
				p := &testProtocol{
					name: "test",
					invoker: invoker(func(ctx *context.Context) (*context.Context, interface{}, error) {
						invoked = true
						return test.invoker(ctx)
					}),
					builder: builder(func(ctx *context.Context) (assert.Assertion, error) {
						built = true
						return test.builder(ctx)
					}),
				}
				protocol.Register(p)
				defer protocol.Unregister(p.Name())

				r, err := scenarigo.NewRunner(scenarigo.WithScenarios(test.scenario))
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				var b bytes.Buffer
				ok := reporter.Run(func(rptr reporter.Reporter) {
					r.Run(context.New(rptr))
				}, reporter.WithWriter(&b))
				if !ok {
					t.Fatalf("scenario failed:\n%s", b.String())
				}
				if !invoked {
					t.Error("did not invoke")
				}
				if !built {
					t.Error("did not build the assertion")
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			scenario string
			invoker  func(*context.Context) (*context.Context, interface{}, error)
			builder  func(*context.Context) (assert.Assertion, error)
		}{
			"failed to invoke": {
				scenario: "testdata/scenarios/simple.yaml",
				invoker: func(ctx *context.Context) (*context.Context, interface{}, error) {
					return nil, nil, errors.New("some error occurred")
				},
			},
			"failed to build the assertion": {
				scenario: "testdata/scenarios/simple.yaml",
				invoker:  func(ctx *context.Context) (*context.Context, interface{}, error) { return ctx, nil, nil },
				builder:  func(ctx *context.Context) (assert.Assertion, error) { return nil, errors.New("some error occurred") },
			},
			"assertion error": {
				scenario: "testdata/scenarios/simple.yaml",
				invoker:  func(ctx *context.Context) (*context.Context, interface{}, error) { return ctx, nil, nil },
				builder: func(ctx *context.Context) (assert.Assertion, error) {
					return assert.AssertionFunc(func(_ interface{}) error { return errors.New("some error occurred") }), nil
				},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				var invoked, built bool
				p := &testProtocol{
					name: "test",
					invoker: invoker(func(ctx *context.Context) (*context.Context, interface{}, error) {
						invoked = true
						return test.invoker(ctx)
					}),
					builder: builder(func(ctx *context.Context) (assert.Assertion, error) {
						built = true
						return test.builder(ctx)
					}),
				}
				protocol.Register(p)
				defer protocol.Unregister(p.Name())

				r, err := scenarigo.NewRunner(scenarigo.WithScenarios(test.scenario))
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				var b bytes.Buffer
				ok := reporter.Run(func(rptr reporter.Reporter) {
					r.Run(context.New(rptr))
				}, reporter.WithWriter(&b))
				if ok {
					t.Fatal("test passed")
				}
				if test.invoker != nil && !invoked {
					t.Error("did not invoke")
				}
				if test.builder != nil && !built {
					t.Error("did not build the assertion")
				}
			})
		}
	})
}

func TestRunner_Run_Scenarios(t *testing.T) {
	tests := map[string]struct {
		ok    string
		ng    string
		setup func(*testing.T) func()
	}{
		"http": {
			ok:    "testdata/scenarios/http.yaml",
			setup: startHTTPServer,
		},
		"grpc": {
			ok: "testdata/scenarios/grpc.yaml",
			ng: "testdata/scenarios/grpc-ng.yaml",
			setup: func(t *testing.T) func() {
				t.Helper()

				token := "XXXXX"
				testServer := &testGRPCServer{
					users: map[string]string{
						token: "test user",
					},
				}
				s := grpc.NewServer()
				test.RegisterTestServer(s, testServer)

				ln, err := net.Listen("tcp", "localhost:0")
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				if err := os.Setenv("TEST_ADDR", ln.Addr().String()); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if err := os.Setenv("TEST_TOKEN", token); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				go func() {
					_ = s.Serve(ln)
				}()

				return func() {
					s.Stop()
					os.Unsetenv("TEST_ADDR")
					os.Unsetenv("TEST_TOKEN")
				}
			},
		},
		"complex": {
			ok: "testdata/scenarios/complex.yaml",
			setup: func(t *testing.T) func() {
				t.Helper()
				mux := http.NewServeMux()
				mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				})
				mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
					body := map[string]string{}
					d := json.NewDecoder(r.Body)
					defer r.Body.Close()
					if err := d.Decode(&body); err != nil {
						t.Fatalf("failed to decode request body: %s", err)
					}
					var msg string
					if m, ok := body["message"]; ok {
						msg = m
					}
					b, err := json.Marshal(map[string]string{
						"message": msg,
					})
					if err != nil {
						t.Fatalf("failed to marshal: %s", err)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(b)
				})
				var count int32
				mux.HandleFunc("/count", func(w http.ResponseWriter, r *http.Request) {
					i := atomic.AddInt32(&count, 1)
					_, _ = w.Write([]byte(strconv.Itoa(int(i))))
				})

				s := httptest.NewServer(mux)
				if err := os.Setenv("TEST_ADDR", s.URL); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				return func() {
					s.Close()
					os.Unsetenv("TEST_ADDR")
				}
			},
		},
		"assert": {
			ok: "testdata/scenarios/assert.yaml",
			setup: func(t *testing.T) func() {
				t.Helper()

				mux := http.NewServeMux()
				mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					defer r.Body.Close()
					w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
					_, _ = io.Copy(w, r.Body)
				})

				s := httptest.NewServer(mux)
				if err := os.Setenv("TEST_ADDR", s.URL); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				return func() {
					s.Close()
					os.Unsetenv("TEST_ADDR")
				}
			},
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			teardown := tc.setup(t)
			defer teardown()

			if tc.ok != "" {
				t.Run("ok", func(t *testing.T) {
					r, err := scenarigo.NewRunner(scenarigo.WithScenarios(tc.ok))
					if err != nil {
						t.Fatalf("unexpected error: %s", err)
					}

					var b bytes.Buffer
					ok := reporter.Run(func(rptr reporter.Reporter) {
						r.Run(context.New(rptr).WithPluginDir("testdata/gen/plugins"))
					}, reporter.WithWriter(&b))
					if !ok {
						t.Fatalf("scenario failed:\n%s", b.String())
					}
				})
			}

			if tc.ng != "" {
				t.Run("ng", func(t *testing.T) {
					r, err := scenarigo.NewRunner(scenarigo.WithScenarios(tc.ng))
					if err != nil {
						t.Fatalf("unexpected error: %s", err)
					}

					ok := reporter.Run(func(rptr reporter.Reporter) {
						r.Run(context.New(rptr).WithPluginDir("testdata/gen/plugins"))
					})
					if ok {
						t.Fatalf("expect failure but no error")
					}
				})
			}
		})
	}
}

func TestRunner_GenerateTestReport(t *testing.T) {
	teardown := startHTTPServer(t)
	defer teardown()

	r, err := scenarigo.NewRunner(scenarigo.WithScenarios("testdata/scenarios/report.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var b bytes.Buffer
	ok := reporter.Run(func(rptr reporter.Reporter) {
		r.Run(context.New(rptr).WithPluginDir("testdata/gen/plugins"))

		report, err := reporter.GenerateTestReport(rptr)
		if err != nil {
			t.Fatalf("failed to generate report: %s", err)
		}

		if diff := cmp.Diff(
			&reporter.TestReport{
				Result: reporter.TestResultPassed,
				Files: []reporter.ScenarioFileReport{
					{
						Name:   "testdata/scenarios/report.yaml",
						Result: reporter.TestResultPassed,
						Scenarios: []reporter.ScenarioReport{
							{
								Name:   "/echo",
								File:   "testdata/scenarios/report.yaml",
								Result: reporter.TestResultPassed,
								Steps: []reporter.StepReport{
									{
										Name:   "include",
										Result: reporter.TestResultPassed,
										SubSteps: []reporter.SubStepReport{
											{
												Name:   "included.yaml",
												Result: reporter.TestResultPassed,
												SubSteps: []reporter.SubStepReport{
													{
														Name:   "step plugin",
														Result: reporter.TestResultPassed,
													},
												},
											},
										},
									},
									{
										Name:   "POST /echo",
										Result: reporter.TestResultPassed,
									},
								},
							},
						},
					},
				},
			},
			report,
			cmp.FilterValues(func(_, _ reporter.TestDuration) bool {
				return true
			}, cmp.Ignore()),
			cmp.FilterValues(func(_, _ reporter.ReportLogs) bool {
				return true
			}, cmp.Ignore()),
		); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}, reporter.WithWriter(&b))
	if !ok {
		t.Fatalf("scenario failed:\n%s", b.String())
	}
}

func startHTTPServer(t *testing.T) func() {
	t.Helper()
	token := "XXXXX"
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != fmt.Sprintf("Bearer %s", token) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.Header.Get("Content-Type") {
		case "application/x-www-form-urlencoded":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse form: %s", err)
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(strings.Join([]string{
				r.Form.Get("id"),
				r.Form.Get("message"),
				r.Form.Get("bool"),
			}, ", ")))
		default:
			body := map[string]string{}
			d := json.NewDecoder(r.Body)
			defer r.Body.Close()
			if err := d.Decode(&body); err != nil {
				t.Fatalf("failed to decode request body: %s", err)
			}
			b, err := json.Marshal(map[string]string{
				"message": body["message"],
				"id":      r.URL.Query().Get("id"),
			})
			if err != nil {
				t.Fatalf("failed to marshal: %s", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(b)
		}
	})

	s := httptest.NewServer(mux)
	if err := os.Setenv("TEST_ADDR", s.URL); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := os.Setenv("TEST_TOKEN", token); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	return func() {
		s.Close()
		os.Unsetenv("TEST_ADDR")
		os.Unsetenv("TEST_TOKEN")
	}
}

// +build !race

package scenarigo

import (
	"bytes"
	gocontext "context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/sergi/go-diff/diffmatchpatch"
	"google.golang.org/grpc"

	"github.com/zoncoen/scenarigo"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/logger"
	"github.com/zoncoen/scenarigo/internal/testutil"
	"github.com/zoncoen/scenarigo/mock"
	"github.com/zoncoen/scenarigo/mock/protocol"
	"github.com/zoncoen/scenarigo/reporter"
	"github.com/zoncoen/scenarigo/testdata/gen/pb/test"
)

func TestE2E(t *testing.T) {
	dir := "testdata/testcases"
	infos, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	files := []string{}
	for _, info := range infos {
		if info.IsDir() {
			continue
		}
		if strings.HasSuffix(info.Name(), ".yaml") {
			files = append(files, filepath.Join(dir, info.Name()))
		}
	}

	teardown := startGRPCServer(t)
	defer teardown()

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			f, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			var tc TestCase
			if err := yaml.NewDecoder(f).Decode(&tc); err != nil {
				t.Fatal(err)
			}

			for _, scenario := range tc.Scenarios {
				t.Run(scenario.Filename, func(t *testing.T) {
					r, err := scenarigo.NewRunner(scenarigo.WithScenarios(filepath.Join(dir, "scenarios", scenario.Filename)))
					if err != nil {
						t.Fatal(err)
					}

					// create mock server
					if scenario.Mocks != "" {
						if err := runMockServer(t, filepath.Join(dir, "mocks", scenario.Mocks), !scenario.Success); err != nil {
							t.Fatalf("failed to start mock server: %s", err)
						}
					}

					var b bytes.Buffer
					ok := reporter.Run(func(rptr reporter.Reporter) {
						r.Run(context.New(rptr).WithPluginDir("testdata/gen/plugins"))
					}, reporter.WithWriter(&b))
					if ok != scenario.Success {
						t.Errorf("expect %t but got %t", scenario.Success, ok)
					}

					f, err := os.Open(filepath.Join(dir, "stdout", scenario.Output.Stdout))
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()

					stdout, err := io.ReadAll(f)
					if err != nil {
						t.Fatal(err)
					}

					if got, expect := testutil.ReplaceOutput(b.String()), string(stdout); got != expect {
						dmp := diffmatchpatch.New()
						diffs := dmp.DiffMain(expect, got, false)
						t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
					}
				})
			}
		})
	}
}

type TestCase struct {
	Tilte     string         `yaml:"title"`
	Scenarios []TestScenario `yaml:"scenarios"`
}

type TestScenario struct {
	Filename string       `yaml:"filename"`
	Mocks    string       `yaml:"mocks"`
	Success  bool         `yaml:"success"`
	Output   ExpectOutput `yaml:"output"`
}

type ExpectOutput struct {
	Stdout string `yaml:"stdout"`
}

func runMockServer(t *testing.T, filename string, ignoreMocksRemainError bool) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	var config mock.ServerConfig
	if err := yaml.NewDecoder(f, yaml.Strict()).Decode(&config); err != nil {
		return err
	}
	srv, err := mock.NewServer(&config, logger.NewNopLogger())
	if err != nil {
		return err
	}
	ch := make(chan error)
	go func() {
		ch <- srv.Start(gocontext.Background())
	}()
	ctx, cancel := gocontext.WithTimeout(gocontext.Background(), time.Second)
	defer cancel()
	if err := srv.Wait(ctx); err != nil {
		t.Fatalf("failed to wait: %s", err)
	}
	addrs, err := srv.Addrs()
	if err != nil {
		return err
	}
	for p, addr := range addrs {
		os.Setenv(fmt.Sprintf("TEST_%s_ADDR", strings.ToUpper(p)), addr)
	}
	t.Cleanup(func() {
		ctx, cancel := gocontext.WithTimeout(gocontext.Background(), time.Second)
		defer cancel()
		if err := srv.Stop(ctx); err != nil {
			mrerr := &protocol.MocksRemainError{}
			if errors.As(err, &mrerr) {
				if ignoreMocksRemainError {
					err = nil
				}
			}
			if err != nil {
				t.Fatalf("failed to stop: %s", err)
			}
		}
		if err := <-ch; err != nil {
			t.Fatalf("failed to start: %s", err)
		}
	})
	return nil
}

func startGRPCServer(t *testing.T) func() {
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

	if err := os.Setenv("TEST_GRPC_SERVER_ADDR", ln.Addr().String()); err != nil {
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
		os.Unsetenv("TEST_GRPC_SERVER_ADDR")
		os.Unsetenv("TEST_TOKEN")
	}
}

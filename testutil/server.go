// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package testutil

// TestServer is a test helper. It uses a fork/exec model to create
// a test Nomad server instance in the background and initialize it
// with some data and/or services. The test server can then be used
// to run a unit test, and offers an easy API to tear itself down
// when the test has completed. The only prerequisite is to have a nomad
// binary available on the $PATH.
//
// This package does not use Nomad's official API client. This is
// because we use TestServer to test the API client, which would
// otherwise cause an import cycle.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/nomad/ci"
	"github.com/hashicorp/nomad/helper/discover"
	"github.com/hashicorp/nomad/helper/pointer"
)

// TestServerConfig is the main server configuration struct.
type TestServerConfig struct {
	NodeName          string         `json:"name,omitempty"`
	DataDir           string         `json:"data_dir,omitempty"`
	Region            string         `json:"region,omitempty"`
	DisableCheckpoint bool           `json:"disable_update_check"`
	LogLevel          string         `json:"log_level,omitempty"`
	Consuls           []*Consul      `json:"consul,omitempty"`
	AdvertiseAddrs    *Advertise     `json:"advertise,omitempty"`
	Ports             *PortsConfig   `json:"ports,omitempty"`
	Server            *ServerConfig  `json:"server,omitempty"`
	Client            *ClientConfig  `json:"client,omitempty"`
	Vaults            []*VaultConfig `json:"vault,omitempty"`
	ACL               *ACLConfig     `json:"acl,omitempty"`
	DevMode           bool           `json:"-"`
	DevConnectMode    bool           `json:"-"`
	Stdout, Stderr    io.Writer      `json:"-"`
}

// Consul is used to configure the communication with Consul
type Consul struct {
	Name                      string                  `json:"name,omitempty"`
	Address                   string                  `json:"address,omitempty"`
	Auth                      string                  `json:"auth,omitempty"`
	Token                     string                  `json:"token,omitempty"`
	ServiceIdentity           *WorkloadIdentityConfig `json:"service_identity,omitempty"`
	ServiceIdentityAuthMethod string                  `json:"service_auth_method,omitempty"`
	TaskIdentity              *WorkloadIdentityConfig `json:"task_identity,omitempty"`
	TaskIdentityAuthMethod    string                  `json:"task_auth_method,omitempty"`
}

// WorkloadIdentityConfig is the configuration for default workload identities.
type WorkloadIdentityConfig struct {
	Audience    []string          `json:"aud"`
	Env         bool              `json:"env"`
	File        bool              `json:"file"`
	TTL         string            `json:"ttl"`
	ExtraClaims map[string]string `json:"extra_claims,omitempty"`
}

// Advertise is used to configure the addresses to advertise
type Advertise struct {
	HTTP string `json:"http,omitempty"`
	RPC  string `json:"rpc,omitempty"`
	Serf string `json:"serf,omitempty"`
}

// PortsConfig is used to configure the network ports we use.
type PortsConfig struct {
	HTTP int `json:"http,omitempty"`
	RPC  int `json:"rpc,omitempty"`
	Serf int `json:"serf,omitempty"`
}

// ServerConfig is used to configure the nomad server.
type ServerConfig struct {
	Enabled         bool `json:"enabled"`
	BootstrapExpect int  `json:"bootstrap_expect"`
	RaftProtocol    int  `json:"raft_protocol,omitempty"`
}

// ClientConfig is used to configure the client
type ClientConfig struct {
	Enabled      bool `json:"enabled"`
	TotalCompute int  `json:"cpu_total_compute"`
}

// VaultConfig is used to configure Vault
type VaultConfig struct {
	Name                 string                  `json:"name,omitempty"`
	Enabled              bool                    `json:"enabled"`
	Address              string                  `json:"address"`
	AllowUnauthenticated *bool                   `json:"allow_unauthenticated,omitempty"`
	Token                string                  `json:"token,omitemtpy"`
	Role                 string                  `json:"role,omitempty"`
	JWTAuthBackendPath   string                  `json:"jwt_auth_backend_path,omitempty"`
	DefaultIdentity      *WorkloadIdentityConfig `json:"default_identity,omitempty"`
}

// ACLConfig is used to configure ACLs
type ACLConfig struct {
	Enabled        bool   `json:"enabled"`
	BootstrapToken string `json:"-"` // not in the real config
}

// ServerConfigCallback is a function interface which can be
// passed to NewTestServerConfig to modify the server config.
type ServerConfigCallback func(c *TestServerConfig)

// defaultServerConfig returns a new TestServerConfig struct
// with all of the listen ports incremented by one.
func defaultServerConfig() *TestServerConfig {
	ports := ci.PortAllocator.Grab(3)
	return &TestServerConfig{
		NodeName:          fmt.Sprintf("node-%d", ports[0]),
		DisableCheckpoint: true,
		LogLevel:          "DEBUG",
		Ports: &PortsConfig{
			HTTP: ports[0],
			RPC:  ports[1],
			Serf: ports[2],
		},
		Server: &ServerConfig{
			Enabled:         true,
			BootstrapExpect: 1,
		},
		Client: &ClientConfig{
			Enabled: false,
		},
		Vaults: []*VaultConfig{{
			Enabled:              false,
			AllowUnauthenticated: pointer.Of(true),
		}},
		ACL: &ACLConfig{
			Enabled: false,
		},
	}
}

// TestServer is the main server wrapper struct.
type TestServer struct {
	cmd    *exec.Cmd
	Config *TestServerConfig
	t      testing.TB

	HTTPAddr   string
	SerfAddr   string
	HTTPClient *http.Client
}

// NewTestServer creates a new TestServer, and makes a call to
// an optional callback function to modify the configuration.
func NewTestServer(t testing.TB, cb ServerConfigCallback) *TestServer {
	path, err := discover.NomadExecutable()
	if err != nil {
		t.Skipf("nomad not found, skipping: %v", err)
	}

	// Check that we are actually running nomad
	vcmd := exec.Command(path, "-version")
	vcmd.Stdout = nil
	vcmd.Stderr = nil
	if err := vcmd.Run(); err != nil {
		t.Skipf("nomad version failed: %v", err)
	}
	out, _ := vcmd.Output()
	t.Logf("nomad version: %s", out)

	dataDir, err := os.MkdirTemp("", "nomad")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	configFile, err := os.CreateTemp(dataDir, "nomad")
	if err != nil {
		defer os.RemoveAll(dataDir)
		t.Fatalf("err: %s", err)
	}
	defer configFile.Close()

	nomadConfig := defaultServerConfig()
	nomadConfig.DataDir = dataDir

	if cb != nil {
		cb(nomadConfig)
	}

	configContent, err := json.Marshal(nomadConfig)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if _, err := configFile.Write(configContent); err != nil {
		t.Fatalf("err: %s", err)
	}
	configFile.Close()

	stdout := io.Writer(os.Stdout)
	if nomadConfig.Stdout != nil {
		stdout = nomadConfig.Stdout
	}

	stderr := io.Writer(os.Stderr)
	if nomadConfig.Stderr != nil {
		stderr = nomadConfig.Stderr
	}
	t.Logf("CONFIG JSON: %s", string(configContent))

	args := []string{"agent", "-config", configFile.Name()}
	if nomadConfig.DevMode {
		args = append(args, "-dev")
	}
	if nomadConfig.DevConnectMode {
		args = append(args, "-dev-connect")
	}

	// Start the server
	cmd := exec.Command(path, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	client := cleanhttp.DefaultClient()

	server := &TestServer{
		Config: nomadConfig,
		cmd:    cmd,
		t:      t,

		HTTPAddr:   fmt.Sprintf("127.0.0.1:%d", nomadConfig.Ports.HTTP),
		SerfAddr:   fmt.Sprintf("127.0.0.1:%d", nomadConfig.Ports.Serf),
		HTTPClient: client,
	}

	// Wait for the server to be ready
	if nomadConfig.Server.Enabled && nomadConfig.Server.BootstrapExpect != 0 {
		server.waitForServers()
	} else {
		server.waitForAPI()
	}

	if nomadConfig.ACL.Enabled && nomadConfig.ACL.BootstrapToken != "" {
		server.bootstrapSelf()
	}

	// Wait for the client to be ready
	if nomadConfig.DevMode {
		server.waitForClient()
	}
	return server
}

// Stop stops the test Nomad server, and removes the Nomad data
// directory once we are done.
func (s *TestServer) Stop() {
	defer os.RemoveAll(s.Config.DataDir)

	// wait for the process to exit to be sure that the data dir can be
	// deleted on all platforms.
	done := make(chan struct{})
	go func() {
		defer close(done)

		s.cmd.Wait()
	}()

	// kill and wait gracefully
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		s.t.Errorf("err: %s", err)
	}

	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		s.t.Logf("timed out waiting for process to gracefully terminate")
	}

	if err := s.cmd.Process.Kill(); err != nil {
		s.t.Errorf("err: %s", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.t.Logf("timed out waiting for process to be killed")
	}

}

// bootstrapSelf bootstraps the ACL system from the provided token.
func (s *TestServer) bootstrapSelf() {

	contentType := "application/json"

	rootToken := s.Config.ACL.BootstrapToken
	body := struct{ BootstrapSecret string }{rootToken}
	buf, err := json.Marshal(body)
	if err != nil {
		s.t.Fatalf("err: %s", err)
	}

	resp, err := s.HTTPClient.Post(s.url("/v1/acl/bootstrap"), contentType, bytes.NewBuffer(buf))
	if err != nil {
		s.t.Fatalf("err: %s", err)
	}
	defer resp.Body.Close()
	if err := s.requireOK(resp); err != nil {
		s.t.Fatalf("err: %s", err)
	}
}

// waitForAPI waits for only the agent HTTP endpoint to start
// responding. This is an indication that the agent has started,
// but will likely return before a leader is elected.
func (s *TestServer) waitForAPI() {
	WaitForResult(func() (bool, error) {
		// Using this endpoint as it is does not have restricted access
		resp, err := s.HTTPClient.Get(s.url("/v1/metrics"))
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		if err := s.requireOK(resp); err != nil {
			return false, err
		}
		return true, nil
	}, func(err error) {
		defer s.Stop()
		s.t.Fatalf("err: %s", err)
	})
}

// waitForServers waits for the Nomad server's HTTP API to become available,
// and then waits for the keyring to be intialized. This implies a leader has
// been elected and Raft writes have occurred.
func (s *TestServer) waitForServers() {
	WaitForResult(func() (bool, error) {
		// Query the API and check the status code
		// Using this endpoint as it is does not have restricted access
		resp, err := s.HTTPClient.Get(s.url("/.well-known/jwks.json"))
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		if err := s.requireOK(resp); err != nil {
			return false, err
		}

		jwks := struct {
			Keys []interface{} `json:"keys"`
		}{}
		if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
			return false, fmt.Errorf("error decoding jwks response: %w", err)
		}
		if len(jwks.Keys) == 0 {
			return false, fmt.Errorf("no keys found")
		}

		return true, nil
	}, func(err error) {
		defer s.Stop()
		s.t.Fatalf("err: %s", err)
	})
}

// waitForClient waits for the Nomad client to be ready. The function returns
// immediately if the server is not in dev mode.
func (s *TestServer) waitForClient() {
	if !s.Config.DevMode {
		return
	}

	WaitForResult(func() (bool, error) {
		req, err := http.NewRequest(http.MethodGet, s.url("/v1/nodes"), nil)
		if err != nil {
			return false, err
		}
		if s.Config.ACL.BootstrapToken != "" {
			req.Header.Set("X-Nomad-Token", s.Config.ACL.BootstrapToken)
		}
		resp, err := s.HTTPClient.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		if err := s.requireOK(resp); err != nil {
			return false, err
		}

		var decoded []struct {
			ID     string
			Status string
		}

		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&decoded); err != nil {
			return false, err
		}

		if len(decoded) != 1 || decoded[0].Status != "ready" {
			return false, fmt.Errorf("Node not ready: %v", decoded)
		}

		return true, nil
	}, func(err error) {
		defer s.Stop()
		s.t.Fatalf("err: %s", err)
	})
}

// url is a helper function which takes a relative URL and
// makes it into a proper URL against the local Nomad server.
func (s *TestServer) url(path string) string {
	return fmt.Sprintf("http://%s%s", s.HTTPAddr, path)
}

// requireOK checks the HTTP response code and ensures it is acceptable.
func (s *TestServer) requireOK(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bad status code: %d", resp.StatusCode)
	}
	return nil
}

// put performs a new HTTP PUT request.
func (s *TestServer) put(path string, body io.Reader) *http.Response {
	req, err := http.NewRequest(http.MethodPut, s.url(path), body)
	if err != nil {
		s.t.Fatalf("err: %s", err)
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		s.t.Fatalf("err: %s", err)
	}
	if err := s.requireOK(resp); err != nil {
		defer resp.Body.Close()
		s.t.Fatal(err)
	}
	return resp
}

// get performs a new HTTP GET request.
func (s *TestServer) get(path string) *http.Response {
	resp, err := s.HTTPClient.Get(s.url(path))
	if err != nil {
		s.t.Fatalf("err: %s", err)
	}
	if err := s.requireOK(resp); err != nil {
		defer resp.Body.Close()
		s.t.Fatal(err)
	}
	return resp
}

// encodePayload returns a new io.Reader wrapping the encoded contents
// of the payload, suitable for passing directly to a new request.
func (s *TestServer) encodePayload(payload interface{}) io.Reader {
	var encoded bytes.Buffer
	enc := json.NewEncoder(&encoded)
	if err := enc.Encode(payload); err != nil {
		s.t.Fatalf("err: %s", err)
	}
	return &encoded
}

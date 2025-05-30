// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-secure-stdlib/listenerutil"

	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/hashicorp/nomad/nomad/structs"
)

// ConsulConfig contains the configuration information necessary to
// communicate with a Consul Agent in order to:
//
// - Register services and their checks with Consul
//
//   - Bootstrap this Nomad Client with the list of Nomad Servers registered
//     with Consul
//
//   - Establish how this Nomad Client will resolve Envoy Connect Sidecar
//     images.
//
// Both the Agent and the executor need to be able to import ConsulConfig.
type ConsulConfig struct {
	Name string `mapstructure:"name"`

	// ServerServiceName is the name of the service that Nomad uses to register
	// servers with Consul
	ServerServiceName string `mapstructure:"server_service_name"`

	// ServerHTTPCheckName is the name of the health check that Nomad uses
	// to register the server HTTP health check with Consul
	ServerHTTPCheckName string `mapstructure:"server_http_check_name"`

	// ServerSerfCheckName is the name of the health check that Nomad uses
	// to register the server Serf health check with Consul
	ServerSerfCheckName string `mapstructure:"server_serf_check_name"`

	// ServerRPCCheckName is the name of the health check that Nomad uses
	// to register the server RPC health check with Consul
	ServerRPCCheckName string `mapstructure:"server_rpc_check_name"`

	// ServerFailuresBeforeCritical is the number of failures before the
	// server health check is marked as critical
	ServerFailuresBeforeCritical int `mapstructure:"server_failures_before_critical"`

	// ServerFailuresBeforeWarning is the number of failures before the
	// server health check is marked as a warning
	ServerFailuresBeforeWarning int `mapstructure:"server_failures_before_warning"`

	// ClientServiceName is the name of the service that Nomad uses to register
	// clients with Consul
	ClientServiceName string `mapstructure:"client_service_name"`

	// ClientHTTPCheckName is the name of the health check that Nomad uses
	// to register the client HTTP health check with Consul
	ClientHTTPCheckName string `mapstructure:"client_http_check_name"`

	// ClientFailuresBeforeCritical is the number of failures before the
	// client health check is marked as critical
	ClientFailuresBeforeCritical int `mapstructure:"client_failures_before_critical"`

	// ClientFailuresBeforeWarning is the number of failures before the
	// client health check is marked as a warning
	ClientFailuresBeforeWarning int `mapstructure:"client_failures_before_warning"`

	// Tags are optional service tags that get registered with the service
	// in Consul
	Tags []string `mapstructure:"tags"`

	// AutoAdvertise determines if this Nomad Agent will advertise its
	// services via Consul.  When true, Nomad Agent will register
	// services with Consul.
	AutoAdvertise *bool `mapstructure:"auto_advertise"`

	// ChecksUseAdvertise specifies that Consul checks should use advertise
	// address instead of bind address
	ChecksUseAdvertise *bool `mapstructure:"checks_use_advertise"`

	// Addr is the HTTP endpoint address of the local Consul agent
	//
	// Uses Consul's default and env var.
	Addr string `mapstructure:"address"`

	// GRPCAddr is the gRPC endpoint address of the local Consul agent
	GRPCAddr string `mapstructure:"grpc_address"`

	// Timeout is used by Consul HTTP Client
	Timeout    time.Duration `mapstructure:"-"`
	TimeoutHCL string        `mapstructure:"timeout" json:"-"`

	// Token is used by clients to deregisters services and by servers
	// to create configuration entries for Consul Service Mesh
	Token string `mapstructure:"token"`

	// Auth is the information to use for http access to Consul agent
	Auth string `mapstructure:"auth"`

	// EnableSSL sets the transport scheme to talk to the Consul agent as https
	//
	// Uses Consul's default and env var.
	EnableSSL *bool `mapstructure:"ssl"`

	// ShareSSL enables Consul Connect Native applications to use the TLS
	// configuration of the Nomad Client for establishing connections to Consul.
	//
	// Does not include sharing of ACL tokens.
	ShareSSL *bool `mapstructure:"share_ssl"`

	// VerifySSL enables or disables SSL verification when the transport scheme
	// for the consul api client is https
	//
	// Uses Consul's default and env var.
	VerifySSL *bool `mapstructure:"verify_ssl"`

	// GRPCCAFile is the path to the ca certificate used for Consul gRPC communication.
	//
	// Uses Consul's default and env var.
	GRPCCAFile string `mapstructure:"grpc_ca_file"`

	// CAFile is the path to the ca certificate used for Consul communication.
	//
	// Uses Consul's default and env var.
	CAFile string `mapstructure:"ca_file"`

	// CertFile is the path to the certificate for Consul communication
	CertFile string `mapstructure:"cert_file"`

	// KeyFile is the path to the private key for Consul communication
	KeyFile string `mapstructure:"key_file"`

	// ServerAutoJoin enables Nomad servers to find peers by querying Consul and
	// joining them
	ServerAutoJoin *bool `mapstructure:"server_auto_join"`

	// ClientAutoJoin enables Nomad servers to find addresses of Nomad servers
	// and register with them
	ClientAutoJoin *bool `mapstructure:"client_auto_join"`

	// Namespace sets the Consul namespace used for all calls against the
	// Consul API. If this is unset, then Nomad does not specify a consul namespace.
	Namespace string `mapstructure:"namespace"`

	// ServiceIdentity is intended to reduce overhead for jobspec authors and make
	// for graceful upgrades without forcing rewrite of all jobspecs. If set, when a
	// job has a service block with the "consul" provider, the Nomad server will sign
	// a Workload Identity for that service and add it to the service block. The
	// client will use this identity rather than the client's Consul token for the
	// group_service and envoy_bootstrap_hook.
	//
	// The name field of the identity is always set to
	// "consul-service/${service_task_name}-${service_name}-${service_port}" or
	// "consul-service/${service_name}-${service_port}".
	//
	// ServiceIdentity is set on the server.
	ServiceIdentity *WorkloadIdentityConfig `mapstructure:"service_identity"`

	// ServiceIdentityAuthMethod is the name of the Consul authentication method
	// that will be used to login with a Nomad JWT for services.
	ServiceIdentityAuthMethod string `mapstructure:"service_auth_method"`

	// TaskIdentity is intended to reduce overhead for jobspec authors and make
	// for graceful upgrades without forcing rewrite of all jobspecs. If set, when a
	// job has both a template block and a consul block, the Nomad server will sign a
	// Workload Identity for that task. The client will use this identity rather than
	// the client's Consul token for the template hook.
	//
	// The name field of the identity is always set to "consul_$clusterName".
	//
	// TaskIdentity is set on the server.
	TaskIdentity *WorkloadIdentityConfig `mapstructure:"task_identity"`

	// TaskIdentityAuthMethod is the name of the Consul authentication method
	// that will be used to login with a Nomad JWT for tasks.
	TaskIdentityAuthMethod string `mapstructure:"task_auth_method"`

	// ExtraKeysHCL is used by hcl to surface unexpected keys
	ExtraKeysHCL []string `mapstructure:",unusedKeys" json:"-"`
}

// DefaultConsulConfig returns the canonical defaults for the Nomad
// `consul` configuration. Uses Consul's default configuration which reads
// environment variables.
func DefaultConsulConfig() *ConsulConfig {
	def := consul.DefaultConfig()
	return &ConsulConfig{
		Name:                      "default",
		ServerServiceName:         "nomad",
		ServerHTTPCheckName:       "Nomad Server HTTP Check",
		ServerSerfCheckName:       "Nomad Server Serf Check",
		ServerRPCCheckName:        "Nomad Server RPC Check",
		ClientServiceName:         "nomad-client",
		ClientHTTPCheckName:       "Nomad Client HTTP Check",
		AutoAdvertise:             pointer.Of(true),
		ChecksUseAdvertise:        pointer.Of(false),
		ServerAutoJoin:            pointer.Of(true),
		ClientAutoJoin:            pointer.Of(true),
		Timeout:                   5 * time.Second,
		ServiceIdentityAuthMethod: structs.ConsulWorkloadsDefaultAuthMethodName,
		TaskIdentityAuthMethod:    structs.ConsulWorkloadsDefaultAuthMethodName,

		// From Consul api package defaults
		Addr:      def.Address,
		EnableSSL: pointer.Of(def.Scheme == "https"),
		VerifySSL: pointer.Of(!def.TLSConfig.InsecureSkipVerify),
		CAFile:    def.TLSConfig.CAFile,
		Namespace: def.Namespace,
	}
}

// Merge merges two Consul Configurations together.
func (c *ConsulConfig) Merge(b *ConsulConfig) *ConsulConfig {
	result := c.Copy()

	if b.Name != "" {
		result.Name = b.Name
	}
	if b.ServerServiceName != "" {
		result.ServerServiceName = b.ServerServiceName
	}
	if b.ServerHTTPCheckName != "" {
		result.ServerHTTPCheckName = b.ServerHTTPCheckName
	}
	if b.ServerSerfCheckName != "" {
		result.ServerSerfCheckName = b.ServerSerfCheckName
	}
	if b.ServerRPCCheckName != "" {
		result.ServerRPCCheckName = b.ServerRPCCheckName
	}
	if b.ServerFailuresBeforeCritical != 0 {
		result.ServerFailuresBeforeCritical = b.ServerFailuresBeforeCritical
	}
	if b.ServerFailuresBeforeWarning != 0 {
		result.ServerFailuresBeforeWarning = b.ServerFailuresBeforeWarning
	}
	if b.ClientServiceName != "" {
		result.ClientServiceName = b.ClientServiceName
	}
	if b.ClientHTTPCheckName != "" {
		result.ClientHTTPCheckName = b.ClientHTTPCheckName
	}
	if b.ClientFailuresBeforeCritical != 0 {
		result.ClientFailuresBeforeCritical = b.ClientFailuresBeforeCritical
	}
	if b.ClientFailuresBeforeWarning != 0 {
		result.ClientFailuresBeforeWarning = b.ClientFailuresBeforeWarning
	}
	result.Tags = append(result.Tags, b.Tags...)
	if b.AutoAdvertise != nil {
		result.AutoAdvertise = pointer.Of(*b.AutoAdvertise)
	}
	if b.Addr != "" {
		result.Addr = b.Addr
	}
	if b.GRPCAddr != "" {
		result.GRPCAddr = b.GRPCAddr
	}
	if b.Timeout != 0 {
		result.Timeout = b.Timeout
	}
	if b.TimeoutHCL != "" {
		result.TimeoutHCL = b.TimeoutHCL
	}
	if b.Token != "" {
		result.Token = b.Token
	}
	if b.Auth != "" {
		result.Auth = b.Auth
	}
	if b.EnableSSL != nil {
		result.EnableSSL = pointer.Of(*b.EnableSSL)
	}
	if b.VerifySSL != nil {
		result.VerifySSL = pointer.Of(*b.VerifySSL)
	}
	if b.ShareSSL != nil {
		result.ShareSSL = pointer.Of(*b.ShareSSL)
	}
	if b.GRPCCAFile != "" {
		result.GRPCCAFile = b.GRPCCAFile
	}
	if b.CAFile != "" {
		result.CAFile = b.CAFile
	}
	if b.CertFile != "" {
		result.CertFile = b.CertFile
	}
	if b.KeyFile != "" {
		result.KeyFile = b.KeyFile
	}
	if b.ServerAutoJoin != nil {
		result.ServerAutoJoin = pointer.Of(*b.ServerAutoJoin)
	}
	if b.ClientAutoJoin != nil {
		result.ClientAutoJoin = pointer.Of(*b.ClientAutoJoin)
	}
	if b.ChecksUseAdvertise != nil {
		result.ChecksUseAdvertise = pointer.Of(*b.ChecksUseAdvertise)
	}
	if b.Namespace != "" {
		result.Namespace = b.Namespace
	}
	if b.ServiceIdentityAuthMethod != "" {
		result.ServiceIdentityAuthMethod = b.ServiceIdentityAuthMethod
	}
	if b.TaskIdentityAuthMethod != "" {
		result.TaskIdentityAuthMethod = b.TaskIdentityAuthMethod
	}

	if result.ServiceIdentity == nil && b.ServiceIdentity != nil {
		sID := *b.ServiceIdentity
		result.ServiceIdentity = &sID
	} else if b.ServiceIdentity != nil {
		result.ServiceIdentity = result.ServiceIdentity.Merge(b.ServiceIdentity)
	}

	if result.TaskIdentity == nil && b.TaskIdentity != nil {
		tID := *b.TaskIdentity
		result.TaskIdentity = &tID
	} else if b.TaskIdentity != nil {
		result.TaskIdentity = result.TaskIdentity.Merge(b.TaskIdentity)
	}

	return result
}

// ApiConfig returns a usable Consul config that can be passed directly to
// hashicorp/consul/api.  NOTE: datacenter is not set
func (c *ConsulConfig) ApiConfig() (*consul.Config, error) {
	// Get the default config from consul to reuse things like the default
	// http.Transport.
	config := consul.DefaultConfig()
	if c.Addr != "" {
		ipStr, err := listenerutil.ParseSingleIPTemplate(c.Addr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse address template %q: %v", c.Addr, err)
		}
		config.Address = ipStr
	}
	if c.Token != "" {
		config.Token = c.Token
	}
	if c.Timeout != 0 {
		// Create a custom Client to set the timeout
		if config.HttpClient == nil {
			config.HttpClient = &http.Client{}
		}
		config.HttpClient.Timeout = c.Timeout
		config.HttpClient.Transport = config.Transport
	}
	if c.Auth != "" {
		var username, password string
		if strings.Contains(c.Auth, ":") {
			split := strings.SplitN(c.Auth, ":", 2)
			username = split[0]
			password = split[1]
		} else {
			username = c.Auth
		}

		config.HttpAuth = &consul.HttpBasicAuth{
			Username: username,
			Password: password,
		}
	}
	if c.EnableSSL != nil && *c.EnableSSL {
		config.Scheme = "https"
		config.TLSConfig = consul.TLSConfig{
			Address:  config.Address,
			CAFile:   c.CAFile,
			CertFile: c.CertFile,
			KeyFile:  c.KeyFile,
		}
		if c.VerifySSL != nil {
			config.TLSConfig.InsecureSkipVerify = !*c.VerifySSL
		}
		tlsConfig, err := consul.SetupTLSConfig(&config.TLSConfig)
		if err != nil {
			return nil, err
		}
		config.Transport.TLSClientConfig = tlsConfig
	}
	if c.Namespace != "" {
		config.Namespace = c.Namespace
	}
	return config, nil
}

// Copy returns a copy of this Consul config.
func (c *ConsulConfig) Copy() *ConsulConfig {
	if c == nil {
		return nil
	}

	return &ConsulConfig{
		Name:                         c.Name,
		ServerServiceName:            c.ServerServiceName,
		ServerHTTPCheckName:          c.ServerHTTPCheckName,
		ServerSerfCheckName:          c.ServerSerfCheckName,
		ServerRPCCheckName:           c.ServerRPCCheckName,
		ServerFailuresBeforeCritical: c.ServerFailuresBeforeCritical,
		ServerFailuresBeforeWarning:  c.ServerFailuresBeforeWarning,
		ClientServiceName:            c.ClientServiceName,
		ClientHTTPCheckName:          c.ClientHTTPCheckName,
		ClientFailuresBeforeCritical: c.ClientFailuresBeforeCritical,
		ClientFailuresBeforeWarning:  c.ClientFailuresBeforeWarning,
		Tags:                         slices.Clone(c.Tags),
		AutoAdvertise:                c.AutoAdvertise,
		ChecksUseAdvertise:           c.ChecksUseAdvertise,
		Addr:                         c.Addr,
		GRPCAddr:                     c.GRPCAddr,
		Timeout:                      c.Timeout,
		TimeoutHCL:                   c.TimeoutHCL,
		Token:                        c.Token,
		Auth:                         c.Auth,
		EnableSSL:                    c.EnableSSL,
		ShareSSL:                     c.ShareSSL,
		VerifySSL:                    c.VerifySSL,
		GRPCCAFile:                   c.GRPCCAFile,
		CAFile:                       c.CAFile,
		CertFile:                     c.CertFile,
		KeyFile:                      c.KeyFile,
		ServerAutoJoin:               c.ServerAutoJoin,
		ClientAutoJoin:               c.ClientAutoJoin,
		Namespace:                    c.Namespace,
		ServiceIdentity:              c.ServiceIdentity.Copy(),
		TaskIdentity:                 c.TaskIdentity.Copy(),
		ServiceIdentityAuthMethod:    c.ServiceIdentityAuthMethod,
		TaskIdentityAuthMethod:       c.TaskIdentityAuthMethod,
		ExtraKeysHCL:                 slices.Clone(c.ExtraKeysHCL),
	}
}

package tfschema

import (
	"fmt"
	"os/exec"
	"sort"

	plugin "github.com/hashicorp/go-plugin"
	tfplugin "github.com/hashicorp/terraform/plugin"
	"github.com/hashicorp/terraform/plugin/discovery"
	tfplugin6 "github.com/hashicorp/terraform/plugin6"
	"github.com/hashicorp/terraform/providers"
)

// GRPCClient implements Client interface.
// This implementaion is for Terraform v0.12+.
type GRPCClient struct {
	// provider is a provider interface of Terraform.
	provider providers.Interface
}

// NewGRPCClient creates a new GRPCClient instance.
func NewGRPCClient(providerName string, options Option) (Client, error) {
	// find a provider plugin
	pluginMeta, err := findPlugin("provider", providerName, options.RootDir)
	if err != nil {
		return nil, err
	}

	// create a plugin client config
	config := newGRPCClientConfig(pluginMeta, options)

	// initialize a plugin client.
	client := plugin.NewClient(config)
	rpcClient, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize GRPC plugin: %s", err)
	}

	// create a new GRPCProvider.
	raw, err := rpcClient.Dispense(tfplugin.ProviderPluginName)
	if err != nil {
		return nil, fmt.Errorf("Failed to dispense GRPC plugin: %s", err)
	}

	// To clean up the plugin process, we need to explicitly store references.
	protocolVersion := client.NegotiatedVersion()
	switch protocolVersion {
	// For Terraform v0.12+
	case 5:
		provider := raw.(*tfplugin.GRPCProvider)
		provider.PluginClient = client
		return &GRPCClient{
			provider: provider,
		}, nil

	// For Terraform v0.15+
	case 6:
		provider := raw.(*tfplugin6.GRPCProvider)
		provider.PluginClient = client
		return &GRPCClient{
			provider: provider,
		}, nil

	default:
		return nil, fmt.Errorf("unknown protocol version: %d", protocolVersion)
	}
}

// newGRPCClientConfig returns a default plugin client config for Terraform v0.12+.
func newGRPCClientConfig(pluginMeta *discovery.PluginMeta, options Option) *plugin.ClientConfig {
	return &plugin.ClientConfig{
		HandshakeConfig:  tfplugin.Handshake,
		Logger:           options.Logger,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Managed:          true,
		Cmd:              exec.Command(pluginMeta.Path),
		AutoMTLS:         true,
		VersionedPlugins: tfplugin.VersionedPlugins,
	}
}

// getSchema is a helper function to get a schema from provider.
func (c *GRPCClient) getSchema() (providers.GetProviderSchemaResponse, error) {
	res := c.provider.GetProviderSchema()
	if res.Diagnostics.HasErrors() {
		return res, fmt.Errorf("Failed to get schema from provider: %s", res.Diagnostics.Err())
	}

	return res, nil
}

// GetProviderSchema returns a type definiton of provider schema.
func (c *GRPCClient) GetProviderSchema() (*Block, error) {
	res, err := c.getSchema()
	if err != nil {
		return nil, err
	}

	b := NewBlock(res.Provider.Block)
	return b, nil
}

// GetResourceTypeSchema returns a type definiton of resource type.
func (c *GRPCClient) GetResourceTypeSchema(resourceType string) (*Block, error) {
	res, err := c.getSchema()
	if err != nil {
		return nil, err
	}

	schema, ok := res.ResourceTypes[resourceType]
	if !ok {
		return nil, fmt.Errorf("Failed to find resource type: %s", resourceType)
	}

	b := NewBlock(schema.Block)
	return b, nil
}

// GetDataSourceSchema returns a type definiton of data source.
func (c *GRPCClient) GetDataSourceSchema(dataSource string) (*Block, error) {
	res, err := c.getSchema()
	if err != nil {
		return nil, err
	}

	schema, ok := res.DataSources[dataSource]
	if !ok {
		return nil, fmt.Errorf("Failed to find data source: %s", dataSource)
	}

	b := NewBlock(schema.Block)
	return b, nil
}

// ResourceTypes returns a list of resource types.
func (c *GRPCClient) ResourceTypes() ([]string, error) {
	res, err := c.getSchema()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(res.ResourceTypes))
	for k := range res.ResourceTypes {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	return keys, nil
}

// DataSources returns a list of data sources.
func (c *GRPCClient) DataSources() ([]string, error) {
	res, err := c.getSchema()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(res.DataSources))
	for k := range res.DataSources {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	return keys, nil
}

// Close closes a connection and kills a process of the plugin.
func (c *GRPCClient) Close() {
	c.provider.Close()
}

// Package kubeconfig implements the kubeconfig provider plugin. It owns the
// client-go/clientcmd work so scafctl core never imports heavy Kubernetes
// client packages: merging/writing exec-credential kubeconfig entries, removing
// them, reading the current server, detecting the cluster auth style, checking
// reachability, and running a SelfSubjectReview whoami.
//
// All operations dispatch on the "operation" input field and every response
// carries a boolean "success" field, as required by CapabilityKubeconfig.
package kubeconfig

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
	sdkplugin "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	sdkprovider "github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	sdkhelper "github.com/oakwood-commons/scafctl-plugin-sdk/provider/schemahelper"
)

const (
	// ProviderName is the unique identifier for this provider.
	ProviderName = "kubeconfig"

	// Version is the provider version.
	Version = "0.1.0"
)

// Plugin implements the scafctl ProviderPlugin interface.
type Plugin struct{}

// GetProviders returns the list of providers exposed by this plugin.
//
//nolint:revive // ctx required by interface
func (p *Plugin) GetProviders(_ context.Context) ([]string, error) {
	return []string{ProviderName}, nil
}

// GetProviderDescriptor returns the descriptor for the named provider.
//
//nolint:revive // ctx required by interface
func (p *Plugin) GetProviderDescriptor(_ context.Context, providerName string) (*sdkprovider.Descriptor, error) {
	if providerName != ProviderName {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}

	return &sdkprovider.Descriptor{
		Name:        ProviderName,
		DisplayName: "Kubeconfig Provider",
		Description: "Kubeconfig and cluster operations provider for scafctl",
		APIVersion:  "v1",
		Version:     semver.MustParse(Version),
		Category:    "custom",
		Capabilities: []sdkprovider.Capability{
			sdkprovider.CapabilityKubeconfig,
		},
		WriteOperations: []string{OpWrite, OpRemove},
		Schema:          inputSchema(),
		OutputSchemas: map[sdkprovider.Capability]*jsonschema.Schema{
			sdkprovider.CapabilityKubeconfig: outputSchema(),
		},
	}, nil
}

// inputSchema describes the union of fields across all operations. The only
// required field is "operation"; the rest are operation-specific.
func inputSchema() *jsonschema.Schema {
	return sdkhelper.ObjectSchema(
		[]string{"operation"},
		map[string]*jsonschema.Schema{
			"operation": sdkhelper.StringProp(
				"Operation to perform",
				sdkhelper.WithEnum(OpWrite, OpRemove, OpCurrentServer, OpDetectAuth, OpReachable, OpWhoami),
			),
			"server":              sdkhelper.StringProp("Cluster API server URL"),
			"audience":            sdkhelper.StringProp("Token audience (accepted for contract parity; not used by this provider)"),
			"cluster_name":        sdkhelper.StringProp("Kubeconfig cluster name"),
			"context_name":        sdkhelper.StringProp("Kubeconfig context name"),
			"user_name":           sdkhelper.StringProp("Kubeconfig user name"),
			"kubeconfig_path":     sdkhelper.StringProp("Path to kubeconfig (empty resolves KUBECONFIG or ~/.kube/config)"),
			"exec_command":        sdkhelper.StringProp("Exec-credential command baked into the user entry"),
			"exec_args":           sdkhelper.ArrayProp("Exec-credential command arguments", sdkhelper.WithItems(&jsonschema.Schema{Type: "string"})),
			"token":               sdkhelper.StringProp("Bearer token (whoami only; never cached)", sdkhelper.WithWriteOnly()),
			"insecure_skip_tls":   sdkhelper.BoolProp("Skip TLS verification (development only)"),
			"set_current_context": sdkhelper.BoolProp("Set the written context as current-context"),
		},
	)
}

// outputSchema describes the union of output fields. "success" is mandated by
// CapabilityKubeconfig validation.
func outputSchema() *jsonschema.Schema {
	return sdkhelper.ObjectSchema(
		[]string{"success"},
		map[string]*jsonschema.Schema{
			"success":         {Type: "boolean"},
			"error":           {Type: "string"},
			"context_name":    {Type: "string"},
			"kubeconfig_path": {Type: "string"},
			"removed":         {Type: "boolean"},
			"server":          {Type: "string"},
			"auth_type":       {Type: "string"},
			"oidc_issuer":     {Type: "string"},
			"oauth_endpoint":  {Type: "string"},
			"reachable":       {Type: "boolean"},
			"status":          {Type: "integer"},
			"username":        {Type: "string"},
			"groups":          {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			"uid":             {Type: "string"},
		},
	)
}

// ExecuteProvider executes the named provider, dispatching on the "operation"
// input field.
func (p *Plugin) ExecuteProvider(ctx context.Context, providerName string, input map[string]any) (*sdkprovider.Output, error) {
	if providerName != ProviderName {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}

	operation := stringField(input, "operation")
	data, err := dispatch(ctx, operation, input)
	if err != nil {
		return nil, err
	}
	return &sdkprovider.Output{Data: data}, nil
}

// dispatch routes an operation to its handler and returns the flat output map.
func dispatch(ctx context.Context, operation string, input map[string]any) (map[string]any, error) {
	switch operation {
	case OpWrite:
		return opWrite(parseWriteInput(input))
	case OpRemove:
		return opRemove(parseRemoveInput(input))
	case OpCurrentServer:
		return opCurrentServer(parseCurrentServerInput(input))
	case OpDetectAuth:
		return opDetectAuth(ctx, parseDetectInput(input))
	case OpReachable:
		return opReachable(ctx, parseReachableInput(input))
	case OpWhoami:
		return opWhoami(ctx, parseWhoamiInput(input))
	case "":
		return nil, fmt.Errorf("missing required input %q", "operation")
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// DescribeWhatIf returns a description of what the provider would do.
//
//nolint:revive // ctx required by interface
func (p *Plugin) DescribeWhatIf(_ context.Context, providerName string, input map[string]any) (string, error) {
	if providerName != ProviderName {
		return "", fmt.Errorf("unknown provider: %s", providerName)
	}

	operation := stringField(input, "operation")
	switch operation {
	case OpWrite:
		in := parseWriteInput(input)
		name := sanitizeName(in.ContextName, sanitizeName(in.ClusterName, "cluster"))
		return fmt.Sprintf("Would write kubeconfig context %q for server %q", name, in.Server), nil
	case OpRemove:
		in := parseRemoveInput(input)
		name := sanitizeName(in.ContextName, sanitizeName(in.ClusterName, "cluster"))
		return fmt.Sprintf("Would remove kubeconfig context %q", name), nil
	case OpCurrentServer:
		return "Would read the current cluster server from kubeconfig", nil
	case OpDetectAuth:
		return fmt.Sprintf("Would detect the auth type for server %q", stringField(input, "server")), nil
	case OpReachable:
		return fmt.Sprintf("Would check reachability of server %q", stringField(input, "server")), nil
	case OpWhoami:
		return fmt.Sprintf("Would run a SelfSubjectReview against server %q", stringField(input, "server")), nil
	case "":
		return "", fmt.Errorf("missing required input %q", "operation")
	default:
		return "", fmt.Errorf("unknown operation: %s", operation)
	}
}

// ConfigureProvider stores host-side configuration.
//
//nolint:revive // ctx and cfg required by interface
func (p *Plugin) ConfigureProvider(_ context.Context, _ string, _ sdkplugin.ProviderConfig) error {
	return nil
}

// ExecuteProviderStream is not supported.
//
//nolint:revive // all params required by interface
func (p *Plugin) ExecuteProviderStream(_ context.Context, _ string, _ map[string]any, _ func(sdkplugin.StreamChunk)) error {
	return sdkplugin.ErrStreamingNotSupported
}

// ExtractDependencies returns resolver keys this input depends on.
//
//nolint:revive // all params required by interface
func (p *Plugin) ExtractDependencies(_ context.Context, _ string, _ map[string]any) ([]string, error) {
	return nil, nil
}

// StopProvider performs cleanup for the named provider.
//
//nolint:revive // all params required by interface
func (p *Plugin) StopProvider(_ context.Context, _ string) error {
	return nil
}

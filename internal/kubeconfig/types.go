package kubeconfig

// Operation names dispatched on the "operation" input field. The host-side
// manager (pkg/kubeconfig) sends one of these with every Execute call.
const (
	OpWrite         = "kubeconfig_write" //nolint:gosec // operation name, not a credential
	OpRemove        = "kubeconfig_remove"
	OpCurrentServer = "current_server"
	OpDetectAuth    = "detect_auth_type"
	OpReachable     = "reachable"
	OpWhoami        = "whoami"
)

// Auth types returned by detect_auth_type.
const (
	AuthTypeAuto  = "auto"
	AuthTypeOAuth = "oauth"
	AuthTypeOIDC  = "oidc"
)

// writeInput holds the parameters for the kubeconfig_write operation.
type writeInput struct {
	Server            string   `json:"server"`
	Audience          string   `json:"audience"`
	ClusterName       string   `json:"cluster_name"`
	ContextName       string   `json:"context_name"`
	UserName          string   `json:"user_name"`
	KubeconfigPath    string   `json:"kubeconfig_path"`
	ExecCommand       string   `json:"exec_command"`
	ExecArgs          []string `json:"exec_args"`
	InsecureSkipTLS   bool     `json:"insecure_skip_tls"`
	SetCurrentContext bool     `json:"set_current_context"`
}

// removeInput holds the parameters for the kubeconfig_remove operation.
type removeInput struct {
	ClusterName    string `json:"cluster_name"`
	ContextName    string `json:"context_name"`
	UserName       string `json:"user_name"`
	KubeconfigPath string `json:"kubeconfig_path"`
}

// currentServerInput holds the parameters for the current_server operation.
type currentServerInput struct {
	KubeconfigPath string `json:"kubeconfig_path"`
	ContextName    string `json:"context_name"`
}

// detectInput holds the parameters for the detect_auth_type operation.
type detectInput struct {
	Server          string `json:"server"`
	InsecureSkipTLS bool   `json:"insecure_skip_tls"`
}

// reachableInput holds the parameters for the reachable operation.
type reachableInput struct {
	Server          string `json:"server"`
	InsecureSkipTLS bool   `json:"insecure_skip_tls"`
}

// whoamiInput holds the parameters for the whoami operation.
type whoamiInput struct {
	Server          string `json:"server"`
	Token           string `json:"token"`
	Audience        string `json:"audience"`
	InsecureSkipTLS bool   `json:"insecure_skip_tls"`
}

// decodeInput maps the flat wire map onto a typed input struct.
func parseWriteInput(in map[string]any) writeInput {
	return writeInput{
		Server:            stringField(in, "server"),
		Audience:          stringField(in, "audience"),
		ClusterName:       stringField(in, "cluster_name"),
		ContextName:       stringField(in, "context_name"),
		UserName:          stringField(in, "user_name"),
		KubeconfigPath:    stringField(in, "kubeconfig_path"),
		ExecCommand:       stringField(in, "exec_command"),
		ExecArgs:          stringSliceField(in, "exec_args"),
		InsecureSkipTLS:   boolField(in, "insecure_skip_tls"),
		SetCurrentContext: boolField(in, "set_current_context"),
	}
}

func parseRemoveInput(in map[string]any) removeInput {
	return removeInput{
		ClusterName:    stringField(in, "cluster_name"),
		ContextName:    stringField(in, "context_name"),
		UserName:       stringField(in, "user_name"),
		KubeconfigPath: stringField(in, "kubeconfig_path"),
	}
}

func parseCurrentServerInput(in map[string]any) currentServerInput {
	return currentServerInput{
		KubeconfigPath: stringField(in, "kubeconfig_path"),
		ContextName:    stringField(in, "context_name"),
	}
}

func parseDetectInput(in map[string]any) detectInput {
	return detectInput{
		Server:          stringField(in, "server"),
		InsecureSkipTLS: boolField(in, "insecure_skip_tls"),
	}
}

func parseReachableInput(in map[string]any) reachableInput {
	return reachableInput{
		Server:          stringField(in, "server"),
		InsecureSkipTLS: boolField(in, "insecure_skip_tls"),
	}
}

func parseWhoamiInput(in map[string]any) whoamiInput {
	return whoamiInput{
		Server:          stringField(in, "server"),
		Token:           stringField(in, "token"),
		Audience:        stringField(in, "audience"),
		InsecureSkipTLS: boolField(in, "insecure_skip_tls"),
	}
}

// stringField extracts a string value from the wire map, tolerating absence.
func stringField(in map[string]any, key string) string {
	v, _ := in[key].(string)
	return v
}

// boolField extracts a bool value from the wire map, tolerating absence and the
// string encodings "true"/"false" that some transports produce.
func boolField(in map[string]any, key string) bool {
	switch v := in[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

// stringSliceField extracts a []string from the wire map, tolerating []any.
func stringSliceField(in map[string]any, key string) []string {
	switch v := in[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

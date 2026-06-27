package kubeconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// execAPIVersion is the client-go exec credential plugin API version baked into
// kubeconfig user entries written by this provider.
const execAPIVersion = "client.authentication.k8s.io/v1"

// resolveKubeconfigPath returns an explicit kubeconfig path, falling back to the
// host's default resolution (KUBECONFIG env, then ~/.kube/config).
func resolveKubeconfigPath(path string) string {
	if path != "" {
		return path
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if p := rules.GetDefaultFilename(); p != "" {
		return p
	}
	return clientcmd.RecommendedHomeFile
}

// loadOrNewConfig loads a single kubeconfig file, returning a fresh empty config
// when the file does not yet exist.
func loadOrNewConfig(path string) (*clientcmdapi.Config, error) {
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return clientcmdapi.NewConfig(), nil
		}
		return nil, fmt.Errorf("load kubeconfig %q: %w", path, err)
	}
	return cfg, nil
}

// opWrite merges an exec-credential cluster/user/context into a kubeconfig file.
func opWrite(in writeInput) (map[string]any, error) {
	if in.Server == "" {
		return failure("server is required"), nil
	}
	if in.ExecCommand == "" {
		return failure("exec_command is required"), nil
	}

	clusterName := sanitizeName(in.ClusterName, "cluster")
	userName := sanitizeName(in.UserName, clusterName)
	contextName := sanitizeName(in.ContextName, clusterName)

	path := resolveKubeconfigPath(in.KubeconfigPath)
	cfg, err := loadOrNewConfig(path)
	if err != nil {
		return nil, err
	}

	cfg.Clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                in.Server,
		InsecureSkipTLSVerify: in.InsecureSkipTLS,
	}

	cfg.AuthInfos[userName] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			APIVersion:      execAPIVersion,
			Command:         in.ExecCommand,
			Args:            in.ExecArgs,
			InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode,
		},
	}

	cfg.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:  clusterName,
		AuthInfo: userName,
	}

	if in.SetCurrentContext || cfg.CurrentContext == "" {
		cfg.CurrentContext = contextName
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create kubeconfig dir: %w", err)
	}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		return nil, fmt.Errorf("write kubeconfig %q: %w", path, err)
	}

	return map[string]any{
		"success":         true,
		"context_name":    contextName,
		"kubeconfig_path": path,
	}, nil
}

// opRemove deletes a cluster/user/context entry from a kubeconfig file.
func opRemove(in removeInput) (map[string]any, error) {
	clusterName := sanitizeName(in.ClusterName, "cluster")
	userName := sanitizeName(in.UserName, clusterName)
	contextName := sanitizeName(in.ContextName, clusterName)

	path := resolveKubeconfigPath(in.KubeconfigPath)
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"success": true, "removed": false}, nil
		}
		return nil, fmt.Errorf("load kubeconfig %q: %w", path, err)
	}

	removed := false
	if _, ok := cfg.Clusters[clusterName]; ok {
		delete(cfg.Clusters, clusterName)
		removed = true
	}
	if _, ok := cfg.AuthInfos[userName]; ok {
		delete(cfg.AuthInfos, userName)
		removed = true
	}
	if _, ok := cfg.Contexts[contextName]; ok {
		delete(cfg.Contexts, contextName)
		removed = true
	}
	if cfg.CurrentContext == contextName {
		cfg.CurrentContext = ""
	}

	if removed {
		if err := clientcmd.WriteToFile(*cfg, path); err != nil {
			return nil, fmt.Errorf("write kubeconfig %q: %w", path, err)
		}
	}

	return map[string]any{
		"success": true,
		"removed": removed,
	}, nil
}

// opCurrentServer returns the API server URL for the selected (or current)
// context in a kubeconfig.
func opCurrentServer(in currentServerInput) (map[string]any, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if in.KubeconfigPath != "" {
		rules.ExplicitPath = in.KubeconfigPath
	}
	cfg, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	contextName := in.ContextName
	if contextName == "" {
		contextName = cfg.CurrentContext
	}
	if contextName == "" {
		return failure("no context selected and no current-context set"), nil
	}

	kubeContext, ok := cfg.Contexts[contextName]
	if !ok {
		return failure(fmt.Sprintf("context %q not found", contextName)), nil
	}
	cluster, ok := cfg.Clusters[kubeContext.Cluster]
	if !ok {
		return failure(fmt.Sprintf("cluster %q not found", kubeContext.Cluster)), nil
	}

	return map[string]any{
		"success": true,
		"server":  cluster.Server,
	}, nil
}

// failure builds a standard unsuccessful operation output with an error message.
func failure(msg string) map[string]any {
	return map[string]any{
		"success": false,
		"error":   msg,
	}
}

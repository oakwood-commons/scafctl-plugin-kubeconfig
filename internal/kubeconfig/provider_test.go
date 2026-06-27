package kubeconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	sdkplugin "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
	sdkprovider "github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func TestGetProviders(t *testing.T) {
	p := &Plugin{}
	providers, err := p.GetProviders(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{ProviderName}, providers)
}

func TestGetProviderDescriptor(t *testing.T) {
	p := &Plugin{}

	t.Run("known provider", func(t *testing.T) {
		desc, err := p.GetProviderDescriptor(context.Background(), ProviderName)
		require.NoError(t, err)
		assert.Equal(t, ProviderName, desc.Name)
		assert.NotEmpty(t, desc.Description)
		assert.NotNil(t, desc.Schema)
		assert.NotEmpty(t, desc.Capabilities)
		assert.NotNil(t, desc.OutputSchemas, "OutputSchemas must be present")
		assert.Equal(t, []string{OpWrite, OpRemove}, desc.WriteOperations)
		for _, cap := range desc.Capabilities {
			assert.Contains(t, desc.OutputSchemas, cap, "OutputSchemas must include capability %s", cap)
		}
		require.NoError(t, sdkprovider.ValidateDescriptor(desc), "descriptor must pass SDK validation")
	})

	t.Run("unknown provider", func(t *testing.T) {
		_, err := p.GetProviderDescriptor(context.Background(), "unknown")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown provider")
	})
}

func TestExecuteProvider_UnknownProvider(t *testing.T) {
	p := &Plugin{}
	_, err := p.ExecuteProvider(context.Background(), "unknown", nil)
	assert.Error(t, err)
}

func TestExecuteProvider_OperationErrors(t *testing.T) {
	p := &Plugin{}

	t.Run("missing operation", func(t *testing.T) {
		_, err := p.ExecuteProvider(context.Background(), ProviderName, map[string]any{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "operation")
	})

	t.Run("unknown operation", func(t *testing.T) {
		_, err := p.ExecuteProvider(context.Background(), ProviderName, map[string]any{"operation": "bogus"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation")
	})
}

// executeData runs an operation and returns its flat output map.
func executeData(t *testing.T, input map[string]any) map[string]any {
	t.Helper()
	p := &Plugin{}
	out, err := p.ExecuteProvider(context.Background(), ProviderName, input)
	require.NoError(t, err)
	data, ok := out.Data.(map[string]any)
	require.True(t, ok, "expected Data to be map[string]any")
	return data
}

func TestWriteRemoveCurrentServer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	// Write a context.
	data := executeData(t, map[string]any{
		"operation":           OpWrite,
		"server":              "https://api.example.com:6443",
		"cluster_name":        "My Cluster",
		"exec_command":        "mycli",
		"exec_args":           []any{"auth", "token", "openshift"},
		"kubeconfig_path":     path,
		"set_current_context": true,
	})
	assert.Equal(t, true, data["success"])
	assert.Equal(t, "my-cluster", data["context_name"])
	assert.Equal(t, path, data["kubeconfig_path"])

	// File should be loadable and contain the exec block with the host binary.
	cfg, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	require.Contains(t, cfg.AuthInfos, "my-cluster")
	exec := cfg.AuthInfos["my-cluster"].Exec
	require.NotNil(t, exec)
	assert.Equal(t, "mycli", exec.Command)
	assert.Equal(t, []string{"auth", "token", "openshift"}, exec.Args)
	assert.Equal(t, "my-cluster", cfg.CurrentContext)

	// current_server should read it back.
	data = executeData(t, map[string]any{
		"operation":       OpCurrentServer,
		"kubeconfig_path": path,
	})
	assert.Equal(t, true, data["success"])
	assert.Equal(t, "https://api.example.com:6443", data["server"])

	// Remove the context.
	data = executeData(t, map[string]any{
		"operation":       OpRemove,
		"cluster_name":    "My Cluster",
		"kubeconfig_path": path,
	})
	assert.Equal(t, true, data["success"])
	assert.Equal(t, true, data["removed"])

	cfg, err = clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.NotContains(t, cfg.Clusters, "my-cluster")
	assert.NotContains(t, cfg.Contexts, "my-cluster")
	assert.Empty(t, cfg.CurrentContext)
}

func TestWrite_Validation(t *testing.T) {
	t.Run("missing server", func(t *testing.T) {
		data := executeData(t, map[string]any{
			"operation":    OpWrite,
			"exec_command": "mycli",
		})
		assert.Equal(t, false, data["success"])
	})

	t.Run("missing exec_command", func(t *testing.T) {
		data := executeData(t, map[string]any{
			"operation": OpWrite,
			"server":    "https://api.example.com:6443",
		})
		assert.Equal(t, false, data["success"])
	})
}

func TestRemove_MissingFile(t *testing.T) {
	data := executeData(t, map[string]any{
		"operation":       OpRemove,
		"cluster_name":    "anything",
		"kubeconfig_path": filepath.Join(t.TempDir(), "does-not-exist"),
	})
	assert.Equal(t, true, data["success"])
	assert.Equal(t, false, data["removed"])
}

func TestCurrentServer_NoContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600))

	data := executeData(t, map[string]any{
		"operation":       OpCurrentServer,
		"kubeconfig_path": path,
	})
	assert.Equal(t, false, data["success"])
}

func TestReachable(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Run("healthy", func(t *testing.T) {
		data := executeData(t, map[string]any{
			"operation":         OpReachable,
			"server":            srv.URL,
			"insecure_skip_tls": true,
		})
		assert.Equal(t, true, data["success"])
		assert.Equal(t, true, data["reachable"])
		assert.Equal(t, http.StatusOK, data["status"])
	})

	t.Run("unreachable", func(t *testing.T) {
		data := executeData(t, map[string]any{
			"operation":         OpReachable,
			"server":            "https://127.0.0.1:1", // nothing listening
			"insecure_skip_tls": true,
		})
		assert.Equal(t, true, data["success"])
		assert.Equal(t, false, data["reachable"])
		assert.Equal(t, 0, data["status"])
	})
}

func TestDetectAuthType(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantType string
		checkKey string
		checkVal string
	}{
		{
			name: "oauth",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/oauth-authorization-server" {
					_ = json.NewEncoder(w).Encode(map[string]string{
						"authorization_endpoint": "https://oauth.example.com/authorize",
					})
					return
				}
				w.WriteHeader(http.StatusNotFound)
			},
			wantType: AuthTypeOAuth,
			checkKey: "oauth_endpoint",
			checkVal: "https://oauth.example.com/authorize",
		},
		{
			name: "oidc",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/openid-configuration" {
					_ = json.NewEncoder(w).Encode(map[string]string{
						"issuer": "https://oidc.example.com",
					})
					return
				}
				w.WriteHeader(http.StatusNotFound)
			},
			wantType: AuthTypeOIDC,
			checkKey: "oidc_issuer",
			checkVal: "https://oidc.example.com",
		},
		{
			name: "auto",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantType: AuthTypeAuto,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(tt.handler)
			defer srv.Close()

			data := executeData(t, map[string]any{
				"operation":         OpDetectAuth,
				"server":            srv.URL,
				"insecure_skip_tls": true,
			})
			assert.Equal(t, true, data["success"])
			assert.Equal(t, tt.wantType, data["auth_type"])
			if tt.checkKey != "" {
				assert.Equal(t, tt.checkVal, data[tt.checkKey])
			}
		})
	}
}

func TestWhoami(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"apiVersion": "authentication.k8s.io/v1",
			"kind":       "SelfSubjectReview",
			"status": map[string]any{
				"userInfo": map[string]any{
					"username": "alice",
					"uid":      "uid-123",
					"groups":   []string{"system:authenticated", "devs"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	data := executeData(t, map[string]any{
		"operation":         OpWhoami,
		"server":            srv.URL,
		"token":             "secret-token",
		"insecure_skip_tls": true,
	})
	assert.Equal(t, true, data["success"])
	assert.Equal(t, "alice", data["username"])
	assert.Equal(t, "uid-123", data["uid"])
	assert.Equal(t, []any{"system:authenticated", "devs"}, data["groups"])
}

func TestWhoami_Validation(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		data := executeData(t, map[string]any{
			"operation": OpWhoami,
			"server":    "https://api.example.com:6443",
		})
		assert.Equal(t, false, data["success"])
	})
}

func TestDescribeWhatIf(t *testing.T) {
	p := &Plugin{}

	cases := []struct {
		name     string
		input    map[string]any
		contains string
	}{
		{"write", map[string]any{"operation": OpWrite, "cluster_name": "prod", "server": "https://x"}, "prod"},
		{"remove", map[string]any{"operation": OpRemove, "cluster_name": "prod"}, "prod"},
		{"current_server", map[string]any{"operation": OpCurrentServer}, "current"},
		{"detect", map[string]any{"operation": OpDetectAuth, "server": "https://x"}, "https://x"},
		{"reachable", map[string]any{"operation": OpReachable, "server": "https://x"}, "https://x"},
		{"whoami", map[string]any{"operation": OpWhoami, "server": "https://x"}, "https://x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc, err := p.DescribeWhatIf(context.Background(), ProviderName, tc.input)
			require.NoError(t, err)
			assert.Contains(t, desc, tc.contains)
		})
	}

	t.Run("unknown provider", func(t *testing.T) {
		_, err := p.DescribeWhatIf(context.Background(), "unknown", nil)
		assert.Error(t, err)
	})

	t.Run("unknown operation", func(t *testing.T) {
		_, err := p.DescribeWhatIf(context.Background(), ProviderName, map[string]any{"operation": "bogus"})
		assert.Error(t, err)
	})
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		raw      string
		fallback string
		want     string
	}{
		{"My Cluster", "fb", "my-cluster"},
		{"api.example.com:6443", "fb", "api.example.com-6443"},
		{"  ", "fb", "fb"},
		{"!!!", "fb", "fb"},
		{"Already-Valid.1", "fb", "already-valid.1"},
		{"--trim--", "fb", "trim"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeName(tt.raw, tt.fallback))
		})
	}
}

func TestPlugin_SupportingMethods(t *testing.T) {
	p := &Plugin{}
	require.NoError(t, p.ConfigureProvider(context.Background(), ProviderName, sdkplugin.ProviderConfig{}))
	require.NoError(t, p.StopProvider(context.Background(), ProviderName))

	deps, err := p.ExtractDependencies(context.Background(), ProviderName, nil)
	require.NoError(t, err)
	assert.Nil(t, deps)

	err = p.ExecuteProviderStream(context.Background(), ProviderName, nil, nil)
	assert.Error(t, err)
}

func BenchmarkSanitizeName(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = sanitizeName("My Cluster Name", "cluster")
	}
}

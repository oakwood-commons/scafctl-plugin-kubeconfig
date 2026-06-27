package kubeconfig

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// probeTimeout bounds every outbound request the provider makes to a cluster.
const probeTimeout = 10 * time.Second

// newHTTPClient builds an HTTP client honoring the insecure-skip-tls option.
func newHTTPClient(insecureSkipTLS bool) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipTLS, //nolint:gosec // opt-in, documented dev-only
			MinVersion:         tls.VersionTLS12,
		},
	}
	return &http.Client{
		Timeout:   probeTimeout,
		Transport: transport,
	}
}

// opReachable checks whether the cluster API server answers its health endpoint.
func opReachable(ctx context.Context, in reachableInput) (map[string]any, error) {
	if in.Server == "" {
		return failure("server is required"), nil
	}

	client := newHTTPClient(in.InsecureSkipTLS)
	url := strings.TrimRight(in.Server, "/") + "/healthz"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build reachability request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		// A transport error means unreachable, not a provider failure.
		return map[string]any{
			"success":   true,
			"reachable": false,
			"status":    0,
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	return map[string]any{
		"success":   true,
		"reachable": resp.StatusCode < http.StatusInternalServerError,
		"status":    resp.StatusCode,
	}, nil
}

// opDetectAuth best-effort probes well-known endpoints to classify the cluster's
// authentication style as oauth (OpenShift), oidc, or auto (undetermined).
func opDetectAuth(ctx context.Context, in detectInput) (map[string]any, error) {
	if in.Server == "" {
		return failure("server is required"), nil
	}

	client := newHTTPClient(in.InsecureSkipTLS)
	base := strings.TrimRight(in.Server, "/")

	// OpenShift exposes an OAuth authorization-server discovery document.
	var oauthDoc struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
	}
	if ok := getJSON(ctx, client, base+"/.well-known/oauth-authorization-server", &oauthDoc); ok && oauthDoc.AuthorizationEndpoint != "" {
		return map[string]any{
			"success":        true,
			"auth_type":      AuthTypeOAuth,
			"oauth_endpoint": oauthDoc.AuthorizationEndpoint,
			"oidc_issuer":    "",
		}, nil
	}

	// Plain OIDC discovery (service-account issuer / external IdP).
	var oidcDoc struct {
		Issuer string `json:"issuer"`
	}
	if ok := getJSON(ctx, client, base+"/.well-known/openid-configuration", &oidcDoc); ok && oidcDoc.Issuer != "" {
		return map[string]any{
			"success":        true,
			"auth_type":      AuthTypeOIDC,
			"oidc_issuer":    oidcDoc.Issuer,
			"oauth_endpoint": "",
		}, nil
	}

	return map[string]any{
		"success":        true,
		"auth_type":      AuthTypeAuto,
		"oidc_issuer":    "",
		"oauth_endpoint": "",
	}, nil
}

// getJSON performs a GET and decodes a 2xx JSON body into out. It returns false
// on any transport, status, or decode error so callers can fall through.
func getJSON(ctx context.Context, client *http.Client, url string, out any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(out) == nil
}

// opWhoami runs a SelfSubjectReview against the cluster using the supplied token
// and reports the authenticated identity. The token is never logged or cached.
func opWhoami(ctx context.Context, in whoamiInput) (map[string]any, error) {
	if in.Server == "" {
		return failure("server is required"), nil
	}
	if in.Token == "" {
		return failure("token is required"), nil
	}

	cfg := &rest.Config{
		Host:        in.Server,
		BearerToken: in.Token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: in.InsecureSkipTLS,
		},
		Timeout: probeTimeout,
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}

	review, err := clientset.AuthenticationV1().SelfSubjectReviews().Create(
		ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{},
	)
	if err != nil {
		return failure(fmt.Sprintf("whoami failed: %v", err)), nil
	}

	user := review.Status.UserInfo
	groups := make([]any, len(user.Groups))
	for i, g := range user.Groups {
		groups[i] = g
	}

	return map[string]any{
		"success":  true,
		"username": user.Username,
		"groups":   groups,
		"uid":      user.UID,
	}, nil
}

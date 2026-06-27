// Package main is the entry point for the scafctl-plugin-kubeconfig plugin.
package main

import (
	"github.com/oakwood-commons/scafctl-plugin-kubeconfig/internal/kubeconfig"

	sdkplugin "github.com/oakwood-commons/scafctl-plugin-sdk/plugin"
)

func main() {
	sdkplugin.Serve(&kubeconfig.Plugin{})
}

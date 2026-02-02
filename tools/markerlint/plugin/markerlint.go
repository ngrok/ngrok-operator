// Package plugin provides a golangci-lint module plugin for the markerlint analyzer.
//
// To use with golangci-lint, add to .golangci.yml:
//
//	linters-settings:
//	  custom:
//	    markerlint:
//	      type: "module"
//	      path: "github.com/ngrok/ngrok-operator/tools/markerlint/plugin"
package plugin

import (
	"github.com/ngrok/ngrok-operator/tools/markerlint"
	"golang.org/x/tools/go/analysis"
)

// AnalyzerPlugin is the golangci-lint plugin interface.
type AnalyzerPlugin struct{}

// GetAnalyzers returns the analyzers provided by this plugin.
func (*AnalyzerPlugin) GetAnalyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{markerlint.Analyzer}
}

// New creates a new instance of the plugin.
func New(_ any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{markerlint.Analyzer}, nil
}

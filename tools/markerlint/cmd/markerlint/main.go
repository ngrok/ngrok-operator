// Command markerlint provides a standalone CLI for the markerlint analyzer.
//
// Usage:
//
//	markerlint ./api/...
//	markerlint ./...
package main

import (
	"github.com/ngrok/ngrok-operator/tools/markerlint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(markerlint.Analyzer)
}

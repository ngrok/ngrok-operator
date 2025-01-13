package managerdriver

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestManagerDriver(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ManagerDriver package Test Suite")
}

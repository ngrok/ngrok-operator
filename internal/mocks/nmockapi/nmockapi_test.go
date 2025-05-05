package nmockapi_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNmockAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "nmockapi Suite")
}

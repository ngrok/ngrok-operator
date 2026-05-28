package testutils

const DefaultControllerName = "ngrok.com/ingress-controller"

// LEGACY-PREFIX-MIGRATION: BEGIN
// LegacyControllerName is referenced by store dual-read tests. Delete this
// constant along with those tests in the release immediately before 1.0.
const LegacyControllerName = "k8s.ngrok.com/ingress-controller"

// LEGACY-PREFIX-MIGRATION: END

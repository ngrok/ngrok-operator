package labels

// prefix is the unified label prefix introduced in the ngrok.com migration.
const prefix = "ngrok.com/"

// LEGACY-PREFIX-MIGRATION: BEGIN
// Deprecated label prefix retained for read-side compatibility.
// Delete this const and every `Legacy*` symbol downstream of it in the
// release immediately before ngrok-operator 1.0.
const legacyPrefix = "k8s.ngrok.com/"

// LEGACY-PREFIX-MIGRATION: END

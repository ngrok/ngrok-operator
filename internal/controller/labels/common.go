package labels

// prefix is the unified label prefix introduced in the ngrok.com migration.
const prefix = "ngrok.com/"

// LEGACY-PREFIX-MIGRATION: BEGIN
// Deprecated label prefix retained for read-side compatibility.
// LEGACY-PREFIX-MIGRATION (read-side cleanup): delete this const and every
// `Legacy*` symbol downstream of it.
const legacyPrefix = "k8s.ngrok.com/"

// LEGACY-PREFIX-MIGRATION: END

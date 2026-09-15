package managerdriver

import (
	"encoding/json"

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
)

// metadataForGeneratedObject converts the operator's internally-computed
// metadata (a flat JSON object string built by ir.MergeMetadata) into the
// object form the CRDs store.
//
// The operator writes the object form only. The legacy JSON-string form stays
// readable on the v1alpha1 CRDs for objects users have not converted yet, but
// nothing here produces it — see docs/developer-guide/passivity-shims.md
// "CRD `spec.metadata` type change".
//
// An empty input yields nil so the field is omitted and the CRD default
// applies, matching what the operator wrote before the object form.
//
// A parse failure is a bug rather than user input, since the caller's string is
// operator-built. Falling back to the CRD default map keeps a value on the
// object instead of silently dropping metadata.
func metadataForGeneratedObject(metadata string) json.RawMessage {
	if metadata == "" {
		return nil
	}
	m, err := commonv1alpha1.MetadataMapFromJSON(metadata)
	if err != nil {
		m = commonv1alpha1.DefaultMetadataMap()
	}
	return commonv1alpha1.MetadataFromMap(m)
}

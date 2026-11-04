/*
MIT License

Copyright (c) 2026 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package ippolicy

import (
	"encoding/json"
	"fmt"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
)

// Rule is the kind-agnostic view of one IP policy rule, decoded from
// IPPolicyResource.GetRulesJSON. Reconcilers work with Rule so they never
// depend on either kind's concrete IPPolicyRule type.
type Rule struct {
	Description string `json:"description,omitempty"`
	// Metadata is kept raw: the legacy kind allows a JSON string or an
	// object here, the canonical kind always encodes an object. Use
	// MetadataAPIString to get what the ngrok API stores.
	Metadata json.RawMessage `json:"metadata,omitempty"`
	CIDR     string          `json:"cidr,omitempty"`
	Action   string          `json:"action,omitempty"`
}

// MetadataAPIString returns the rule metadata encoded as the string the ngrok
// API stores, with the same encoding the operator has always used for it.
func (r Rule) MetadataAPIString() string {
	return common.MetadataAPIString(r.Metadata)
}

// DecodeRules returns the spec rules of any IP-policy kind as []Rule.
func DecodeRules(p IPPolicyResource) ([]Rule, error) {
	raw, err := p.GetRulesJSON()
	if err != nil {
		return nil, fmt.Errorf("encoding rules: %w", err)
	}
	var rules []Rule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("decoding rules: %w", err)
	}
	return rules, nil
}

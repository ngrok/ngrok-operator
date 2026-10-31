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

package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// fullLegacyStatus sets every field of the legacy status, so a field the JSON
// bridge drops shows up as a diff.
func fullLegacyStatus() ingressv1alpha1.DomainStatus {
	now := metav1.NewTime(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC))
	return ingressv1alpha1.DomainStatus{
		ObservedGeneration:       3,
		ID:                       "rd_123",
		Domain:                   "a.example.com",
		ResolvesTo:               []ingressv1alpha1.DomainResolvesToEntry{{Value: "us"}},
		CNAMETarget:              new("abc.ngrok-cname.com"),
		ACMEChallengeCNAMETarget: new("acme.ngrok-cname.com"),
		Certificate:              &ingressv1alpha1.DomainStatusCertificateInfo{ID: "cert_1"},
		CertificateManagementPolicy: &ingressv1alpha1.DomainStatusCertificateManagementPolicy{
			Authority: "letsencrypt", PrivateKeyType: "ecdsa",
		},
		CertificateManagementStatus: &ingressv1alpha1.DomainStatusCertificateManagementStatus{
			RenewsAt: &now,
			ProvisioningJob: &ingressv1alpha1.DomainStatusProvisioningJob{
				ErrorCode: "DNS_ERROR", Message: "waiting", StartedAt: &now, RetriesAt: &now,
			},
		},
		Conditions: []metav1.Condition{{
			Type: ConditionReady, Status: metav1.ConditionTrue, Reason: ReasonDomainActive,
			Message: "ok", ObservedGeneration: 3, LastTransitionTime: now,
		}},
	}
}

func TestFromLegacy_StatusRoundTrip(t *testing.T) {
	legacy := &ingressv1alpha1.Domain{Status: fullLegacyStatus()}
	want := legacy.Status.DeepCopy()

	r, err := FromLegacy(legacy)
	require.NoError(t, err)

	// The bridged status carries every field.
	legacyJSON, err := json.Marshal(want)
	require.NoError(t, err)
	bridgedJSON, err := json.Marshal(r.Status())
	require.NoError(t, err)
	assert.JSONEq(t, string(legacyJSON), string(bridgedJSON))

	// Mutations through Status land on the object only after CommitStatus.
	r.Status().ID = "rd_456"
	assert.Equal(t, "rd_123", legacy.Status.ID)
	require.NoError(t, r.CommitStatus())
	assert.Equal(t, "rd_456", legacy.Status.ID)

	// Compare as JSON: metav1.Time decodes into the Local location.
	want.ID = "rd_456"
	wantJSON, err := json.Marshal(want)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(legacy.Status)
	require.NoError(t, err)
	assert.JSONEq(t, string(wantJSON), string(gotJSON))
}

// jsonFieldPaths returns every JSON field path reachable from t.
func jsonFieldPaths(t reflect.Type, prefix string, out map[string]string) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == "k8s.io/apimachinery/pkg/apis/meta/v1" {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		path := prefix + "." + name
		out[path] = f.Type.Kind().String()
		jsonFieldPaths(f.Type, path, out)
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// The legacy adapter bridges status through JSON and copies spec field by
// field. Both rely on the two kinds having the same shape, so a field added to
// one and not the other must fail here rather than be silently dropped.
func TestDomainKindsHaveTheSameShape(t *testing.T) {
	for _, tc := range []struct {
		name            string
		legacy, v1      reflect.Type
		allowedToDiffer []string
	}{
		{"status", reflect.TypeFor[ingressv1alpha1.DomainStatus](), reflect.TypeFor[ngrokv1.DomainStatus](), nil},
		// spec.metadata is a raw JSON value on the legacy kind and a map on v1.
		{"spec", reflect.TypeFor[ingressv1alpha1.DomainSpec](), reflect.TypeFor[ngrokv1.DomainSpec](), []string{".metadata"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy, v1 := map[string]string{}, map[string]string{}
			jsonFieldPaths(tc.legacy, "", legacy)
			jsonFieldPaths(tc.v1, "", v1)
			for _, p := range tc.allowedToDiffer {
				delete(legacy, p)
				delete(v1, p)
			}
			assert.Equal(t, sortedKeys(legacy), sortedKeys(v1))
			assert.Equal(t, legacy, v1)
		})
	}
}

func TestMetadataAPIString_KindsAgreeOnDefault(t *testing.T) {
	// The CRD defaults of both kinds must produce the same API string, or every
	// adopted reservation would get a metadata rewrite on upgrade.
	legacy := &ingressv1alpha1.Domain{Spec: ingressv1alpha1.DomainSpec{
		Metadata: json.RawMessage(`"{\"owned-by\":\"ngrok-operator\"}"`),
	}}
	v1 := &ngrokv1.Domain{Spec: ngrokv1.DomainSpec{
		Metadata: map[string]string{"owned-by": "ngrok-operator"},
	}}
	lr, err := FromLegacy(legacy)
	require.NoError(t, err)
	assert.Equal(t, lr.MetadataAPIString(), FromV1(v1).MetadataAPIString())
	assert.JSONEq(t, `{"owned-by":"ngrok-operator"}`, FromV1(v1).MetadataAPIString())
}

func TestFromLegacy_PassesMetadataThroughVerbatim(t *testing.T) {
	legacy := &ingressv1alpha1.Domain{Spec: ingressv1alpha1.DomainSpec{
		Metadata: commonv1alpha1.MetadataFromLegacyString("free text"),
	}}
	r, err := FromLegacy(legacy)
	require.NoError(t, err)
	assert.Equal(t, "free text", r.MetadataAPIString())
}

func TestFromLegacy_SpecAccessors(t *testing.T) {
	legacy := &ingressv1alpha1.Domain{Spec: ingressv1alpha1.DomainSpec{
		Domain:        "a.example.com",
		Description:   "mine",
		ResolvesTo:    []ingressv1alpha1.DomainResolvesToEntry{{Value: "us"}},
		ReclaimPolicy: ingressv1alpha1.DomainReclaimPolicyRetain,
	}}
	r, err := FromLegacy(legacy)
	require.NoError(t, err)
	assert.True(t, r.IsLegacy())
	assert.Same(t, legacy, r.Object())
	assert.Equal(t, "a.example.com", r.Hostname())
	assert.Equal(t, "mine", r.Description())
	assert.Equal(t, []ngrokv1.DomainResolvesToEntry{{Value: "us"}}, r.ResolvesTo())
	assert.Equal(t, ngrokv1.DomainReclaimPolicyRetain, r.ReclaimPolicy())
}

// domainKinds returns a fresh Resource of each kind for hostname with status
// id. Condition logic is written once against Resource; running it against
// both kinds proves the legacy adapter doesn't change the outcome.
func domainKinds(t *testing.T, hostname, id string) map[string]Resource {
	t.Helper()
	legacy, err := FromLegacy(&ingressv1alpha1.Domain{ // LEGACY-domain-kind: delete this entry at cleanup.
		ObjectMeta: metav1.ObjectMeta{Name: "d", Generation: 1},
		Spec:       ingressv1alpha1.DomainSpec{Domain: hostname},
		Status:     ingressv1alpha1.DomainStatus{ID: id},
	})
	require.NoError(t, err)
	return map[string]Resource{
		"canonical": FromV1(&ngrokv1.Domain{
			ObjectMeta: metav1.ObjectMeta{Name: "d", Generation: 1},
			Spec:       ngrokv1.DomainSpec{Domain: hostname},
			Status:     ngrokv1.DomainStatus{ID: id},
		}),
		"legacy": legacy,
	}
}

func TestUpdateConditions_BothKinds(t *testing.T) {
	cases := []struct {
		name        string
		hostname    string
		id          string
		ngrokDomain *ngrok.ReservedDomain
		err         error
		wantReady   bool
		wantReason  string
	}{
		{"ngrok managed", "a.ngrok.app", "rd_1", &ngrok.ReservedDomain{ID: "rd_1"}, nil, true, ReasonDomainActive},
		{"custom provisioning", "a.example.com", "rd_1", &ngrok.ReservedDomain{ID: "rd_1", CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{}}, nil, false, ReasonProvisioningError},
		{"no id", "a.example.com", "", nil, nil, false, ReasonDomainInvalid},
		{"create error", "a.example.com", "", nil, errors.New("boom"), false, ReasonDomainCreationFailed},
	}
	for _, tc := range cases {
		for kind, r := range domainKinds(t, tc.hostname, tc.id) {
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				UpdateConditions(r, tc.ngrokDomain, tc.err)
				require.NoError(t, r.CommitStatus())

				cond := meta.FindStatusCondition(r.Status().Conditions, ConditionReady)
				require.NotNil(t, cond)
				assert.Equal(t, tc.wantReason, cond.Reason)
				assert.Equal(t, tc.wantReady, IsReady(r))
			})
		}
	}
}

func TestSetSupersededCondition(t *testing.T) {
	r := domainKinds(t, "a.example.com", "rd_1")["legacy"]
	SetSupersededCondition(r, "ns", "a-example-com")

	cond := meta.FindStatusCondition(r.Status().Conditions, ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, ReasonSupersededByV1, cond.Reason)
	assert.Contains(t, cond.Message, "domains.ngrok.com ns/a-example-com")
	assert.True(t, IsReady(r), "a superseded legacy Domain still reports its shared reservation as usable")
}

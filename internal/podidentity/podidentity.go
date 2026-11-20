// Package podidentity finds the Pod behind a connection's source IP, so a
// forwarder can tell ngrok which Pod made the call.
package podidentity

import (
	"context"
	"fmt"
	"strings"

	parser "github.com/ngrok/ngrok-operator/internal/annotations/parser"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IPIndex is the Pod field index that Lookup reads.
const IPIndex = "status.podIP"

// Identity is the calling Pod as ngrok sees it. The zero value means no
// Pod matched.
type Identity struct {
	UID         string
	Name        string
	Namespace   string
	Annotations map[string]string
}

// RegisterIndex adds the Pod IP index that Lookup needs. The cache must hold
// Pods in every namespace a caller can run in.
func RegisterIndex(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(ctx, &corev1.Pod{}, IPIndex, podIP); err != nil {
		return fmt.Errorf("unable to create pod IP index: %w", err)
	}
	return nil
}

func podIP(obj client.Object) []string {
	pod := obj.(*corev1.Pod)
	if pod.Status.PodIP == "" {
		return nil
	}
	return []string{pod.Status.PodIP}
}

// Lookup returns the identity of the Pod with IP ip and how many Pods have
// that IP. With no match it returns the zero Identity. With more than one it
// returns the first, which is arbitrary.
func Lookup(ctx context.Context, r client.Reader, ip string) (Identity, int, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.MatchingFields{IPIndex: ip}); err != nil {
		return Identity{}, 0, err
	}
	if len(pods.Items) == 0 {
		return Identity{}, 0, nil
	}
	return FromPod(&pods.Items[0]), len(pods.Items), nil
}

// FromPod builds pod's identity. It keeps only ngrok annotations, so other
// annotations on the Pod are never sent to ngrok.
func FromPod(pod *corev1.Pod) Identity {
	anns := make(map[string]string)
	for key, value := range pod.Annotations {
		// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the legacy prefix match
		if strings.HasPrefix(key, parser.CanonicalAnnotationsPrefix+"/") ||
			strings.HasPrefix(key, parser.LegacyAnnotationsPrefix+"/") {
			anns[key] = value
		}
	}
	return Identity{
		UID:         string(pod.UID),
		Name:        pod.Name,
		Namespace:   pod.Namespace,
		Annotations: anns,
	}
}

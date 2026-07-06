package labels

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestControllerLabels(t *testing.T) {
	tests := []struct {
		name                string
		controllerNamespace string
		controllerName      string
		want                map[string]string
	}{
		{
			name:                "returns labels with namespace and name (dual-write)",
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name:                "handles empty values (dual-write)",
			controllerNamespace: "",
			controllerName:      "",
			want: map[string]string{
				ControllerNamespace:       "",
				ControllerName:            "",
				LegacyControllerNamespace: "",
				LegacyControllerName:      "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ControllerLabels(tt.controllerNamespace, tt.controllerName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasControllerLabels(t *testing.T) {
	tests := []struct {
		name                string
		obj                 client.Object
		controllerNamespace string
		controllerName      string
		want                bool
	}{
		{
			name: "object has matching labels",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace: "ngrok-operator",
						ControllerName:      "my-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want:                true,
		},
		{
			name: "object has nil labels",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: nil,
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want:                false,
		},
		{
			name: "object has wrong namespace label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace: "other-namespace",
						ControllerName:      "my-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want:                false,
		},
		{
			name: "object has wrong name label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace: "ngrok-operator",
						ControllerName:      "other-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want:                false,
		},
		{
			name: "object missing namespace label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerName: "my-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want:                false,
		},
		{
			name: "object missing name label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace: "ngrok-operator",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want:                false,
		},
		{
			name: "object has matching labels with additional labels",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace: "ngrok-operator",
						ControllerName:      "my-controller",
						"app":               "my-app",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasControllerLabels(tt.obj, tt.controllerNamespace, tt.controllerName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnsureControllerLabels(t *testing.T) {
	tests := []struct {
		name                string
		obj                 client.Object
		controllerNamespace string
		controllerName      string
		wantModified        bool
		wantLabels          map[string]string
	}{
		{
			name: "adds labels to object with nil labels (dual-write)",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: nil,
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "adds labels to object with empty labels (dual-write)",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "preserves existing labels and adds controller labels (dual-write)",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "my-app",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				"app":                     "my-app",
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "returns false when both new and legacy labels already match",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace:       "ngrok-operator",
						ControllerName:            "my-controller",
						LegacyControllerNamespace: "ngrok-operator",
						LegacyControllerName:      "my-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        false,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "ensure-sets legacy pair when only new pair is present",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace: "ngrok-operator",
						ControllerName:      "my-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "ensure-sets new pair when only legacy pair is present (R1 keeps legacy)",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LegacyControllerNamespace: "ngrok-operator",
						LegacyControllerName:      "my-controller",
						"app":                     "my-app",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				"app":                     "my-app",
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "updates namespace label when different",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace:       "old-namespace",
						ControllerName:            "my-controller",
						LegacyControllerNamespace: "old-namespace",
						LegacyControllerName:      "my-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "updates name label when different",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace:       "ngrok-operator",
						ControllerName:            "old-controller",
						LegacyControllerNamespace: "ngrok-operator",
						LegacyControllerName:      "old-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "adds missing namespace label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerName:       "my-controller",
						LegacyControllerName: "my-controller",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
		{
			name: "adds missing name label",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						ControllerNamespace:       "ngrok-operator",
						LegacyControllerNamespace: "ngrok-operator",
					},
				},
			},
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			wantModified:        true,
			wantLabels: map[string]string{
				ControllerNamespace:       "ngrok-operator",
				ControllerName:            "my-controller",
				LegacyControllerNamespace: "ngrok-operator",
				LegacyControllerName:      "my-controller",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureControllerLabels(tt.obj, tt.controllerNamespace, tt.controllerName)
			assert.Equal(t, tt.wantModified, got)
			assert.Equal(t, tt.wantLabels, tt.obj.GetLabels())
		})
	}
}

func TestControllerLabelSelector(t *testing.T) {
	tests := []struct {
		name                string
		controllerNamespace string
		controllerName      string
		want                client.MatchingLabels
	}{
		{
			name:                "returns matching labels selector",
			controllerNamespace: "ngrok-operator",
			controllerName:      "my-controller",
			want: client.MatchingLabels{
				ControllerNamespace: "ngrok-operator",
				ControllerName:      "my-controller",
			},
		},
		{
			name:                "handles empty values",
			controllerNamespace: "",
			controllerName:      "",
			want: client.MatchingLabels{
				ControllerNamespace: "",
				ControllerName:      "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ControllerLabelSelector(tt.controllerNamespace, tt.controllerName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestControllerLabelValues(t *testing.T) {
	t.Run("NewControllerLabelValues", func(t *testing.T) {
		clv := NewControllerLabelValues("ngrok-operator", "my-controller")
		assert.Equal(t, "ngrok-operator", clv.Namespace)
		assert.Equal(t, "my-controller", clv.Name)
	})

	t.Run("Labels", func(t *testing.T) {
		clv := ControllerLabelValues{Namespace: "ngrok-operator", Name: "my-controller"}
		got := clv.Labels()
		want := map[string]string{
			ControllerNamespace:       "ngrok-operator",
			ControllerName:            "my-controller",
			LegacyControllerNamespace: "ngrok-operator",
			LegacyControllerName:      "my-controller",
		}
		assert.Equal(t, want, got)
	})

	t.Run("HasLabels", func(t *testing.T) {
		clv := ControllerLabelValues{Namespace: "ngrok-operator", Name: "my-controller"}

		objWithLabels := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					ControllerNamespace: "ngrok-operator",
					ControllerName:      "my-controller",
				},
			},
		}
		assert.True(t, clv.HasLabels(objWithLabels))

		objWithoutLabels := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Labels: nil,
			},
		}
		assert.False(t, clv.HasLabels(objWithoutLabels))
	})

	t.Run("EnsureLabels", func(t *testing.T) {
		clv := ControllerLabelValues{Namespace: "ngrok-operator", Name: "my-controller"}

		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Labels: nil,
			},
		}
		modified := clv.EnsureLabels(obj)
		assert.True(t, modified)
		assert.Equal(t, map[string]string{
			ControllerNamespace:       "ngrok-operator",
			ControllerName:            "my-controller",
			LegacyControllerNamespace: "ngrok-operator",
			LegacyControllerName:      "my-controller",
		}, obj.GetLabels())

		modified = clv.EnsureLabels(obj)
		assert.False(t, modified)
	})

	t.Run("Selector", func(t *testing.T) {
		clv := ControllerLabelValues{Namespace: "ngrok-operator", Name: "my-controller"}
		got := clv.Selector()
		want := client.MatchingLabels{
			ControllerNamespace: "ngrok-operator",
			ControllerName:      "my-controller",
		}
		assert.Equal(t, want, got)
	})
}

func TestHasControllerLabels_DualPrefix(t *testing.T) {
	t.Run("matches legacy prefix", func(t *testing.T) {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			LegacyControllerNamespace: "ngrok-operator",
			LegacyControllerName:      "my-controller",
		}}}
		assert.True(t, HasControllerLabels(obj, "ngrok-operator", "my-controller"))
	})

	t.Run("matches new prefix", func(t *testing.T) {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			ControllerNamespace: "ngrok-operator",
			ControllerName:      "my-controller",
		}}}
		assert.True(t, HasControllerLabels(obj, "ngrok-operator", "my-controller"))
	})

	t.Run("no match when both partial", func(t *testing.T) {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			ControllerNamespace:  "ngrok-operator",
			LegacyControllerName: "my-controller",
		}}}
		assert.False(t, HasControllerLabels(obj, "ngrok-operator", "my-controller"),
			"requires a full pair on one prefix; partials must not cross-match")
	})
}

func TestEnsureControllerLabels_R1KeepsLegacy(t *testing.T) {
	// R1 dual-writes: an object that arrives with only the legacy pair must
	// be promoted to having both pairs, not have the legacy pair removed.
	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		LegacyControllerNamespace: "ngrok-operator",
		LegacyControllerName:      "my-controller",
		"app":                     "my-app",
	}}}

	modified := EnsureControllerLabels(obj, "ngrok-operator", "my-controller")
	assert.True(t, modified)

	got := obj.GetLabels()
	assert.Equal(t, "ngrok-operator", got[ControllerNamespace])
	assert.Equal(t, "my-controller", got[ControllerName])
	assert.Equal(t, "ngrok-operator", got[LegacyControllerNamespace],
		"R1 must preserve the legacy pair (in R2 this becomes a delete)")
	assert.Equal(t, "my-controller", got[LegacyControllerName],
		"R1 must preserve the legacy pair (in R2 this becomes a delete)")
	assert.Equal(t, "my-app", got["app"], "unrelated labels are preserved")
}

func TestControllerLabelSelectors_ReturnsBoth(t *testing.T) {
	got := ControllerLabelSelectors("ngrok-operator", "my-controller")
	assert.Len(t, got, 2)
	assert.Equal(t, client.MatchingLabels{
		ControllerNamespace: "ngrok-operator",
		ControllerName:      "my-controller",
	}, got[0])
	assert.Equal(t, client.MatchingLabels{
		LegacyControllerNamespace: "ngrok-operator",
		LegacyControllerName:      "my-controller",
	}, got[1])
}

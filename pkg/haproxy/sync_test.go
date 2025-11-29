package haproxy

import (
	"testing"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildBackendsFromEndpointSlices(t *testing.T) {
	ready := boolPtr(true)
	notReady := boolPtr(false)

	testCases := []struct {
		name     string
		slices   []*discoveryv1.EndpointSlice
		expected int
	}{
		{
			name: "single endpoint",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "slice1"},
					Endpoints: []discoveryv1.Endpoint{
						{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: ready}},
					},
					Ports: []discoveryv1.EndpointPort{{Port: int32Ptr(8080)}},
				},
			},
			expected: 1,
		},
		{
			name: "skips not ready",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "slice2"},
					Endpoints: []discoveryv1.Endpoint{
						{Addresses: []string{"10.0.0.2"}, Conditions: discoveryv1.EndpointConditions{Ready: notReady}},
					},
					Ports: []discoveryv1.EndpointPort{{Port: int32Ptr(80)}},
				},
			},
			expected: 0,
		},
		{
			name: "multiple ports and addresses",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "slice3"},
					Endpoints: []discoveryv1.Endpoint{
						{Addresses: []string{"10.0.0.3", "10.0.0.4"}, Conditions: discoveryv1.EndpointConditions{Ready: ready}},
					},
					Ports: []discoveryv1.EndpointPort{{Port: int32Ptr(8080)}, {Port: int32Ptr(8443)}},
				},
			},
			expected: 4,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backends := BuildBackendsFromEndpointSlices(tc.slices)
			if len(backends) != tc.expected {
				t.Fatalf("expected %d backends, got %d", tc.expected, len(backends))
			}
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func int32Ptr(v int32) *int32 {
	return &v
}

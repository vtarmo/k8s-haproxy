package haproxy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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

			backends := BuildBackendsFromEndpointSlices(tc.slices, map[string]string{}, 0)
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

func TestBuildBackendsFromEndpoints(t *testing.T) {
	testCases := []struct {
		name     string
		eps      []*corev1.Endpoints
		expected int
	}{
		{
			name: "single endpoint",
			eps: []*corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc"},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
							Ports:     []corev1.EndpointPort{{Port: 80}},
						},
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple subsets",
			eps: []*corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc"},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}},
							Ports:     []corev1.EndpointPort{{Port: 8080}},
						},
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.0.0.3"}},
							Ports:     []corev1.EndpointPort{{Port: 8443}},
						},
					},
				},
			},
			expected: 2,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backends := BuildBackendsFromEndpoints(tc.eps, map[string]string{}, 0)
			if len(backends) != tc.expected {
				t.Fatalf("expected %d backends, got %d", tc.expected, len(backends))
			}
		})
	}
}

func TestResolveAddressPrefersNodeIP(t *testing.T) {
	nodeName := "node1"
	addr := resolveAddress("10.0.0.10", &nodeName, map[string]string{"node1": "192.168.0.5"})
	if addr != "192.168.0.5" {
		t.Fatalf("expected node IP override, got %s", addr)
	}
}

func TestSelectPortOverride(t *testing.T) {
	p := int32(8080)
	if got := selectPort(&p, 30443); got != 30443 {
		t.Fatalf("expected override 30443, got %d", got)
	}
	if got := selectPort(&p, 0); got != 8080 {
		t.Fatalf("expected found 8080, got %d", got)
	}
	if got := selectPort(nil, 0); got != 0 {
		t.Fatalf("expected 0 when both nil and no override, got %d", got)
	}
}

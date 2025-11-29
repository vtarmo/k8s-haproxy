package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"example.com/haproxy-k8s-sync/internal/k8s"
)

func TestProcessNextWorkItemInvokesSyncer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	client := fake.NewSimpleClientset()
	informers := k8s.NewInformers(client, "ingress-nginx", "ingress-nginx", 0)
	syncer := &stubSyncer{}
	c := NewController(informers, syncer, 1)

	informers.Start(ctx)
	if ok := informers.WaitForSync(ctx); !ok {
		t.Fatalf("failed to sync caches")
	}

	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "slice",
			Namespace: "ingress-nginx",
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
		},
		Ports: []discoveryv1.EndpointPort{{Port: int32Ptr(80)}},
	}

	if err := informers.EndpointSliceInformer.GetStore().Add(slice); err != nil {
		t.Fatalf("failed adding slice to store: %v", err)
	}

	c.enqueue(nil)
	if ok := c.processNextWorkItem(ctx); !ok {
		t.Fatalf("work item was not processed")
	}

	if syncer.calls == 0 {
		t.Fatalf("expected syncer to be called")
	}
}

type stubSyncer struct {
	calls int
}

func (s *stubSyncer) Sync(_ context.Context, slices []*discoveryv1.EndpointSlice, _ []*corev1.Endpoints) error {
	if len(slices) == 0 {
		return nil
	}
	s.calls++
	return nil
}

func int32Ptr(v int32) *int32 {
	return &v
}

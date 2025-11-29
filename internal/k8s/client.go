package k8s

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	coreinformers "k8s.io/client-go/informers/core/v1"
	discoveryinformers "k8s.io/client-go/informers/discovery/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	discoverylisters "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// BuildConfig builds a Kubernetes rest.Config using in-cluster config by default and falling back to an optional kubeconfig path.
func BuildConfig(_ context.Context, kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	return rest.InClusterConfig()
}

// Informers bundles the shared informers and listers used by the controller.
type Informers struct {
	EndpointsInformer       cache.SharedIndexInformer
	EndpointSliceInformer   cache.SharedIndexInformer
	EndpointsLister         corelisters.EndpointsLister
	EndpointSliceLister     discoverylisters.EndpointSliceLister
	endpointsHasSynced      cache.InformerSynced
	endpointSlicesHasSynced cache.InformerSynced
}

// NewInformers sets up filtered informers for Endpoints and EndpointSlices scoped to the given namespace and service.
func NewInformers(client kubernetes.Interface, namespace, serviceName string, resync time.Duration) *Informers {
	endpointsInformer := coreinformers.NewFilteredEndpointsInformer(
		client,
		namespace,
		resync,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", serviceName).String()
		},
	)

	endpointSliceInformer := discoveryinformers.NewFilteredEndpointSliceInformer(
		client,
		namespace,
		resync,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(options *metav1.ListOptions) {
			options.LabelSelector = fmt.Sprintf("kubernetes.io/service-name=%s", serviceName)
		},
	)

	return &Informers{
		EndpointsInformer:       endpointsInformer,
		EndpointSliceInformer:   endpointSliceInformer,
		EndpointsLister:         corelisters.NewEndpointsLister(endpointsInformer.GetIndexer()),
		EndpointSliceLister:     discoverylisters.NewEndpointSliceLister(endpointSliceInformer.GetIndexer()),
		endpointsHasSynced:      endpointsInformer.HasSynced,
		endpointSlicesHasSynced: endpointSliceInformer.HasSynced,
	}
}

// Start begins informer event processing.
func (i *Informers) Start(ctx context.Context) {
	go i.EndpointsInformer.Run(ctx.Done())
	go i.EndpointSliceInformer.Run(ctx.Done())
}

// WaitForSync blocks until caches have been synced or context is cancelled.
func (i *Informers) WaitForSync(ctx context.Context) bool {
	return cache.WaitForCacheSync(ctx.Done(), i.endpointsHasSynced, i.endpointSlicesHasSynced)
}

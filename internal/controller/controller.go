package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"example.com/haproxy-k8s-sync/internal/k8s"
)

const queueKey = "ingress-backends"

// BackendSyncer reconciles Kubernetes endpoints to HAProxy backends.
type BackendSyncer interface {
	Sync(ctx context.Context, slices []*discoveryv1.EndpointSlice, endpoints []*corev1.Endpoints) error
}

// Controller watches Endpoints and EndpointSlices and syncs HAProxy backends.
type Controller struct {
	queue             workqueue.RateLimitingInterface
	informers         *k8s.Informers
	syncer            BackendSyncer
	workerCount       int
	syncRetryInterval time.Duration
}

// NewController wires informers to the backend syncer and returns a ready controller instance.
func NewController(informers *k8s.Informers, syncer BackendSyncer, workerCount int) *Controller {
	c := &Controller{
		queue:             workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		informers:         informers,
		syncer:            syncer,
		workerCount:       workerCount,
		syncRetryInterval: time.Second,
	}

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue,
		UpdateFunc: func(_, _ interface{}) { c.enqueue(nil) },
		DeleteFunc: func(_ interface{}) { c.enqueue(nil) },
	}

	informers.EndpointsInformer.AddEventHandler(handler)
	informers.EndpointSliceInformer.AddEventHandler(handler)

	return c
}

// Run starts workers and blocks until context cancellation.
func (c *Controller) Run(ctx context.Context) error {
	defer c.queue.ShutDown()

	c.informers.Start(ctx)
	if ok := c.informers.WaitForSync(ctx); !ok {
		return errors.New("failed to sync informer caches")
	}

	// Ensure at least one reconcile happens even if no events arrive immediately.
	c.enqueue(nil)

	for i := 0; i < c.workerCount; i++ {
		go c.runWorker(ctx)
	}

	<-ctx.Done()
	return nil
}

func (c *Controller) enqueue(_ interface{}) {
	c.queue.Add(queueKey)
}

func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	err := c.sync(ctx)
	if err != nil {
		log.Printf("sync failed: %v", err)
		c.queue.AddRateLimited(queueKey)
		return true
	}

	c.queue.Forget(item)
	return true
}

func (c *Controller) sync(ctx context.Context) error {
	slices, err := c.informers.EndpointSliceLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("listing endpoint slices: %w", err)
	}

	var slicePtrs []*discoveryv1.EndpointSlice
	for i := range slices {
		slicePtrs = append(slicePtrs, slices[i])
	}

	endpoints, err := c.informers.EndpointsLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("listing endpoints: %w", err)
	}

	log.Printf("reconciling backends: %d endpoint slices, %d endpoints", len(slicePtrs), len(endpoints))

	if err := c.syncer.Sync(ctx, slicePtrs, endpoints); err != nil {
		return fmt.Errorf("syncing haproxy backends: %w", err)
	}

	return nil
}

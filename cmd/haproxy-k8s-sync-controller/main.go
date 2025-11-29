package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"

	"example.com/haproxy-k8s-sync/internal/config"
	"example.com/haproxy-k8s-sync/internal/controller"
	"example.com/haproxy-k8s-sync/internal/k8s"
	"example.com/haproxy-k8s-sync/pkg/haproxy"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go startHealthServer(ctx)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	restCfg, err := k8s.BuildConfig(ctx, cfg.KubeconfigPath)
	if err != nil {
		log.Fatalf("failed to build kube config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		log.Fatalf("failed to create kubernetes client: %v", err)
	}

	informers := k8s.NewInformers(clientset, cfg.IngressNamespace, cfg.IngressServiceName, cfg.ResyncPeriod)
	haproxyClient := haproxy.NewDataPlaneClient(cfg.HAProxyBaseURL, cfg.HAProxyUsername, cfg.HAProxyPassword, cfg.HAProxyToken)
	syncer := haproxy.NewSyncer(haproxyClient)
	ctrl := controller.NewController(informers, syncer, cfg.WorkerCount)

	log.Printf("starting controller for %s/%s", cfg.IngressNamespace, cfg.IngressServiceName)
	if err := ctrl.Run(ctx); err != nil {
		log.Fatalf("controller stopped with error: %v", err)
	}

	log.Printf("controller exited gracefully at %s", time.Now().Format(time.RFC3339))
}

func startHealthServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("health server error: %v", err)
	}
}

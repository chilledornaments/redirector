package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	kyaml "sigs.k8s.io/yaml"
	"sync"
	"time"
)

var (
	generateOutputPath       string
	generateServiceName      string
	generateIngressName      string
	generateNamespace        string
	generateIngressClassName string
)

func parseArgs() {
	generateFS := flag.NewFlagSet("generate", flag.ExitOnError)
	p := generateFS.String("out", "./redirector-ingress.yml", "where to write Ingress manifest")
	n := generateFS.String("namespace", "redirector", "Kubernetes namespace where redirector is deployed")
	s := generateFS.String("service-name", "redirector", "Kubernetes service name to send traffic to")
	i := generateFS.String("ingress-name", "redirector", "Kubernetes service name to send traffic to")
	c := generateFS.String("ingress-class", "nginx", "Kubernetes ingress class set as ingressClassName")

	err := generateFS.Parse(os.Args[2:])
	if err != nil {
		log.Fatal(err.Error())
	}

	generateOutputPath = *p
	generateNamespace = *n
	generateServiceName = *s
	generateIngressName = *i
	generateIngressClassName = *c

}

func generateIngress(logger *slog.Logger) error {
	ingressClass := generateIngressClassName

	// TODO abstract this
	confPath, ok := os.LookupEnv("CONFIG_PATH")
	if !ok {
		logger.Error("CONFIG_PATH environment variable is not set, exiting")
		os.Exit(1)
	}

	cfg, confErr := loadConfig(logger, confPath)
	if confErr != nil {
		logger.Error("error parsing cfg file", "err", confErr.Error())
		return confErr
	}
	if cfg == nil {
		logger.Error("cfg nil after loading")
		return errors.New("cfg nil after loading")
	}

	logger.With("manifest_path", generateOutputPath).Info("generating manifest")
	ing := networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: networkingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateIngressName,
			Namespace: generateNamespace,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/use-regex": "true",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules:            []networkingv1.IngressRule{},
		},
	}

	// Because we use regular expressions, we have to leave it up to the Ingress Controller
	pt := networkingv1.PathTypeImplementationSpecific

	for domain, rules := range cfg.RuleMap {
		r := networkingv1.IngressRule{
			Host: domain,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{},
				},
			},
		}

		for _, rule := range rules {
			u, err := fromAsURL(logger, rule.From)
			if err != nil {
				logger.With("from", rule.From).With("to", rule.To).Warn("skipping ")
				return err
			}

			p := networkingv1.HTTPIngressPath{
				Path:     u.Path,
				PathType: &pt,
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: generateServiceName,
						Port: networkingv1.ServiceBackendPort{
							// TODO accept flag for this
							Number: 8484,
						},
					},
				},
			}

			r.IngressRuleValue.HTTP.Paths = append(r.IngressRuleValue.HTTP.Paths, p)

		}
		ing.Spec.Rules = append(ing.Spec.Rules, r)
	}

	m, err := kyaml.Marshal(ing)

	if err != nil {
		return err
	}

	f, err := os.Create(generateOutputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(m)

	return nil
}

func newMetricsServer() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return mux
}

func newServer(logger *slog.Logger, cache Cache, ac *AppConfig) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/", handleRequest(logger, cache, ac))
	mux.Handle("/status", handleStatus())
	return mux
}

func server(ctx context.Context, logger *slog.Logger) error {
	confPath, ok := os.LookupEnv("CONFIG_PATH")
	if !ok {
		logger.Error("CONFIG_PATH environment variable is not set, exiting")
		os.Exit(1)
	}

	cfg, confErr := loadConfig(logger, confPath)
	if confErr != nil {
		logger.Error("error parsing cfg file", "err", confErr.Error())
	}
	if cfg == nil {
		logger.Error("cfg nil after loading")
		os.Exit(1)
	}

	cache := NewInMemoryCache(ctx, logger, cfg.Cache.CleanupInterval, cfg.Cache.TTL)

	// start background config reloader
	go reloader(ctx, logger, confPath, cfg)

	srv := newServer(logger, cache, cfg)

	s := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           srv,
		ReadTimeout:       1 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	msrv := newMetricsServer()
	ms := &http.Server{
		Addr:         cfg.MetricsServerListenAddress,
		Handler:      msrv,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		IdleTimeout:  1 * time.Minute,
	}

	go func() {
		logger.WithGroup("server").Info("starting server", "listen_address", cfg.ListenAddress)
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithGroup("server").Error("error serving", "err", err.Error())
			os.Exit(1)
		}
	}()

	go func() {
		logger.WithGroup("metrics_server").Info("starting metrics")
		if err := ms.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithGroup("metrics_server").Error("error serving", "err", err.Error())
			os.Exit(1)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// block until message received
		<-ctx.Done()
		shutdownCtx := context.Background()
		shutdownCtx, cancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			logger.WithGroup("server").Error("error shutting down", "err", err.Error())
		} else {
			logger.Info("shutdown redirect server")
		}
	}()

	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutdownCtx := context.Background()
		shutdownCtx, cancel := context.WithTimeout(shutdownCtx, 2*time.Second)
		defer cancel()
		if err := ms.Shutdown(shutdownCtx); err != nil {
			logger.WithGroup("metrics_server").Error("error shutting down", "err", err.Error())
		} else {
			logger.Info("shutdown metrics server")
		}
	}()

	wg.Wait()
	return nil
}

func run(ctx context.Context, args []string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	parseArgs()

	logLevel := slog.LevelInfo
	logSrc := false
	if os.Getenv("DEBUG_LOGS") != "" {
		logLevel = slog.LevelDebug
		logSrc = true
	}

	logger := NewLogger(logLevel, logSrc)

	switch args[1] {
	case "server":
		return server(ctx, logger)
	case "generate":
		return generateIngress(logger)
	default:
		return errors.New("usage: redirector [server|generate]")
	}
}

func main() {
	ctx := context.Background()

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: redirector [server|generate]")
		os.Exit(1)
	}

	if err := run(ctx, os.Args); err != nil {
		fmt.Fprint(os.Stderr, err.Error(), "\n")
		os.Exit(1)
	}
}

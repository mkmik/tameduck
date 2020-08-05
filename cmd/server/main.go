package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bitnami-labs/flagenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

// flags are flags.
type Flags struct {
	Listen           string
	GracefulShutdown bool
	PreStopSleep     time.Duration
}

func (f *Flags) Bind(fs *flag.FlagSet) {
	if fs == nil {
		fs = flag.CommandLine
	}
	fs.StringVar(&f.Listen, "listen", ":8080", "Addr:port to listen to")
	fs.BoolVar(&f.GracefulShutdown, "graceful-shutdown", true, "Whether to shutdown gracefully")
	fs.DurationVar(&f.PreStopSleep, "pre-stop-sleep", 0, "How long to wait after receiving TERM before shutting down the server")
}

// metrics
var (
	reqDurationsHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency distributions.",
		Buckets: prometheus.DefBuckets,
	}, []string{"code", "method"})
)

func init() {
	prometheus.MustRegister(reqDurationsHistogram)
	prometheus.MustRegister(prometheus.NewBuildInfoCollector())
}

type healthz struct {
	sync.Mutex
	good bool
}

func (h *healthz) SetHealth(health bool) {
	h.Lock()
	defer h.Unlock()
	h.good = health
}

func (h *healthz) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	defer h.Unlock()
	if !h.good {
		klog.Infof("/healthz bad")
		http.Error(w, "bad health", http.StatusInternalServerError)
		return
	}
	klog.Infof("/healthz ok")
	fmt.Fprintf(w, "ok")
}

// handle implements a dummy handler
func handle(w http.ResponseWriter, r *http.Request) {
	klog.Infof("got request %q", r.URL)

	n := 1 + rand.Int31n(500)
	time.Sleep(time.Duration(n) * time.Millisecond)

	fmt.Fprintln(w, "ok")
}

// mainE is like main but receives parsed flags and can return error.
func mainE(flags Flags) error {
	klog.Infof("Flags %#v", flags)

	h := promhttp.InstrumentHandlerDuration(reqDurationsHistogram, http.HandlerFunc(handle))
	http.Handle("/", h)

	healthz := &healthz{good: true}
	http.Handle("/healthz", healthz)
	http.Handle("/metrics", promhttp.Handler())

	klog.Infof("listening to %s", flags.Listen)

	srv := &http.Server{
		Addr: flags.Listen,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.Fatal(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	klog.Infof("quitting...")
	if flags.GracefulShutdown {
		healthz.SetHealth(false)
		time.Sleep(flags.PreStopSleep)
		klog.Infof("shutting down http server")
		if err := srv.Shutdown(context.Background()); err != nil {
			return err
		}
		klog.Infof("server exited properly")
	}
	return nil
}

func main() {
	var flags Flags
	flags.Bind(nil)
	klog.InitFlags(nil)
	flagenv.SetFlagsFromEnv("TAMEDUCK", flag.CommandLine)
	flag.Parse()

	if err := mainE(flags); err != nil {
		klog.Fatal(err)
	}
}

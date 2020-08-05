package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

// flags are flags.
type Flags struct {
	Listen string
}

func (f *Flags) Bind(fs *flag.FlagSet) {
	if fs == nil {
		fs = flag.CommandLine
	}
	fs.StringVar(&f.Listen, "l", ":8080", "Addr:port to listen to")
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

// handle implements a dummy handler
func handle(w http.ResponseWriter, r *http.Request) {
	klog.Infof("got request %q", r.URL)

	n := 1 + rand.Int31n(500)
	time.Sleep(time.Duration(n) * time.Millisecond)

	fmt.Fprintln(w, "ok")
}

// mainE is like main but receives parsed flags and can return error.
func mainE(flags Flags) error {
	h := promhttp.InstrumentHandlerDuration(reqDurationsHistogram, http.HandlerFunc(handle))
	http.Handle("/", h)

	http.Handle("/metrics", promhttp.Handler())

	klog.Infof("listening to %s", flags.Listen)
	return http.ListenAndServe(flags.Listen, nil)
}

func main() {
	var flags Flags
	flags.Bind(nil)
	klog.InitFlags(nil)
	flag.Parse()

	if err := mainE(flags); err != nil {
		klog.Fatal(err)
	}
}

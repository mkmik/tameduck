package main

import (
	"context"
	"flag"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"k8s.io/klog/v2"
)

// flags are flags.
type Flags struct {
	Addr    string
	Listen  string
	Rate    float64
	Timeout time.Duration
}

func (f *Flags) Bind(fs *flag.FlagSet) {
	if fs == nil {
		fs = flag.CommandLine
	}
	fs.StringVar(&f.Addr, "a", "http://localhost:8080", "Addr:port to connect to hammer")
	fs.StringVar(&f.Listen, "l", ":8082", "Addr:port to listen to (for /metrics)")
	fs.Float64Var(&f.Rate, "r", 1, "Request per second")
	fs.DurationVar(&f.Timeout, "timeout", 60*time.Second, "timeout")
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

// hit performs a request to url u.
func hit(httpClient *http.Client, u string) {
	klog.Infof("hitting %s", u)
	_, err := httpClient.Get(u)
	if err != nil {
		klog.Error(err)
	}
}

// mainE is like main but receives parsed flags and can return error.
func mainE(flags Flags) error {

	http.Handle("/metrics", promhttp.Handler())

	klog.Infof("listening to %s", flags.Listen)
	go func() {
		if err := http.ListenAndServe(flags.Listen, nil); err != nil {
			klog.Fatal(err)
		}
	}()

	ctx := context.Background()
	lim := rate.NewLimiter(rate.Limit(flags.Rate), 1)

	httpClient := &http.Client{
		Timeout:   flags.Timeout,
		Transport: promhttp.InstrumentRoundTripperDuration(reqDurationsHistogram, http.DefaultTransport),
	}
	for {
		err := lim.Wait(ctx)
		if err != nil {
			return err
		}

		go hit(httpClient, flags.Addr)
	}
	return nil
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

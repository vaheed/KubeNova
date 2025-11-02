package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    ReconcileSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
        Namespace: "kubenova",
        Name:      "reconcile_seconds",
        Help:      "Duration of reconcile loops.",
        Buckets:   prometheus.DefBuckets,
    })
    EventsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "kubenova",
        Name:      "events_total",
        Help:      "Total events ingested from agents.",
    })
    AdapterErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "kubenova",
        Name:      "adapter_errors_total",
        Help:      "Adapter translation errors.",
    })
)

func init(){
    prometheus.MustRegister(ReconcileSeconds, EventsTotal, AdapterErrorsTotal)
}


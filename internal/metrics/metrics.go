package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// CommentsCreatedTotal counts created comments split by kind.
	CommentsCreatedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "comments_created_total",
		Help: "Number of comments created",
	}, []string{"kind"})

	// CommentsUpdatedTotal counts edits.
	CommentsUpdatedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "comments_updated_total",
		Help: "Number of comments updated",
	}, []string{"status"})

	// CommentsDeletedTotal counts deletions.
	CommentsDeletedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "comments_deleted_total",
		Help: "Number of comments deleted",
	}, []string{"status"})

	// CommentsDuration observes write request latency in the service layer.
	CommentsDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "comments_request_duration_seconds",
		Help:    "Comment write request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})
)

func init() {
	prometheus.MustRegister(CommentsCreatedTotal, CommentsUpdatedTotal, CommentsDeletedTotal, CommentsDuration)
}

func Created(kind string)                    { CommentsCreatedTotal.WithLabelValues(kind).Inc() }
func Updated(status string)                  { CommentsUpdatedTotal.WithLabelValues(status).Inc() }
func Deleted(status string)                  { CommentsDeletedTotal.WithLabelValues(status).Inc() }
func ObserveDuration(op string, sec float64) { CommentsDuration.WithLabelValues(op).Observe(sec) }

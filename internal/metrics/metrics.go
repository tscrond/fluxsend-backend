package metrics

import (
	"net/http"
	"runtime"
	runtimedebug "runtime/debug"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "fluxsend"

var (
	httpDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	httpSizeBuckets     = []float64{256, 512, 1024, 4 * 1024, 16 * 1024, 64 * 1024, 256 * 1024, 1024 * 1024, 4 * 1024 * 1024, 16 * 1024 * 1024}
	ioDurationBuckets   = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}
)

type Metrics struct {
	enabled bool

	registry *prometheus.Registry
	handler  http.Handler

	closeOnce sync.Once
	stopCh    chan struct{}
	wg        sync.WaitGroup

	httpRequestsTotal     *prometheus.CounterVec
	httpRequestDuration   *prometheus.HistogramVec
	httpRequestsInFlight  *prometheus.GaugeVec
	httpResponseSize      *prometheus.HistogramVec
	panicsTotal           *prometheus.CounterVec
	buildInfo             *prometheus.GaugeVec
	dbUp                  prometheus.Gauge
	dbErrorsTotal         *prometheus.CounterVec
	storageOperations     *prometheus.CounterVec
	storageDuration       *prometheus.HistogramVec
	storageBytesTotal     *prometheus.CounterVec
	emailSendTotal        *prometheus.CounterVec
	emailSendDuration     *prometheus.HistogramVec
	uploadSessionsTotal   *prometheus.CounterVec
	uploadPartsTotal      *prometheus.CounterVec
	uploadBytesTotal      *prometheus.CounterVec
	filesUploadedTotal    *prometheus.CounterVec
	filesDeletedTotal     *prometheus.CounterVec
	sharesCreatedTotal    *prometheus.CounterVec
	downloadsTotal        *prometheus.CounterVec
	workspacesCreated     prometheus.Counter
	workspacesDeleted     prometheus.Counter
	workspaceInvitesTotal *prometheus.CounterVec
	workspaceMembersTotal *prometheus.CounterVec
	apiKeysTotal          *prometheus.CounterVec
	apiKeyRequestsTotal   *prometheus.CounterVec
	authLoginTotal        *prometheus.CounterVec
	authSignupTotal       *prometheus.CounterVec
	authRateLimitBlocked  *prometheus.CounterVec
	quotaRejectionsTotal  *prometheus.CounterVec

	aggregateLastRefresh  prometheus.Gauge
	aggregateRefreshError prometheus.Counter
	aggregateCollector    *aggregateCollector
}

func New() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		enabled:  true,
		registry: registry,
		stopCh:   make(chan struct{}),
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests handled by FluxSend.",
		}, []string{"method", "route", "status_code"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds partitioned by method and route.",
			Buckets:   httpDurationBuckets,
		}, []string{"method", "route"}),
		httpRequestsInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "http_requests_in_flight",
			Help:      "Current number of in-flight HTTP requests by route.",
		}, []string{"route"}),
		httpResponseSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_response_size_bytes",
			Help:      "HTTP response sizes in bytes partitioned by method and route.",
			Buckets:   httpSizeBuckets,
		}, []string{"method", "route"}),
		panicsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "panics_total",
			Help:      "Recovered panics partitioned by route.",
		}, []string{"route"}),
		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_info",
			Help:      "Build metadata for the running FluxSend binary.",
		}, []string{"version", "go_version", "vcs_revision"}),
		dbUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_up",
			Help:      "Whether the configured database is reachable.",
		}),
		dbErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "db_errors_total",
			Help:      "Total number of database operation errors by query or operation name.",
		}, []string{"operation"}),
		storageOperations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "storage_operations_total",
			Help:      "Total number of object storage operations by provider, operation, and outcome.",
		}, []string{"provider", "operation", "outcome"}),
		storageDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "storage_operation_duration_seconds",
			Help:      "Object storage operation duration in seconds by provider and operation.",
			Buckets:   ioDurationBuckets,
		}, []string{"provider", "operation"}),
		storageBytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "storage_bytes_total",
			Help:      "Total number of bytes read from or written to object storage where known.",
		}, []string{"provider", "operation"}),
		emailSendTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "email_send_total",
			Help:      "Total number of email send attempts by provider and outcome.",
		}, []string{"provider", "outcome"}),
		emailSendDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "email_send_duration_seconds",
			Help:      "Email send duration in seconds by provider.",
			Buckets:   ioDurationBuckets,
		}, []string{"provider"}),
		uploadSessionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "upload_sessions_total",
			Help:      "Multipart upload session lifecycle events partitioned by upload kind and outcome.",
		}, []string{"kind", "outcome"}),
		uploadPartsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "upload_parts_total",
			Help:      "Multipart upload part results partitioned by upload kind and outcome.",
		}, []string{"kind", "outcome"}),
		uploadBytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "upload_bytes_total",
			Help:      "Total successfully accepted upload bytes by upload kind.",
		}, []string{"kind"}),
		filesUploadedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "files_uploaded_total",
			Help:      "Total successfully created files by upload kind.",
		}, []string{"kind"}),
		filesDeletedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "files_deleted_total",
			Help:      "Total successful file deletion events by deletion kind.",
		}, []string{"kind"}),
		sharesCreatedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "shares_created_total",
			Help:      "Total number of created shares by share type.",
		}, []string{"share_type"}),
		downloadsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "downloads_total",
			Help:      "Total number of download or download resolution attempts by channel, mechanism, and outcome.",
		}, []string{"channel", "mechanism", "outcome"}),
		workspacesCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "workspaces_created_total",
			Help:      "Total number of created workspaces.",
		}),
		workspacesDeleted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "workspaces_deleted_total",
			Help:      "Total number of deleted workspaces.",
		}),
		workspaceInvitesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "workspace_invites_total",
			Help:      "Workspace invite lifecycle events by action.",
		}, []string{"action"}),
		workspaceMembersTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "workspace_members_total",
			Help:      "Workspace membership changes by action.",
		}, []string{"action"}),
		apiKeysTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "api_keys_total",
			Help:      "API key lifecycle events by domain and action.",
		}, []string{"domain", "action"}),
		apiKeyRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "api_key_requests_total",
			Help:      "API key authenticated request outcomes by domain.",
		}, []string{"domain", "outcome"}),
		authLoginTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_login_total",
			Help:      "Authentication login or callback outcomes by provider.",
		}, []string{"provider", "outcome"}),
		authSignupTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_signup_total",
			Help:      "Authentication signup outcomes by provider.",
		}, []string{"provider", "outcome"}),
		authRateLimitBlocked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_rate_limit_blocked_total",
			Help:      "Authentication rate-limit blocks by scope.",
		}, []string{"scope"}),
		quotaRejectionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "quota_rejections_total",
			Help:      "Plan or quota rejections by limit name.",
		}, []string{"limit"}),
		aggregateLastRefresh: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "metrics_snapshot_last_refresh_timestamp_seconds",
			Help:      "Unix timestamp of the last successful refresh of cached DB-derived business gauges.",
		}),
		aggregateRefreshError: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "metrics_snapshot_refresh_errors_total",
			Help:      "Total number of errors while refreshing cached DB-derived business gauges.",
		}),
	}

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.httpRequestsTotal,
		m.httpRequestDuration,
		m.httpRequestsInFlight,
		m.httpResponseSize,
		m.panicsTotal,
		m.buildInfo,
		m.dbUp,
		m.dbErrorsTotal,
		m.storageOperations,
		m.storageDuration,
		m.storageBytesTotal,
		m.emailSendTotal,
		m.emailSendDuration,
		m.uploadSessionsTotal,
		m.uploadPartsTotal,
		m.uploadBytesTotal,
		m.filesUploadedTotal,
		m.filesDeletedTotal,
		m.sharesCreatedTotal,
		m.downloadsTotal,
		m.workspacesCreated,
		m.workspacesDeleted,
		m.workspaceInvitesTotal,
		m.workspaceMembersTotal,
		m.apiKeysTotal,
		m.apiKeyRequestsTotal,
		m.authLoginTotal,
		m.authSignupTotal,
		m.authRateLimitBlocked,
		m.quotaRejectionsTotal,
		m.aggregateLastRefresh,
		m.aggregateRefreshError,
	)

	m.registerBuildInfo()
	m.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
	return m
}

func NewDisabled() *Metrics {
	return &Metrics{handler: http.NotFoundHandler()}
}

func (m *Metrics) Enabled() bool {
	return m != nil && m.enabled
}

func (m *Metrics) Handler() http.Handler {
	if m == nil || m.handler == nil {
		return http.NotFoundHandler()
	}
	return m.handler
}

func (m *Metrics) Close() {
	if !m.Enabled() {
		return
	}

	m.closeOnce.Do(func() {
		close(m.stopCh)
		m.wg.Wait()
	})
}

func (m *Metrics) registerBuildInfo() {
	version := "unknown"
	revision := ""
	if info, ok := runtimedebug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			version = info.Main.Version
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				revision = setting.Value
				break
			}
		}
	}
	m.buildInfo.WithLabelValues(version, runtime.Version(), revision).Set(1)
}

func (m *Metrics) observeDBError(operation string, err error) {
	if !m.Enabled() || err == nil {
		return
	}
	m.dbErrorsTotal.WithLabelValues(operation).Inc()
}

func (m *Metrics) RecordUploadSession(kind, outcome string) {
	if !m.Enabled() {
		return
	}
	m.uploadSessionsTotal.WithLabelValues(kind, outcome).Inc()
}

func (m *Metrics) RecordUploadPart(kind, outcome string) {
	if !m.Enabled() {
		return
	}
	m.uploadPartsTotal.WithLabelValues(kind, outcome).Inc()
}

func (m *Metrics) AddUploadBytes(kind string, size int64) {
	if !m.Enabled() || size <= 0 {
		return
	}
	m.uploadBytesTotal.WithLabelValues(kind).Add(float64(size))
}

func (m *Metrics) RecordFileUploaded(kind string) {
	if !m.Enabled() {
		return
	}
	m.filesUploadedTotal.WithLabelValues(kind).Inc()
}

func (m *Metrics) AddFilesDeleted(kind string, count int) {
	if !m.Enabled() || count <= 0 {
		return
	}
	m.filesDeletedTotal.WithLabelValues(kind).Add(float64(count))
}

func (m *Metrics) RecordShareCreated(shareType string) {
	if !m.Enabled() {
		return
	}
	m.sharesCreatedTotal.WithLabelValues(shareType).Inc()
}

func (m *Metrics) RecordDownload(channel, mechanism, outcome string) {
	if !m.Enabled() {
		return
	}
	m.downloadsTotal.WithLabelValues(channel, mechanism, outcome).Inc()
}

func (m *Metrics) RecordWorkspaceCreated() {
	if !m.Enabled() {
		return
	}
	m.workspacesCreated.Inc()
}

func (m *Metrics) RecordWorkspaceDeleted() {
	if !m.Enabled() {
		return
	}
	m.workspacesDeleted.Inc()
}

func (m *Metrics) RecordWorkspaceInvite(action string) {
	if !m.Enabled() {
		return
	}
	m.workspaceInvitesTotal.WithLabelValues(action).Inc()
}

func (m *Metrics) RecordWorkspaceMember(action string) {
	if !m.Enabled() {
		return
	}
	m.workspaceMembersTotal.WithLabelValues(action).Inc()
}

func (m *Metrics) RecordAPIKey(domain, action string) {
	if !m.Enabled() {
		return
	}
	m.apiKeysTotal.WithLabelValues(domain, action).Inc()
}

func (m *Metrics) RecordAPIKeyRequest(domain, outcome string) {
	if !m.Enabled() {
		return
	}
	m.apiKeyRequestsTotal.WithLabelValues(domain, outcome).Inc()
}

func (m *Metrics) RecordAuthLogin(provider, outcome string) {
	if !m.Enabled() {
		return
	}
	m.authLoginTotal.WithLabelValues(provider, outcome).Inc()
}

func (m *Metrics) RecordAuthSignup(provider, outcome string) {
	if !m.Enabled() {
		return
	}
	m.authSignupTotal.WithLabelValues(provider, outcome).Inc()
}

func (m *Metrics) RecordAuthRateLimitBlocked(scope string) {
	if !m.Enabled() {
		return
	}
	m.authRateLimitBlocked.WithLabelValues(scope).Inc()
}

func (m *Metrics) RecordQuotaRejection(limit string) {
	if !m.Enabled() {
		return
	}
	m.quotaRejectionsTotal.WithLabelValues(limit).Inc()
}

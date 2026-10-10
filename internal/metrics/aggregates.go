package metrics

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

var (
	usersTotalDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "users_total"),
		"Total number of registered users.",
		nil,
		nil,
	)
	usersByPlanDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "users_by_plan"),
		"Total number of users by normalized plan label.",
		[]string{"plan"},
		nil,
	)
	filesTotalDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "files_total"),
		"Total number of stored files across personal and workspace storage.",
		nil,
		nil,
	)
	storageBytesDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "storage_bytes"),
		"Total stored bytes across personal and workspace storage.",
		nil,
		nil,
	)
	workspacesTotalDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "workspaces_total"),
		"Total number of workspaces.",
		nil,
		nil,
	)
	activeSharesDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "active_shares"),
		"Total number of non-expired shares.",
		nil,
		nil,
	)
	activeAPIKeysDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "active_api_keys"),
		"Total number of non-revoked API keys by domain.",
		[]string{"domain"},
		nil,
	)
	knownPlanLabels = []string{"free", "developer", "enterprise", "custom", "unassigned"}
	knownKeyDomains = []string{"private", "workspace"}
)

const (
	aggregateRefreshInterval = 30 * time.Second
	aggregateRefreshTimeout  = 3 * time.Second
)

type aggregateCollector struct {
	queries       sqlc.Querier
	lastRefresh   prometheus.Gauge
	refreshErrors prometheus.Counter

	mu       sync.RWMutex
	snapshot aggregateSnapshot
}

type aggregateSnapshot struct {
	hasData       bool
	usersTotal    int64
	usersByPlan   map[string]int64
	filesTotal    int64
	storageBytes  int64
	workspaces    int64
	activeShares  int64
	activeAPIKeys map[string]int64
}

func (m *Metrics) AttachDatabase(db *sql.DB, queries sqlc.Querier) {
	if !m.Enabled() || db == nil || queries == nil || m.aggregateCollector != nil {
		return
	}

	m.dbUp.Set(0)
	m.startDBMonitor(db)

	collector := &aggregateCollector{
		queries:       queries,
		lastRefresh:   m.aggregateLastRefresh,
		refreshErrors: m.aggregateRefreshError,
	}
	m.aggregateCollector = collector
	m.registry.MustRegister(collector)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		collector.run(m.stopCh)
	}()
}

func (m *Metrics) startDBMonitor(db *sql.DB) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		ping := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if err := db.PingContext(ctx); err != nil {
				m.dbUp.Set(0)
				m.observeDBError("ping", err)
				return
			}
			m.dbUp.Set(1)
		}

		ping()
		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				ping()
			}
		}
	}()
}

func (c *aggregateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- usersTotalDesc
	ch <- usersByPlanDesc
	ch <- filesTotalDesc
	ch <- storageBytesDesc
	ch <- workspacesTotalDesc
	ch <- activeSharesDesc
	ch <- activeAPIKeysDesc
}

func (c *aggregateCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	snapshot := c.snapshot
	c.mu.RUnlock()

	if !snapshot.hasData {
		return
	}

	ch <- prometheus.MustNewConstMetric(usersTotalDesc, prometheus.GaugeValue, float64(snapshot.usersTotal))
	for _, plan := range knownPlanLabels {
		ch <- prometheus.MustNewConstMetric(usersByPlanDesc, prometheus.GaugeValue, float64(snapshot.usersByPlan[plan]), plan)
	}
	ch <- prometheus.MustNewConstMetric(filesTotalDesc, prometheus.GaugeValue, float64(snapshot.filesTotal))
	ch <- prometheus.MustNewConstMetric(storageBytesDesc, prometheus.GaugeValue, float64(snapshot.storageBytes))
	ch <- prometheus.MustNewConstMetric(workspacesTotalDesc, prometheus.GaugeValue, float64(snapshot.workspaces))
	ch <- prometheus.MustNewConstMetric(activeSharesDesc, prometheus.GaugeValue, float64(snapshot.activeShares))
	for _, domain := range knownKeyDomains {
		ch <- prometheus.MustNewConstMetric(activeAPIKeysDesc, prometheus.GaugeValue, float64(snapshot.activeAPIKeys[domain]), domain)
	}
}

func (c *aggregateCollector) run(stop <-chan struct{}) {
	c.refresh()

	ticker := time.NewTicker(aggregateRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.refresh()
		}
	}
}

func (c *aggregateCollector) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), aggregateRefreshTimeout)
	defer cancel()

	usersTotal, err := c.queries.MetricsUsersTotal(ctx)
	if err != nil {
		c.refreshErrors.Inc()
		return
	}

	usersByPlanRows, err := c.queries.MetricsUsersByPlan(ctx)
	if err != nil {
		c.refreshErrors.Inc()
		return
	}

	filesAndStorage, err := c.queries.MetricsFilesAndStorageTotals(ctx)
	if err != nil {
		c.refreshErrors.Inc()
		return
	}

	workspacesTotal, err := c.queries.MetricsWorkspacesTotal(ctx)
	if err != nil {
		c.refreshErrors.Inc()
		return
	}

	activeShares, err := c.queries.MetricsActiveSharesTotal(ctx)
	if err != nil {
		c.refreshErrors.Inc()
		return
	}

	activeAPIKeys, err := c.queries.MetricsActiveAPIKeysByDomain(ctx)
	if err != nil {
		c.refreshErrors.Inc()
		return
	}

	usersByPlan := make(map[string]int64, len(knownPlanLabels))
	for _, row := range usersByPlanRows {
		usersByPlan[normalizePlanLabel(row.PlanName)] += row.UserCount
	}

	activeKeysByDomain := map[string]int64{"private": 0, "workspace": 0}
	for _, row := range activeAPIKeys {
		activeKeysByDomain[row.Domain] = row.ActiveCount
	}

	c.mu.Lock()
	c.snapshot = aggregateSnapshot{
		hasData:       true,
		usersTotal:    usersTotal,
		usersByPlan:   usersByPlan,
		filesTotal:    filesAndStorage.TotalFiles,
		storageBytes:  filesAndStorage.TotalStorageBytes,
		workspaces:    workspacesTotal,
		activeShares:  activeShares,
		activeAPIKeys: activeKeysByDomain,
	}
	c.mu.Unlock()

	c.lastRefresh.SetToCurrentTime()
}

func normalizePlanLabel(label string) string {
	switch label {
	case "free", "developer", "enterprise":
		return label
	case "", "unassigned":
		return "unassigned"
	default:
		return "custom"
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tscrond/fluxsend-backend/internal/config"
	appmetrics "github.com/tscrond/fluxsend-backend/internal/metrics"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestAPIServerMetricsUseRoutePatterns(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)
	metrics := appmetrics.New()
	srv.CoreHandlers.metrics = metrics

	token := "route-label-token-123"
	req := httptest.NewRequest(http.MethodPost, "/share/info/"+token, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRR := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRR, metricsReq)

	require.Equal(t, http.StatusOK, metricsRR.Code)
	assert.Contains(t, metricsRR.Body.String(), `route="/share/info/{token}"`)
	assert.NotContains(t, metricsRR.Body.String(), token)
}

func TestAPIServerInFlightUsesMatchedRoutePattern(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)
	metrics := appmetrics.New()
	srv.CoreHandlers.metrics = metrics

	req := httptest.NewRequest(http.MethodGet, "/auth/providers", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assertInFlightMetric(t, metrics, "/auth/providers")
}

func TestCLIServerInFlightUsesMatchedRoutePattern(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestCLIServer(ctrl, "/api")
	metrics := appmetrics.New()
	srv.CoreHandlers.metrics = metrics

	req := httptest.NewRequest(http.MethodGet, "/api/swagger/json", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assertInFlightMetric(t, metrics, "/api/swagger/json")
}

func TestAdminServerInFlightUsesMatchedRoutePattern(t *testing.T) {
	srv := NewAdminServer(config.BackendConfig{}, AdminServerDependencies{
		CoreHandlersDependencies: CoreHandlersDependencies{
			Log: zap.NewNop().Sugar(),
		},
	})
	metrics := appmetrics.New()
	srv.CoreHandlers.metrics = metrics

	req := httptest.NewRequest(http.MethodGet, "/admin/health", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assertInFlightMetric(t, metrics, "/admin/health")
}

func assertInFlightMetric(t *testing.T, metrics *appmetrics.Metrics, route string) {
	t.Helper()

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRR := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRR, metricsReq)

	require.Equal(t, http.StatusOK, metricsRR.Code)
	assert.Contains(t, metricsRR.Body.String(), `fluxsend_http_requests_in_flight{route="`+route+`"} 0`)
	assert.NotContains(t, metricsRR.Body.String(), `fluxsend_http_requests_in_flight{route="unmatched"}`)
}

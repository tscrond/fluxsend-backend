package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appmetrics "github.com/tscrond/fluxsend-backend/internal/metrics"
	"go.uber.org/mock/gomock"
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

package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func (m *Metrics) HTTPMiddleware() func(http.Handler) http.Handler {
	if !m.Enabled() {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				finalRoute := routePattern(r)
				statusCode := ww.Status()
				if recovered := recover(); recovered != nil {
					m.panicsTotal.WithLabelValues(finalRoute).Inc()
					if statusCode == 0 {
						statusCode = http.StatusInternalServerError
					}
					m.httpRequestsTotal.WithLabelValues(r.Method, finalRoute, strconv.Itoa(statusCode)).Inc()
					m.httpRequestDuration.WithLabelValues(r.Method, finalRoute).Observe(time.Since(started).Seconds())
					m.httpResponseSize.WithLabelValues(r.Method, finalRoute).Observe(float64(ww.BytesWritten()))
					panic(recovered)
				}

				if statusCode == 0 {
					statusCode = http.StatusOK
				}
				m.httpRequestsTotal.WithLabelValues(r.Method, finalRoute, strconv.Itoa(statusCode)).Inc()
				m.httpRequestDuration.WithLabelValues(r.Method, finalRoute).Observe(time.Since(started).Seconds())
				m.httpResponseSize.WithLabelValues(r.Method, finalRoute).Observe(float64(ww.BytesWritten()))
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// HTTPInFlightMiddleware tracks the current number of requests per matched
// route. It must be registered after routing so that the route pattern is
// available when the request enters the middleware.
func (m *Metrics) HTTPInFlightMiddleware() func(http.Handler) http.Handler {
	if !m.Enabled() {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gauge := m.httpRequestsInFlight.WithLabelValues(routePattern(r))
			gauge.Inc()
			defer gauge.Dec()

			next.ServeHTTP(w, r)
		})
	}
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "unmatched"
	}

	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "unmatched"
	}

	pattern := strings.TrimSpace(rctx.RoutePattern())
	if pattern == "" {
		return "unmatched"
	}

	return pattern
}

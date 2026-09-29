package metrics

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
)

type fakeObjectStorage struct {
	putErr        error
	uploadPartErr error
	signedURLErr  error
}

func (f *fakeObjectStorage) UploadPart(ctx context.Context, bucket string, key string, uploadID string, partNumber int32, body io.Reader, size int64) (*storagetypes.UploadPartResult, error) {
	return &storagetypes.UploadPartResult{PartNumber: partNumber, Size: size}, f.uploadPartErr
}

func (f *fakeObjectStorage) AbortMultipartUpload(ctx context.Context, bucket string, key string, uploadID string) error {
	return nil
}

func (f *fakeObjectStorage) CompleteMultipartUpload(ctx context.Context, bucket string, key string, uploadID string, parts []storagetypes.CompletedPart) (*storagetypes.CompleteMultipartUploadResult, error) {
	return &storagetypes.CompleteMultipartUploadResult{ETag: "etag"}, nil
}

func (f *fakeObjectStorage) CreateMultipartUpload(ctx context.Context, bucket, uploadPath, contentType string) (*string, error) {
	uploadID := "upload-id"
	return &uploadID, nil
}

func (f *fakeObjectStorage) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (*storagetypes.PutObjectResult, error) {
	return &storagetypes.PutObjectResult{Size: size, ContentType: contentType}, f.putErr
}

func (f *fakeObjectStorage) BucketExists(ctx context.Context, fullBucketName string) (bool, error) {
	return true, nil
}

func (f *fakeObjectStorage) CreateBucketIfNotExists(ctx context.Context, userId string) error {
	return nil
}

func (f *fakeObjectStorage) GetUserBucketData(ctx context.Context, id string) (any, error) {
	return map[string]string{"bucket": id}, nil
}

func (f *fakeObjectStorage) GetBucketBaseName() string {
	return "bucket"
}

func (f *fakeObjectStorage) GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time, contentDisposition string) (string, error) {
	return "https://example.invalid/object", f.signedURLErr
}

func (f *fakeObjectStorage) DeleteObjectFromBucket(ctx context.Context, object, bucket string) error {
	return nil
}

func (f *fakeObjectStorage) DeleteObjectsFromBucket(ctx context.Context, objects []string, bucket string) error {
	return nil
}

func (f *fakeObjectStorage) MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error {
	return nil
}

func (f *fakeObjectStorage) DeleteBucket(ctx context.Context, bucket string) error {
	return nil
}

func (f *fakeObjectStorage) Close() error {
	return nil
}

type fakeEmailSender struct {
	err error
}

func (f *fakeEmailSender) Send(messageConfig mailtypes.MessageConfig) (any, error) {
	return map[string]string{"id": "message-id"}, f.err
}

func TestMetricsRecordersIncrementExpectedSeries(t *testing.T) {
	m := New()

	m.RecordUploadSession("private", "initiated")
	m.RecordUploadPart("workspace", "completed")
	m.AddUploadBytes("private", 42)
	m.RecordFileUploaded("workspace")
	m.RecordShareCreated("email")
	m.RecordDownload("public", "signed_url", "success")
	m.RecordQuotaRejection("files")
	m.RecordAPIKeyRequest("private", "allowed")
	m.RecordAuthLogin("password", "success")

	assert.Equal(t, 1.0, promtest.ToFloat64(m.uploadSessionsTotal.WithLabelValues("private", "initiated")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.uploadPartsTotal.WithLabelValues("workspace", "completed")))
	assert.Equal(t, 42.0, promtest.ToFloat64(m.uploadBytesTotal.WithLabelValues("private")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.filesUploadedTotal.WithLabelValues("workspace")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.sharesCreatedTotal.WithLabelValues("email")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.downloadsTotal.WithLabelValues("public", "signed_url", "success")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.quotaRejectionsTotal.WithLabelValues("files")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.apiKeyRequestsTotal.WithLabelValues("private", "allowed")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.authLoginTotal.WithLabelValues("password", "success")))
}

func TestMetricsWrappersRecordDependencySeries(t *testing.T) {
	m := New()
	storage := m.WrapObjectStorage("s3", &fakeObjectStorage{})
	mailer := m.WrapEmailSender("standard", &fakeEmailSender{})

	_, err := storage.PutObject(context.Background(), "bucket", "object", bytes.NewBufferString("payload"), 7, "text/plain")
	require.NoError(t, err)
	_, err = storage.GenerateSignedURL(context.Background(), "bucket", "object", time.Now().Add(time.Minute), "")
	require.NoError(t, err)
	_, err = mailer.Send(mailtypes.MessageConfig{To: []string{"person@example.com"}})
	require.NoError(t, err)

	assert.Equal(t, 1.0, promtest.ToFloat64(m.storageOperations.WithLabelValues("s3", "put_object", "success")))
	assert.Equal(t, 7.0, promtest.ToFloat64(m.storageBytesTotal.WithLabelValues("s3", "put_object")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.storageOperations.WithLabelValues("s3", "generate_signed_url", "success")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.emailSendTotal.WithLabelValues("smtp", "success")))

	var _ storagetypes.ObjectStorage = storage
	var _ mailtypes.EmailSender = mailer
}

func TestMetricsHTTPMiddlewareRecordsRouteLabelsAndHistograms(t *testing.T) {
	m := New()
	r := chi.NewRouter()
	r.Use(m.HTTPMiddleware())
	r.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/items/abc123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, 1.0, promtest.ToFloat64(m.httpRequestsTotal.WithLabelValues(http.MethodGet, "/items/{id}", "201")))
	assert.Equal(t, 0.0, promtest.ToFloat64(m.httpRequestsInFlight.WithLabelValues("/items/{id}")))

	durationMetric := findMetric(t, m.registry, "fluxsend_http_request_duration_seconds", map[string]string{
		"method": http.MethodGet,
		"route":  "/items/{id}",
	})
	require.Equal(t, uint64(1), durationMetric.GetHistogram().GetSampleCount())

	responseSizeMetric := findMetric(t, m.registry, "fluxsend_http_response_size_bytes", map[string]string{
		"method": http.MethodGet,
		"route":  "/items/{id}",
	})
	require.Equal(t, uint64(1), responseSizeMetric.GetHistogram().GetSampleCount())
}

func TestMetricsHandlerExposesBuildInfo(t *testing.T) {
	m := New()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	m.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, rr.Body.String(), "fluxsend_build_info")
	assert.Contains(t, rr.Body.String(), "fluxsend_build_info{")
	assert.Contains(t, rr.Body.String(), "} 1")
}

func TestDisabledMetricsDoesNotPanic(t *testing.T) {
	m := NewDisabled()

	require.NotPanics(t, func() {
		m.RecordUploadSession("private", "initiated")
		m.RecordDownload("public", "signed_url", "success")

		handler := m.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/anything", nil))

		storage := m.WrapObjectStorage("s3", &fakeObjectStorage{})
		_, _ = storage.PutObject(context.Background(), "bucket", "object", bytes.NewBufferString("payload"), 7, "text/plain")

		sender := m.WrapEmailSender("standard", &fakeEmailSender{})
		_, _ = sender.Send(mailtypes.MessageConfig{To: []string{"person@example.com"}})
	})
}

func findMetric(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string) *dto.Metric {
	t.Helper()

	families, err := registry.Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricHasLabels(metric, labels) {
				return metric
			}
		}
	}

	t.Fatalf("metric %s with labels %v not found", name, labels)
	return nil
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	for key, expected := range labels {
		matched := false
		for _, label := range metric.GetLabel() {
			if label.GetName() == key && label.GetValue() == expected {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func TestMetricsHandlerOutputIncludesBusinessSeriesAfterRecording(t *testing.T) {
	m := New()
	m.RecordShareCreated("email")

	rr := httptest.NewRecorder()
	m.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	assert.True(t, strings.Contains(rr.Body.String(), "fluxsend_shares_created_total"))
}

// Package storage: реализация Storage поверх MinIO/S3.
//
// Подход к multi-tenant изоляции:
//   - Используется один общий bucket (cfg.Bucket), а tenant-изоляция
//     осуществляется через префикс ключа `tenants/{tenant_id}/...`,
//     который формирует handler.DocumentHandler.
//   - Это упрощает деплой (один bucket = одна quota = одна lifecycle policy)
//     и не требует MakeBucket-прав на каждый новый тенант.
//   - Альтернатива «per-tenant bucket» оставлена в TODO ниже — она безопаснее,
//     но упирается в лимит buckets на S3-провайдере и усложняет provisioning.
//
// At-rest шифрование (см. docs/security-architecture.md § 5.1):
//   - По умолчанию включается SSE-S3 (AES-256, ключи на стороне S3).
//   - Опционально через STORAGE_MINIO_SSE_KMS_KEY включается SSE-KMS,
//     ключ должен быть зарегистрирован в KES/Vault Transit.
//
// TODO(infra):
//   - Per-tenant bucket option (env STORAGE_MINIO_BUCKET_PER_TENANT=true).
//   - SSE-KMS через Vault Transit (а не напрямую KMS-id).
//   - Bucket lifecycle policy: 5-year retention для compliance (115-ФЗ).
//   - Раздельный PII-bucket с более жёсткой политикой доступа.
//   - Health-probe для /minio/health/live (см. main.go).
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/encrypt"
)

// MinIOConfig — конфигурация подключения к MinIO/S3.
//
// Поля приходят из env-переменных в cmd/server/main.go.
// Endpoint указывается без схемы (host:port), UseSSL переключает https.
type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool

	// Region опционально (по умолчанию "us-east-1" для совместимости с MinIO).
	Region string

	// SSEEnabled включает server-side encryption.
	// При SSEKMSKeyID == "" применяется SSE-S3 (AES-256, ключи S3-провайдера).
	// При непустом SSEKMSKeyID применяется SSE-KMS.
	SSEEnabled  bool
	SSEKMSKeyID string

	// HTTPClient опционально — позволяет в тестах подменить транспорт
	// (например, на httptest.Server). Если nil — используется дефолтный
	// http.Client из minio-go.
	HTTPClient *http.Client
}

// ErrInvalidConfig возвращается NewMinIOStorage при отсутствии обязательных полей.
var ErrInvalidConfig = errors.New("storage/minio: invalid config")

// MinIOStorage реализует Storage через minio-go/v7.
type MinIOStorage struct {
	client *minio.Client
	bucket string
	region string

	sseEnabled bool
	sse        encrypt.ServerSide // nil, если SSE отключён
}

// NewMinIOStorage валидирует конфиг, создаёт minio-клиент, проверяет/создаёт bucket.
//
// Возвращает ErrInvalidConfig при неполном cfg, чтобы сервис падал на старте,
// а не на первом Put.
func NewMinIOStorage(cfg MinIOConfig) (*MinIOStorage, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("%w: endpoint is required", ErrInvalidConfig)
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("%w: access/secret key is required", ErrInvalidConfig)
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("%w: bucket is required", ErrInvalidConfig)
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	if cfg.HTTPClient != nil {
		opts.Transport = cfg.HTTPClient.Transport
	}

	client, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("storage/minio: new client: %w", err)
	}

	// Bucket auto-creation: BucketExists + MakeBucket.
	// Используем короткий тайм-аут на bootstrap, чтобы сервис не зависал
	// при недоступном MinIO.
	bootstrapCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, bucketErr := client.BucketExists(bootstrapCtx, cfg.Bucket)
	if bucketErr != nil {
		return nil, fmt.Errorf("storage/minio: BucketExists(%s): %w", cfg.Bucket, bucketErr)
	}
	if !exists {
		makeErr := client.MakeBucket(bootstrapCtx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region})
		if makeErr != nil {
			// Возможно, bucket был создан параллельно — перепроверим.
			ok, _ := client.BucketExists(bootstrapCtx, cfg.Bucket)
			if !ok {
				return nil, fmt.Errorf("storage/minio: MakeBucket(%s): %w", cfg.Bucket, makeErr)
			}
		}
	}

	s := &MinIOStorage{
		client:     client,
		bucket:     cfg.Bucket,
		region:     cfg.Region,
		sseEnabled: cfg.SSEEnabled,
	}

	if cfg.SSEEnabled {
		if cfg.SSEKMSKeyID != "" {
			kms, kmsErr := encrypt.NewSSEKMS(cfg.SSEKMSKeyID, nil)
			if kmsErr != nil {
				return nil, fmt.Errorf("storage/minio: NewSSEKMS: %w", kmsErr)
			}
			s.sse = kms
		} else {
			s.sse = encrypt.NewSSE() // SSE-S3, AES-256, ключи на стороне S3
		}
	}

	return s, nil
}

// Put сохраняет content по ключу key и возвращает канонический путь s3://bucket/key.
//
// sizeHint=-1 заставляет minio-go использовать multipart upload с авточанками
// (5 MiB), что важно для streaming-загрузок (handler.DocumentHandler передаёт
// io.TeeReader, реальный размер заранее неизвестен).
func (s *MinIOStorage) Put(ctx context.Context, key string, content io.Reader, contentType string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}
	if s.sseEnabled && s.sse != nil {
		opts.ServerSideEncryption = s.sse
	}

	start := time.Now()
	info, err := s.client.PutObject(ctx, s.bucket, key, content, -1, opts)
	if err != nil {
		return "", fmt.Errorf("storage/minio: PutObject(%s/%s): %w", s.bucket, key, err)
	}
	// Метрики: размер и латентность. Логирование выполняется на стороне handler;
	// здесь оставляем только observable info через возвращаемый путь.
	_ = info
	_ = start

	return fmt.Sprintf("s3://%s/%s", s.bucket, key), nil
}

// Get возвращает поток чтения объекта по ключу.
//
// minio-go GetObject ленивый: ошибка "key not found" приходит не на GetObject,
// а на первой Stat()/Read(). Поэтому делаем Stat сразу, чтобы превратить
// "NoSuchKey" в ErrObjectNotFound на верхнем уровне (хендлеры ожидают этот контракт).
func (s *MinIOStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage/minio: GetObject(%s/%s): %w", s.bucket, key, err)
	}

	if _, statErr := obj.Stat(); statErr != nil {
		_ = obj.Close()
		if isNotFound(statErr) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("storage/minio: Stat(%s/%s): %w", s.bucket, key, statErr)
	}

	return obj, nil
}

// Exists проверяет наличие объекта по ключу через StatObject.
func (s *MinIOStorage) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("storage/minio: StatObject(%s/%s): %w", s.bucket, key, err)
	}
	return true, nil
}

// isNotFound определяет «объект отсутствует» по minio-go ErrorResponse.
// minio-go возвращает Code "NoSuchKey" для отсутствующего объекта и
// "NoSuchBucket" для отсутствующего bucket — оба трактуем как not-found
// на уровне Storage-API.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "NoSuchKey", "NoSuchBucket", "NotFound":
		return true
	}
	// Fallback: некоторые S3-совместимые сервисы возвращают 404 без code.
	if resp.StatusCode == http.StatusNotFound {
		return true
	}
	// Дополнительный fallback по тексту ошибки (на случай custom transport).
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist")
}

// MaskCredentials возвращает безопасный для лога вариант MinIOConfig:
// access/secret обрезаются до первых 4 символов с маскировкой.
//
// Используется в cmd/server/main.go для structured logging при старте.
func MaskCredentials(cfg MinIOConfig) map[string]any {
	return map[string]any{
		"endpoint":     cfg.Endpoint,
		"bucket":       cfg.Bucket,
		"region":       cfg.Region,
		"use_ssl":      cfg.UseSSL,
		"access_key":   maskSecret(cfg.AccessKey),
		"secret_key":   maskSecret(cfg.SecretKey),
		"sse_enabled":  cfg.SSEEnabled,
		"sse_kms_used": cfg.SSEKMSKeyID != "",
	}
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

// Совместимость с прежним поведением: пустой STORAGE_MINIO_SSE_ENABLED трактуем как true,
// чтобы безопасные дефолты включались автоматически (см. § 5.1).
// Используется только из main.go.
func ParseSSEEnabledEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_MINIO_SSE_ENABLED")))
	if v == "" {
		return true
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

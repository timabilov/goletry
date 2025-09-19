// services/url_cache_service.go
package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
)

// This is the duration for which your presigned URLs will be valid.
const presignedURLExpiration = 15 * time.Minute

// slight less that expiration
const cacheCleanupInterval = 12 * time.Minute

type URLCacheServiceProvider interface {
	GetReadURL(ctx context.Context, objectKey string) (string, error)
}

// URLCacheService now uses eko/gocache.
type URLCacheService struct {
	cache      *cache.LoadableCache[string]
	bucketName string
}

// NewURLCacheService creates a new instance using a Loadable Ristretto cache.
func NewURLCacheService(awsService *AWSService, bucketName string) (*URLCacheService, error) {
	// 1. Initialize the Ristretto cache client
	ristrettoCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // 10M
		MaxCost:     1 << 27, // 1GB
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ristretto cache: %w", err)
	}

	// This is the correct, non-generic way to instantiate the store.
	// The store itself works with `any`, and the cache wrapper provides type safety.
	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache) // <-- CORRECTED LINE

	// 2. Define the "load" function that will be called on a cache miss.
	loadFunction := func(ctx context.Context, key any) (string, []store.Option, error) {
		objectKey, ok := key.(string)
		if !ok {
			return "", nil, fmt.Errorf("invalid key type provided to URL cache: expected string, got %T", key)
		}

		log.Printf("CACHE MISS for key: %s. Generating new presigned URL.", objectKey)
		url, err := awsService.GetPresignedR2FileReadURL(ctx, bucketName, objectKey)
		return url, []store.Option{store.WithExpiration(cacheCleanupInterval)}, err
	}

	// 3. Create the Loadable Cache instance
	loadableCache := cache.NewLoadable[string](
		loadFunction,
		cache.New[string](ristrettoStore),
	)
	fmt.Println("Initialized URLCacheService with Ristretto cache!")
	return &URLCacheService{
		cache:      loadableCache,
		bucketName: bucketName,
	}, nil
}

// GetReadURL remains the same.
func (s *URLCacheService) GetReadURL(ctx context.Context, objectKey string) (string, error) {
	if objectKey == "" {
		return "", nil
	}

	return s.cache.Get(ctx, objectKey)
}

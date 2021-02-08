// SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"errors"
	"io"

	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var (
	// ErrNotFound is a error that indicates that the file is not cached
	ErrNotFound = errors.New("not cached")
)

// CacheDirEnvName is the name of the environment variable that configures cache directory.
const CacheDirEnvName = "OCI_CACHE_DIR"

// Cache is the interface for a oci cache
type Cache interface {
	io.Closer
	Get(desc ocispecv1.Descriptor) (io.ReadCloser, error)
	Add(desc ocispecv1.Descriptor, reader io.ReadCloser) error
}

// InjectCache is a interface to inject a cache.
type InjectCache interface {
	InjectCache(c Cache) error
}

// InjectCacheInto injects a cache if the given object implements the InjectCache interface.
func InjectCacheInto(obj interface{}, cache Cache) error {
	if cache == nil {
		return nil
	}
	if injector, ok := obj.(InjectCache); ok {
		return injector.InjectCache(cache)
	}
	return nil
}

// Options contains all oci cache options to configure the oci cache.
type Options struct {
	// InMemoryOverlay specifies if a overlayFs InMemory cache should be used
	InMemoryOverlay bool

	// InMemoryGCConfig defines the garbage collection configuration for the in memory cache.
	InMemoryGCConfig GarbageCollectionConfiguration

	// BasePath specifies the Base path for the os filepath.
	// Will be defaulted to a temp filepath if not specified
	BasePath string

	// BaseGCConfig defines the garbage collection configuration for the in base cache.
	BaseGCConfig GarbageCollectionConfiguration
}

// Option is the interface to specify different cache options
type Option interface {
	ApplyOption(options *Options)
}

// ApplyOptions applies the given entries options on these options,
// and then returns itself (for convenient chaining).
func (o *Options) ApplyOptions(opts []Option) *Options {
	for _, opt := range opts {
		opt.ApplyOption(o)
	}
	return o
}

// ApplyDefaults sets defaults for the options.
func (o *Options) ApplyDefaults() {
	if o.InMemoryOverlay && len(o.InMemoryGCConfig.Size) == 0 {
		o.InMemoryGCConfig.Size = "200Mi"
	}
}

// WithInMemoryOverlay is the options to specify the usage of a in memory overlayFs
type WithInMemoryOverlay bool

func (w WithInMemoryOverlay) ApplyOption(options *Options) {
	options.InMemoryOverlay = bool(w)
}

// WithBasePath is the options to specify a base path
type WithBasePath string

func (p WithBasePath) ApplyOption(options *Options) {
	options.BasePath = string(p)
}

// WithInMemoryOverlaySize sets the max size of the overly file system.
// See the kubernetes quantity docs for detailed description of the format
// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/resource/quantity.go
type WithInMemoryOverlaySize string

func (p WithInMemoryOverlaySize) ApplyOption(options *Options) {
	options.InMemoryGCConfig.Size = string(p)
}

// WithBaseSize sets the max size of the base file system.
// See the kubernetes quantity docs for detailed description of the format
// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/resource/quantity.go
type WithBaseSize string

func (p WithBaseSize) ApplyOption(options *Options) {
	options.BaseGCConfig.Size = string(p)
}

// WithGCConfig overwrites the garbage collection settings for all caches.
type WithGCConfig GarbageCollectionConfiguration

func (p WithGCConfig) ApplyOption(options *Options) {
	cfg := GarbageCollectionConfiguration(p)
	cfg.Merge(&options.BaseGCConfig)
	cfg.Merge(&options.InMemoryGCConfig)
}

// WithBaseGCConfig overwrites the base garbage collection settings.
type WithBaseGCConfig GarbageCollectionConfiguration

func (p WithBaseGCConfig) ApplyOption(options *Options) {
	cfg := GarbageCollectionConfiguration(p)
	cfg.Merge(&options.BaseGCConfig)
}

// WithBaseGCConfig overwrites the in memory garbage collection settings.
type WithInMemoryGCConfig GarbageCollectionConfiguration

func (p WithInMemoryGCConfig) ApplyOption(options *Options) {
	cfg := GarbageCollectionConfiguration(p)
	cfg.Merge(&options.InMemoryGCConfig)
}

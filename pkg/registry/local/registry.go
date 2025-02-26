package localRegistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/ignitionstack/ignition/internal/repository"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
)

type localRegistry struct {
	dbRepo  repository.DBRepository
	storage registry.Storage
}

func NewLocalRegistry(rootDir string, dbRepo repository.DBRepository) registry.Registry {
	return &localRegistry{
		dbRepo:  dbRepo,
		storage: NewLocalStorage(rootDir),
	}
}

func (r *localRegistry) Get(namespace, name string) (*registry.FunctionMetadata, error) {
	var metadata *registry.FunctionMetadata

	err := r.withReadTx(func(txn *badger.Txn) error {
		return r.getFunctionMetadata(txn, namespace, name, &metadata)
	})

	return metadata, err
}

func (r *localRegistry) Pull(namespace, name, reference string) ([]byte, *registry.VersionInfo, error) {
	// Try to pull by digest first
	wasmBytes, versionInfo, digestErr := r.pullByDigest(namespace, name, reference)
	if digestErr == nil {
		return wasmBytes, versionInfo, nil
	}

	// If that fails, try by tag
	wasmBytes, versionInfo, tagErr := r.pullByTag(namespace, name, reference)
	if tagErr == nil {
		return wasmBytes, versionInfo, nil
	}

	// If both fail with specific errors, return a more helpful error
	if errors.Is(tagErr, registry.ErrTagNotFound) && errors.Is(digestErr, registry.ErrDigestNotFound) {
		return nil, nil, fmt.Errorf("%w: %s", registry.ErrInvalidReference, reference)
	}

	// Otherwise return the tag error
	return nil, nil, tagErr
}

func (r *localRegistry) Push(namespace, name string, payload []byte, fullDigest, tag string, settings manifest.FunctionVersionSettings) error {
	shortDigest := registry.TruncateDigest(fullDigest, 12)
	path := r.storage.BuildWASMPath(namespace, name, shortDigest)

	return r.withWriteTx(func(txn *badger.Txn) error {
		// Get or create function metadata
		metadata, err := r.getOrCreateMetadata(txn, namespace, name)
		if err != nil {
			return fmt.Errorf("failed to get metadata: %w", err)
		}

		// If a tag is provided, remove it from any existing versions
		if tag != "" {
			registry.RemoveTagFromVersions(&metadata.Versions, tag)
		}

		// Check if this version already exists
		versionExists := r.versionExists(metadata, shortDigest)

		// If the version doesn't exist, write the WASM file and create version info
		if !versionExists {
			if err := r.storage.WriteWASMFile(path, payload); err != nil {
				return fmt.Errorf("failed to write WASM file: %w", err)
			}
			newVersion := registry.CreateVersionInfo(shortDigest, fullDigest, payload, tag, settings)
			metadata.Versions = append(metadata.Versions, newVersion)
		} else if tag != "" {
			// If the version exists and a tag is provided, add the tag to the version
			registry.AddTagToVersion(&metadata.Versions, shortDigest, tag)
		}

		// Update the metadata in the database
		return r.updateMetadata(txn, namespace, name, metadata)
	})
}

func (r *localRegistry) ReassignTag(namespace, name, tag, newDigest string) error {
	return r.withWriteTx(func(txn *badger.Txn) error {
		// Get function metadata
		var metadata *registry.FunctionMetadata
		err := r.getFunctionMetadata(txn, namespace, name, &metadata)
		if err != nil {
			if errors.Is(err, registry.ErrFunctionNotFound) {
				return registry.ErrFunctionNotFound
			}
			return fmt.Errorf("failed to get metadata: %w", err)
		}

		// Function was found but metadata might be nil
		if metadata == nil {
			return registry.ErrFunctionNotFound
		}

		// Truncate digest for storage
		shortDigest := registry.TruncateDigest(newDigest, 12)

		// Ensure the target digest exists
		if !r.versionExists(metadata, shortDigest) {
			return registry.ErrDigestNotFound
		}

		// Update tags
		registry.RemoveTagFromVersions(&metadata.Versions, tag)
		registry.AddTagToVersion(&metadata.Versions, shortDigest, tag)

		// Update metadata in database
		return r.updateMetadata(txn, namespace, name, metadata)
	})
}

func (r *localRegistry) DigestExists(namespace, name, digest string) (bool, error) {
	var exists bool

	err := r.withReadTx(func(txn *badger.Txn) error {
		var metadata *registry.FunctionMetadata
		if err := r.getFunctionMetadata(txn, namespace, name, &metadata); err != nil {
			if err == registry.ErrFunctionNotFound {
				// If the function doesn't exist, the digest doesn't exist either
				exists = false
				return nil
			}
			return err
		}

		shortDigest := registry.TruncateDigest(digest, 12)
		exists = r.versionExists(metadata, shortDigest)
		return nil
	})

	return exists, err
}

func (r *localRegistry) ListAll() ([]registry.FunctionMetadata, error) {
	var functions []registry.FunctionMetadata

	err := r.withReadTx(func(txn *badger.Txn) error {
		// Set up the iterator
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		// Iterate through all keys with the "func:" prefix
		prefix := []byte("func:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			// Read and parse the value
			err := item.Value(func(val []byte) error {
				var metadata registry.FunctionMetadata
				if err := json.Unmarshal(val, &metadata); err != nil {
					return fmt.Errorf("failed to unmarshal metadata: %w", err)
				}
				functions = append(functions, metadata)
				return nil
			})

			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list functions: %w", err)
	}

	return functions, nil
}

func (r *localRegistry) withReadTx(fn func(txn *badger.Txn) error) error {
	return r.dbRepo.View(fn)
}

func (r *localRegistry) withWriteTx(fn func(txn *badger.Txn) error) error {
	return r.dbRepo.Update(fn)
}

// getFunctionMetadata retrieves a function's metadata from the database
func (r *localRegistry) getFunctionMetadata(txn *badger.Txn, namespace, name string, metadata **registry.FunctionMetadata) error {
	key := buildFunctionKey(namespace, name)

	// Try to get the item from the database
	item, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		return registry.ErrFunctionNotFound
	}
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	// Read and parse the value
	return item.Value(func(val []byte) error {
		*metadata = &registry.FunctionMetadata{}
		if err := json.Unmarshal(val, *metadata); err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		return nil
	})
}

// getOrCreateMetadata gets a function's metadata or creates it if it doesn't exist
func (r *localRegistry) getOrCreateMetadata(txn *badger.Txn, namespace, name string) (*registry.FunctionMetadata, error) {
	key := buildFunctionKey(namespace, name)

	// Try to get existing metadata
	item, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		// Create new metadata if it doesn't exist
		return &registry.FunctionMetadata{
			Namespace: namespace,
			Name:      name,
			CreatedAt: time.Now(),
			Versions:  make([]registry.VersionInfo, 0),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Read and parse existing metadata
	var metadata registry.FunctionMetadata
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &metadata)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

// updateMetadata writes updated metadata to the database
func (r *localRegistry) updateMetadata(txn *badger.Txn, namespace, name string, metadata *registry.FunctionMetadata) error {
	key := buildFunctionKey(namespace, name)

	// Update the timestamp
	metadata.UpdatedAt = time.Now()

	// Marshal the metadata to JSON
	val, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write to the database
	if err := txn.Set(key, val); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// versionExists checks if a version with the given digest exists
func (r *localRegistry) versionExists(metadata *registry.FunctionMetadata, shortDigest string) bool {
	for _, v := range metadata.Versions {
		if v.Hash == shortDigest {
			return true
		}
	}
	return false
}

// pullByDigest retrieves a function by its digest
func (r *localRegistry) pullByDigest(namespace, name, shortDigest string) ([]byte, *registry.VersionInfo, error) {
	// Build the path to the WASM file
	path := r.storage.BuildWASMPath(namespace, name, shortDigest)

	// Read the WASM file
	wasmBytes, err := r.storage.ReadWASMFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read WASM file: %w", err)
	}

	// Get version info from metadata
	var versionInfo *registry.VersionInfo

	err = r.withReadTx(func(txn *badger.Txn) error {
		var metadata *registry.FunctionMetadata
		if err := r.getFunctionMetadata(txn, namespace, name, &metadata); err != nil {
			return err
		}

		// Find the version with the matching digest
		for _, v := range metadata.Versions {
			if v.Hash == shortDigest {
				// Create a copy to avoid issues with the slice
				versionInfoCopy := v
				versionInfo = &versionInfoCopy
				return nil
			}
		}

		return registry.ErrDigestNotFound
	})

	if err != nil {
		return nil, nil, err
	}

	return wasmBytes, versionInfo, nil
}

// pullByTag retrieves a function by its tag
func (r *localRegistry) pullByTag(namespace, name, tag string) ([]byte, *registry.VersionInfo, error) {
	var wasmBytes []byte
	var versionInfo *registry.VersionInfo

	err := r.withReadTx(func(txn *badger.Txn) error {
		var metadata *registry.FunctionMetadata
		if err := r.getFunctionMetadata(txn, namespace, name, &metadata); err != nil {
			return err
		}

		// Find the version with the matching tag
		for _, v := range metadata.Versions {
			if registry.HasTag(v.Tags, tag) {
				// Create a copy to avoid issues with the slice
				versionInfoCopy := v
				versionInfo = &versionInfoCopy

				// Read the WASM file
				path := r.storage.BuildWASMPath(namespace, name, v.Hash)
				var err error
				wasmBytes, err = r.storage.ReadWASMFile(path)
				if err != nil {
					return fmt.Errorf("failed to read WASM file: %w", err)
				}

				return nil
			}
		}

		return registry.ErrTagNotFound
	})

	if err != nil {
		return nil, nil, err
	}

	return wasmBytes, versionInfo, nil
}

// buildFunctionKey creates a database key for a function
func buildFunctionKey(namespace, name string) []byte {
	return []byte(fmt.Sprintf("func:%s/%s", namespace, name))
}

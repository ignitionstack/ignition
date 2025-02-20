package localRegistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
)

type localRegistry struct {
	db      *badger.DB
	storage registry.Storage
}

func NewLocalRegistry(rootDir string, db *badger.DB) registry.Registry {
	return &localRegistry{
		db:      db,
		storage: NewLocalStorage(rootDir),
	}
}

func (r *localRegistry) getFunctionMetadata(txn *badger.Txn, namespace, name string, metadata **registry.FunctionMetadata) error {
	key := []byte(fmt.Sprintf("func:%s/%s", namespace, name))
	item, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		return registry.ErrFunctionNotFound
	}
	if err != nil {
		return err
	}

	return item.Value(func(val []byte) error {
		*metadata = &registry.FunctionMetadata{}
		return json.Unmarshal(val, *metadata)
	})
}

func (r *localRegistry) Get(namespace, name string) (*registry.FunctionMetadata, error) {
	var metadata *registry.FunctionMetadata
	err := r.db.View(func(txn *badger.Txn) error {
		return r.getFunctionMetadata(txn, namespace, name, &metadata)
	})
	return metadata, err
}

func (r *localRegistry) pullByDigest(namespace, name, shortDigest string) ([]byte, *registry.VersionInfo, error) {
	path := r.storage.BuildWASMPath(namespace, name, shortDigest)
	wasmBytes, err := r.storage.ReadWASMFile(path)
	if err != nil {
		return nil, nil, err
	}

	var versionInfo *registry.VersionInfo
	err = r.db.View(func(txn *badger.Txn) error {
		var metadata *registry.FunctionMetadata
		if err := r.getFunctionMetadata(txn, namespace, name, &metadata); err != nil {
			return err
		}

		for _, v := range metadata.Versions {
			if v.Hash == shortDigest {
				// Create a copy of the version info to avoid issues with the slice
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

func (r *localRegistry) pullByTag(namespace, name, tag string) ([]byte, *registry.VersionInfo, error) {
	var wasmBytes []byte
	var versionInfo *registry.VersionInfo

	err := r.db.View(func(txn *badger.Txn) error {
		var metadata *registry.FunctionMetadata
		if err := r.getFunctionMetadata(txn, namespace, name, &metadata); err != nil {
			return err
		}

		for _, v := range metadata.Versions {
			if registry.HasTag(v.Tags, tag) {
				// Create a copy of the version info to avoid issues with the slice
				versionInfoCopy := v
				versionInfo = &versionInfoCopy
				path := r.storage.BuildWASMPath(namespace, name, v.Hash)
				var err error
				wasmBytes, err = r.storage.ReadWASMFile(path)
				return err
			}
		}
		return registry.ErrTagNotFound
	})

	if err != nil {
		return nil, nil, err
	}

	return wasmBytes, versionInfo, nil
}

func (r *localRegistry) Pull(namespace, name, reference string) ([]byte, *registry.VersionInfo, error) {
	wasmBytes, versionInfo, err := r.pullByDigest(namespace, name, reference)
	if err == nil {
		return wasmBytes, versionInfo, nil
	}

	wasmBytes, versionInfo, tagErr := r.pullByTag(namespace, name, reference)
	if tagErr == nil {
		return wasmBytes, versionInfo, nil
	}

	if errors.Is(tagErr, registry.ErrTagNotFound) && errors.Is(err, registry.ErrDigestNotFound) {
		return nil, nil, fmt.Errorf("%w: %s", registry.ErrInvalidReference, reference)
	}

	return nil, nil, tagErr
}

func (r *localRegistry) getOrCreateMetadata(txn *badger.Txn, namespace, name string) (*registry.FunctionMetadata, error) {
	key := []byte(fmt.Sprintf("func:%s/%s", namespace, name))
	var metadata registry.FunctionMetadata

	item, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		return &registry.FunctionMetadata{
			Namespace: namespace,
			Name:      name,
			CreatedAt: time.Now(),
			Versions:  make([]registry.VersionInfo, 0),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &metadata)
	})
	return &metadata, err
}

func (r *localRegistry) versionExists(metadata *registry.FunctionMetadata, shortDigest string) bool {
	for _, v := range metadata.Versions {
		if v.Hash == shortDigest {
			return true
		}
	}
	return false
}

func (r *localRegistry) updateMetadata(txn *badger.Txn, namespace, name string, metadata *registry.FunctionMetadata) error {
	key := []byte(fmt.Sprintf("func:%s/%s", namespace, name))
	metadata.UpdatedAt = time.Now()

	val, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	return txn.Set(key, val)
}

func (r *localRegistry) Push(namespace, name string, payload []byte, fullDigest, tag string, settings manifest.FunctionVersionSettings) error {
	shortDigest := registry.TruncateDigest(fullDigest, 12)
	path := r.storage.BuildWASMPath(namespace, name, shortDigest)

	return r.db.Update(func(txn *badger.Txn) error {
		metadata, err := r.getOrCreateMetadata(txn, namespace, name)
		if err != nil {
			return err
		}

		if tag != "" {
			registry.RemoveTagFromVersions(&metadata.Versions, tag)
		}

		if !r.versionExists(metadata, shortDigest) {
			if err := r.storage.WriteWASMFile(path, payload); err != nil {
				return err
			}
			metadata.Versions = append(metadata.Versions, registry.CreateVersionInfo(shortDigest, fullDigest, payload, tag, settings))
		} else if tag != "" {
			registry.AddTagToVersion(&metadata.Versions, shortDigest, tag)
		}

		return r.updateMetadata(txn, namespace, name, metadata)
	})
}

func (r *localRegistry) ReassignTag(namespace, name, tag, newDigest string) error {
	return r.db.Update(func(txn *badger.Txn) error {
		metadata, err := r.getOrCreateMetadata(txn, namespace, name)
		if err != nil {
			return err
		}

		shortDigest := registry.TruncateDigest(newDigest, 12)
		if !r.versionExists(metadata, shortDigest) {
			return registry.ErrDigestNotFound
		}

		registry.RemoveTagFromVersions(&metadata.Versions, tag)
		registry.AddTagToVersion(&metadata.Versions, shortDigest, tag)

		return r.updateMetadata(txn, namespace, name, metadata)
	})
}

func (r *localRegistry) DigestExists(namespace, name, digest string) (bool, error) {
	var exists bool
	err := r.db.View(func(txn *badger.Txn) error {
		var metadata *registry.FunctionMetadata
		if err := r.getFunctionMetadata(txn, namespace, name, &metadata); err != nil {
			if err == registry.ErrFunctionNotFound {
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

	err := r.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("func:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var metadata registry.FunctionMetadata
				if err := json.Unmarshal(val, &metadata); err != nil {
					return err
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

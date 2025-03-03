package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/ignitionstack/ignition/pkg/engine/errors"
)

// Repository defines a generic repository interface for storing and retrieving entities.
type Repository[T any] interface {
	// Get retrieves an entity by ID
	Get(ctx context.Context, id string) (T, error)

	// GetAll retrieves all entities
	GetAll(ctx context.Context) ([]T, error)

	// Save stores an entity
	Save(ctx context.Context, id string, entity T) error

	// Delete removes an entity by ID
	Delete(ctx context.Context, id string) error

	// Exists checks if an entity exists
	Exists(ctx context.Context, id string) (bool, error)
}

// InMemoryRepository is a simple in-memory implementation of Repository.
// This is useful for testing and for simple use cases
type InMemoryRepository[T any] struct {
	data  map[string]T
	mutex sync.RWMutex
}

// NewInMemoryRepository creates a new in-memory repository.
func NewInMemoryRepository[T any]() *InMemoryRepository[T] {
	return &InMemoryRepository[T]{
		data: make(map[string]T),
	}
}

// Get retrieves an entity by ID
func (r *InMemoryRepository[T]) Get(ctx context.Context, id string) (T, error) {
	var zero T

	// Check context cancellation
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	default:
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	entity, exists := r.data[id]
	if !exists {
		return zero, errors.Wrap(errors.DomainRegistry, errors.CodeVersionNotFound,
			fmt.Sprintf("Entity with ID %s not found", id), nil)
	}

	return entity, nil
}

// GetAll retrieves all entities
func (r *InMemoryRepository[T]) GetAll(ctx context.Context) ([]T, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	entities := make([]T, 0, len(r.data))
	for _, entity := range r.data {
		entities = append(entities, entity)
	}

	return entities, nil
}

// Save stores an entity
func (r *InMemoryRepository[T]) Save(ctx context.Context, id string, entity T) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.data[id] = entity
	return nil
}

// Delete removes an entity by ID
func (r *InMemoryRepository[T]) Delete(ctx context.Context, id string) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	_, exists := r.data[id]
	if !exists {
		return errors.Wrap(errors.DomainRegistry, errors.CodeVersionNotFound,
			fmt.Sprintf("Entity with ID %s not found", id), nil)
	}

	delete(r.data, id)
	return nil
}

// Exists checks if an entity exists
func (r *InMemoryRepository[T]) Exists(ctx context.Context, id string) (bool, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, exists := r.data[id]
	return exists, nil
}

// Clear removes all entities
func (r *InMemoryRepository[T]) Clear() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.data = make(map[string]T)
}

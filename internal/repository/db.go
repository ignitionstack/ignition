package repository

import "github.com/dgraph-io/badger/v4"

// DBRepository defines the interface for database operations
type DBRepository interface {
	// View executes a read-only transaction
	View(fn func(txn *badger.Txn) error) error
	
	// Update executes a read-write transaction
	Update(fn func(txn *badger.Txn) error) error
	
	// Close closes the database connection
	Close() error
}

// BadgerDBRepository implements DBRepository with BadgerDB
type BadgerDBRepository struct {
	db *badger.DB
}

// NewBadgerDBRepository creates a new BadgerDBRepository
func NewBadgerDBRepository(db *badger.DB) DBRepository {
	return &BadgerDBRepository{db: db}
}

// View implements DBRepository.View
func (r *BadgerDBRepository) View(fn func(txn *badger.Txn) error) error {
	return r.db.View(fn)
}

// Update implements DBRepository.Update
func (r *BadgerDBRepository) Update(fn func(txn *badger.Txn) error) error {
	return r.db.Update(fn)
}

// Close implements DBRepository.Close
func (r *BadgerDBRepository) Close() error {
	return r.db.Close()
}
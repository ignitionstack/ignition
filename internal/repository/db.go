package repository

import "github.com/dgraph-io/badger/v4"

type DBRepository interface {
	View(fn func(txn *badger.Txn) error) error
	Update(fn func(txn *badger.Txn) error) error
	Close() error
}

type BadgerDBRepository struct {
	db *badger.DB
}

func NewBadgerDBRepository(db *badger.DB) DBRepository {
	return &BadgerDBRepository{db: db}
}

func (r *BadgerDBRepository) View(fn func(txn *badger.Txn) error) error {
	return r.db.View(fn)
}

func (r *BadgerDBRepository) Update(fn func(txn *badger.Txn) error) error {
	return r.db.Update(fn)
}

func (r *BadgerDBRepository) Close() error {
	return r.db.Close()
}

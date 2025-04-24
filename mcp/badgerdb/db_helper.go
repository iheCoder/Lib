package main

import (
	"github.com/dgraph-io/badger/v4"
	"strings"
)

type Database struct {
	db *badger.DB
}

func NewDatabase() (*Database, error) {
	// Open the Badger database
	opts := badger.DefaultOptions("./data").WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &Database{db: db}, nil
}

func (d *Database) Add(key string, value string) error {
	return d.db.Update(func(txn *badger.Txn) error {
		err := txn.Set([]byte(key), []byte(value))
		return err
	})
}

func (d *Database) Get(key string) (string, error) {
	var valCopy []byte
	err := d.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		valCopy, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return "", err
	}

	return string(valCopy), nil
}

func (d *Database) Update(key string, newValue string) error {
	return d.Add(key, newValue)
}

// ListKeysAndValues lists all key-value pairs in the current database.
func (d *Database) ListKeysAndValues() (map[string]string, error) {
	results := make(map[string]string)
	err := d.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			results[string(k)] = string(v)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (d *Database) Delete(key string) error {
	return d.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(key))
		return err
	})
}

// SearchKeysAndValuesWithFilter searches key-value pairs containing the keyword, with offset and limit for pagination.
func (d *Database) SearchKeysAndValuesWithFilter(keyword string, offset int, limit int) (map[string]string, error) {
	results := make(map[string]string)
	err := d.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		count := 0
		skipped := 0
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			if !strings.Contains(string(k), keyword) {
				continue
			}

			if skipped < offset {
				skipped++
				continue
			}
			if count >= limit {
				break
			}

			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			results[string(k)] = string(v)
			count++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}

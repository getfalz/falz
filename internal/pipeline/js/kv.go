package js

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"go.etcd.io/bbolt"
)

type KVStore struct {
	db *bbolt.DB
	mu sync.RWMutex
}

func NewKVStore(path string) (*KVStore, error) {
	db, err := bbolt.Open(path, 0666, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	return &KVStore{db: db}, nil
}

func (kv *KVStore) GetModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()

	module.Set("use", func(bucketPath string) interface{} {
		return map[string]interface{}{
			"put": func(key, value string) *goja.Promise {
				return kv.put(vm, bucketPath, key, value)
			},
			"get": func(key string) *goja.Promise {
				return kv.get(vm, bucketPath, key)
			},
			"delete": func(key string) *goja.Promise {
				return kv.delete(vm, bucketPath, key)
			},
		}
	})

	return module
}

func (kv *KVStore) put(vm *goja.Runtime, bucketPath, key, value string) *goja.Promise {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		kv.mu.Lock()
		defer kv.mu.Unlock()

		err := kv.db.Update(func(tx *bbolt.Tx) error {
			bucket, err := kv.getOrCreateBucket(tx, bucketPath)
			if err != nil {
				return err
			}
			return bucket.Put([]byte(key), []byte(value))
		})

		if err != nil {
			reject(err)
		} else {
			resolve(goja.Undefined())
		}
	}()

	return promise
}

func (kv *KVStore) get(vm *goja.Runtime, bucketPath, key string) *goja.Promise {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		kv.mu.RLock()
		defer kv.mu.RUnlock()

		var value string
		err := kv.db.View(func(tx *bbolt.Tx) error {
			bucket := kv.getBucket(tx, bucketPath)
			if bucket == nil {
				return errors.New("bucket not found")
			}
			val := bucket.Get([]byte(key))
			if val == nil {
				return errors.New("key not found")
			}
			value = string(val)
			return nil
		})
		if err != nil {
			reject(err)
		} else {
			resolve(value)
		}
	}()

	return promise
}

func (kv *KVStore) delete(vm *goja.Runtime, bucketPath, key string) *goja.Promise {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		kv.mu.Lock()
		defer kv.mu.Unlock()

		err := kv.db.Update(func(tx *bbolt.Tx) error {
			bucket := kv.getBucket(tx, bucketPath)
			if bucket == nil {
				return fmt.Errorf("bucket '%s' not found", bucketPath)
			}
			return bucket.Delete([]byte(key))
		})

		if err != nil {
			reject(err)
		} else {
			resolve(goja.Undefined())
		}
	}()

	return promise
}

func (kv *KVStore) getOrCreateBucket(tx *bbolt.Tx, bucketPath string) (*bbolt.Bucket, error) {
	parts := strings.Split(bucketPath, ".")
	var bucket *bbolt.Bucket

	for i, part := range parts {
		if i == 0 {
			bucket = tx.Bucket([]byte(part))
			if bucket == nil {
				newBucket, err := tx.CreateBucket([]byte(part))
				if err != nil {
					return nil, err
				}
				bucket = newBucket
			}
		} else {
			subBucket := bucket.Bucket([]byte(part))
			if subBucket == nil {
				newSubBucket, err := bucket.CreateBucket([]byte(part))
				if err != nil {
					return nil, err
				}
				bucket = newSubBucket
			} else {
				bucket = subBucket
			}
		}
	}

	return bucket, nil
}

func (kv *KVStore) getBucket(tx *bbolt.Tx, bucketPath string) *bbolt.Bucket {
	parts := strings.Split(bucketPath, ".")
	bucket := tx.Bucket([]byte(parts[0]))
	for _, part := range parts[1:] {
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte(part))
	}
	return bucket
}

func (kv *KVStore) Close() error {
	return kv.db.Close()
}

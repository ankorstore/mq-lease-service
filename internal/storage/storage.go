package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/rs/zerolog/log"
)

// max age we are storing anything in our K/V DB.
// prevent any leaked objects and keeping track of lease requests
// we can improve the code in the future!
const maxAge = 7 * 24 * time.Hour

type object interface {
	// GetIdentifier returns key used in the K/V storage
	GetIdentifier() string
	// Marshal marshals the object to store in a byte slice representation
	Marshal() ([]byte, error)
	// Unmarshal unmarshals the byte slice representation of the object in storage to its native type
	Unmarshal([]byte) error
}

type badgerLogger struct {
	ctx context.Context
}

// newBadgerLogger creates a new logger for badger
func newBadgerLogger(ctx context.Context) badger.Logger {
	return &badgerLogger{
		ctx: ctx,
	}
}

func (l *badgerLogger) Errorf(format string, v ...interface{}) {
	log.Ctx(l.ctx).Error().Msgf(format, v...)
}

func (l *badgerLogger) Warningf(format string, v ...interface{}) {
	log.Ctx(l.ctx).Warn().Msgf(format, v...)
}

func (l *badgerLogger) Infof(format string, v ...interface{}) {
	log.Ctx(l.ctx).Info().Msgf(format, v...)
}

func (l *badgerLogger) Debugf(format string, v ...interface{}) {
	log.Ctx(l.ctx).Debug().Msgf(format, v...)
}

type Storage[T object] interface {
	// Init initialises the storage (opens it)
	Init() error
	// Close gracefully terminates the storage.
	Close() error
	// Hydrate hydrates the provided object with data coming from the storage
	// the provided object should at least be able to return a non-null and unique Identifier (via the GetIdentifier() method)
	Hydrate(ctx context.Context, defaultObj T) error
	// Save store the provided object in the storage
	// the provided object should at least be able to return a non-null and unique Identifier (via the GetIdentifier() method)
	Save(ctx context.Context, obj T) error
	// HealthCheck verifies if the storage is connected and usable
	HealthCheck(ctx context.Context, hydrationSample func() T) bool
}

type storageImpl[T object] struct {
	options badger.Options
	db      *badger.DB
	setup   sync.Once
}

// New returns an instance of the storage (it doesn't open it)
func New[T object](ctx context.Context, persistentStateDir string) Storage[T] {
	options := badger.DefaultOptions(persistentStateDir)
	options.Logger = newBadgerLogger(ctx)

	return &storageImpl[T]{options: options}
}

// Init initialises the storage (opens it)
func (s *storageImpl[T]) Init() error {
	var err error
	s.setup.Do(func() {
		s.db, err = badger.Open(s.options)
		if err != nil {
			err = fmt.Errorf("failed to open badger connection: %w", err)
		}
	})
	return err
}

// Close gracefully terminates the storage.
func (s *storageImpl[T]) Close() error {
	err := s.db.Close()
	if err != nil {
		return fmt.Errorf("failed to close badger connection: %w", err)
	}
	return nil
}

// Hydrate hydrates the provided object with data coming from the storage
// the provided object should at least be able to return a non-null and unique Identifier (via the GetIdentifier() method)
func (s *storageImpl[T]) Hydrate(ctx context.Context, defaultObj T) error {
	var err error

	id := defaultObj.GetIdentifier()

	txn := s.db.NewTransaction(false)
	defer func(ctx context.Context) {
		nestedErr := txn.Commit()
		if nestedErr != nil {
			log.Ctx(ctx).Error().Err(nestedErr).Msg("Failed to commit read transaction")
		}
	}(ctx)

	res, err := txn.Get([]byte(id))
	if err == badger.ErrKeyNotFound {
		log.Ctx(ctx).Debug().Msg("Not found, passing default object")
		return nil
	}
	if err != nil && err != badger.ErrKeyNotFound {
		log.Ctx(ctx).Warn().Err(err).Msg("Internal Badger error")
		return err
	}

	return res.Value(func(val []byte) error {
		return defaultObj.Unmarshal(val)
	})
}

// Save store the provided object in the storage
// the provided object should at least be able to return a non-null and unique Identifier (via the GetIdentifier() method)
func (s *storageImpl[T]) Save(_ context.Context, obj T) error {
	var err error
	id := obj.GetIdentifier()
	b, err := obj.Marshal()
	if err != nil {
		return err
	}

	txn := s.db.NewTransaction(true)
	entry := badger.NewEntry([]byte(id), b).WithTTL(maxAge)
	err = txn.SetEntry(entry)
	if err != nil {
		txn.Discard()
		return err
	}
	return txn.Commit()
}

// HealthCheck verifies if the storage is connected and usable
func (s *storageImpl[T]) HealthCheck(ctx context.Context, hydrationSample func() T) bool {
	if s.db == nil {
		log.Ctx(ctx).Error().Msg("Storage healthcheck failed: db is nil")
		return false
	}
	if s.db.IsClosed() {
		log.Ctx(ctx).Error().Msg("Storage healthcheck failed: db is closed")
		return false
	}
	if err := s.Hydrate(ctx, hydrationSample()); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Storage healthcheck failed: could not hydrate sample")
		return false
	}

	return true
}

// NullStorage is a dummy object honoring the Storage interface, and can be used in unit tests
// as a drop-in replacement in the dependencies if the test don't actually care about storage actions.
type NullStorage[T object] struct{}

func (s NullStorage[T]) Init() error {
	return nil
}

func (s NullStorage[T]) Close() error {
	return nil
}

func (s NullStorage[T]) Hydrate(context.Context, T) error {
	return nil
}

func (s NullStorage[T]) Save(context.Context, T) error {
	return nil
}

func (s NullStorage[T]) HealthCheck(context.Context, func() T) bool {
	return true
}

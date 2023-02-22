package storage

import (
	"context"
	"os"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/storage"
)

type Helper struct {
	baseDir string
}

// NewHelper will create a dedicated instance of the helper, and will create alongside of it a base temporary folder
// in the FS to host any further storage DBs
func NewHelper() *Helper {
	baseDir, err := os.MkdirTemp("", "e2e-gen-storage-")
	if err != nil {
		panic(err)
	}
	return &Helper{baseDir: baseDir}
}

// NewStorageDir will create a dedicated folder in the temporary global helper directory to host a siloed DB files
func (h *Helper) NewStorageDir() string {
	dir, err := os.MkdirTemp(h.baseDir, "state-")
	if err != nil {
		panic(err)
	}
	return dir
}

// PrefillStorage will preload the DB with a given Provider state (useful to start a test from a know state of the system)
func (h *Helper) PrefillStorage(storageDir string, leaseProviderState *lease.ProviderState) {
	st := storage.New[*lease.ProviderState](context.Background(), storageDir)
	if err := st.Init(); err != nil {
		panic(err)
	}

	defer func() {
		if err := st.Close(); err != nil {
			panic(err)
		}
	}()

	if err := st.Save(context.Background(), leaseProviderState); err != nil {
		panic(err)
	}
}

// Cleanup will drop the storage DBs & temporary folders
func (h *Helper) Cleanup() {
	_ = os.RemoveAll(h.baseDir)
}

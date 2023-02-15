package lease

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
)

func TestLeaseRequest_UpdateLastSeenAt_Nil(t *testing.T) {
	now := time.Now()

	req := &LeaseRequest{}
	req.UpdateLastSeenAt()
	assert.NotNil(t, req.lastSeenAt)
	assert.True(t, now.Before(*req.lastSeenAt))
}

func TestLeaseRequest_UpdateLastSeenAt_Update(t *testing.T) {
	now := time.Now()
	req := &LeaseRequest{
		lastSeenAt: &now,
	}
	// Ensure there's a time shift
	time.Sleep(10 * time.Millisecond)
	req.UpdateLastSeenAt()
	assert.NotNil(t, req.lastSeenAt)
	assert.True(t, now.Before(*req.lastSeenAt))
}

func TestNewLeaseProvider(t *testing.T) {
	creationTime := time.Now()
	opts := &LeaseProviderOpts{
		StabilizeDuration:    time.Minute,
		TTL:                  time.Second,
		ExpectedRequestCount: 4,
	}

	leaseProvider := NewLeaseProvider(opts)
	leaseProviderImpl, ok := leaseProvider.(*leaseProviderImpl)
	assert.True(t, ok)
	assert.Equal(t, opts, leaseProviderImpl.opts)
	assert.NotNil(t, leaseProviderImpl.known)
	assert.Nil(t, leaseProviderImpl.aquired)
	assert.True(t, leaseProviderImpl.lastUpdatedAt.After(creationTime))
}

func Test_leaseProviderImpl_insert_update_ok(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	// Empty at startup
	assert.Equal(t, 0, len(lpImpl.known))

	// Register first one
	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 10,
	}
	lpImpl.insert(req1)
	assert.Equal(t, 1, len(lpImpl.known))
	assert.Equal(t, lpImpl.known[req1.HeadSHA], req1)

	// Register 2nd
	req2 := &LeaseRequest{
		HeadSHA: "sha2",
	}
	lpImpl.insert(req2)
	assert.Equal(t, 2, len(lpImpl.known))
	assert.Equal(t, lpImpl.known[req2.HeadSHA], req2)

	// OVerride 1st request
	req1Update := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1000,
	}
	lpImpl.insert(req1Update)
	assert.Equal(t, 2, len(lpImpl.known))
	assert.Equal(t, lpImpl.known[req1Update.HeadSHA], req1)
	assert.Equal(t, lpImpl.known[req1Update.HeadSHA].Priority, req1Update.Priority)
}

func Test_leaseProviderImpl_insert_invalid_state_new(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	for _, state := range []string{STATUS_AQUIRED, STATUS_COMPLETED, STATUS_FAILURE, STATUS_SUCCESS} {
		req := &LeaseRequest{
			HeadSHA:  "sha1",
			Priority: 10,
			Status:   pointer.String(state),
		}
		_, err := lpImpl.insert(req)
		assert.Error(t, err)
	}
}

func Test_leaseProviderImpl_insert_valid_state_transition(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 10,
	}
	_, err := lpImpl.insert(req)
	assert.NoError(t, err)

	for _, status := range []string{STATUS_FAILURE, STATUS_SUCCESS, STATUS_AQUIRED} {
		// Manually set aquired state. It's a pointer -> it's auto updated in the state
		req.Status = pointer.String(STATUS_AQUIRED)
		lpImpl.aquired = req

		updateReq := &LeaseRequest{
			HeadSHA:  "sha1",
			Priority: 10,
			Status:   pointer.String(status),
		}
		_, err = lpImpl.insert(updateReq)
		assert.NoError(t, err)
	}
}

func Test_leaseProviderImpl_insert_invalid_state_transition(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 10,
	}
	_, err := lpImpl.insert(req)
	assert.NoError(t, err)

	for _, previousStatus := range []string{STATUS_PENDING, STATUS_COMPLETED, STATUS_FAILURE, STATUS_SUCCESS} {
		for _, status := range []string{STATUS_FAILURE, STATUS_SUCCESS, STATUS_AQUIRED} {
			// Manually set previous state. It's a pointer -> it's auto updated in the state
			req.Status = pointer.String(previousStatus)

			updateReq := &LeaseRequest{
				HeadSHA:  "sha1",
				Priority: 10,
				Status:   pointer.String(status),
			}
			_, err = lpImpl.insert(updateReq)
			if previousStatus == status {
				assert.NoErrorf(t, err, "previous: %s, new: %s", previousStatus, status)
			} else {
				assert.Errorf(t, err, "previous: %s, new: %s", previousStatus, status)
			}
		}
	}
}

func Test_leaseProviderImpl_evictTTL(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 10,
	}

	// Insert one request
	_, err := lpImpl.insert(req1)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(lpImpl.known))
	lpImpl.evictTTL()

	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 10,
	}
	_, err = lpImpl.insert(req2)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(lpImpl.known))

	aheadOfTime := time.Now().Add(-100 * time.Second)
	req1.lastSeenAt = &aheadOfTime
	lpImpl.evictTTL() // <-- eviction should evict older entries now
	assert.Equal(t, 1, len(lpImpl.known))
}

func Test_leaseProviderImpl_evictTTL_aquired(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 10,
	}

	// Insert one request
	_, err := lpImpl.insert(req)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(lpImpl.known))

	aheadOfTime := time.Now().Add(-100 * time.Second)
	req.lastSeenAt = &aheadOfTime
	req.Status = pointer.String(STATUS_AQUIRED)
	lpImpl.aquired = req

	// Despite being outdated, this key should not be evicted!
	lpImpl.evictTTL()
	assert.Equal(t, 1, len(lpImpl.known))
}

func Test_leaseProviderImpl_isWinningLeaseRequest_timePassed(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 4})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}

	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	_, err := lpImpl.insert(req1)
	assert.NoError(t, err)
	_, err = lpImpl.insert(req2)
	assert.NoError(t, err)

	// Immediately check if req2 gets the lease (it should not!)
	_ = lpImpl.isWinningLeaseRequest(req2)
	assert.Equal(t, *req2.Status, STATUS_PENDING)

	// Simulate a time passed by setting the last updated timestamp in the past
	lpImpl.lastUpdatedAt = time.Now().Add(-2 * time.Minute)
	_ = lpImpl.isWinningLeaseRequest(req2)
	assert.Equal(t, *req2.Status, STATUS_AQUIRED)
}

func Test_leaseProviderImpl_isWinningLeaseRequest_reachedExpectedRequestCount(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 3})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 1000,
	}
	req3 := &LeaseRequest{
		HeadSHA:  "sha3",
		Priority: 100,
	}

	// Inject the two requests
	_, err := lpImpl.insert(req1)
	assert.NoError(t, err)
	_, err = lpImpl.insert(req2)
	assert.NoError(t, err)

	// Immediately check if req2 gets the lease (it should not!)
	_ = lpImpl.isWinningLeaseRequest(req2)
	assert.Equal(t, *req2.Status, STATUS_PENDING)

	// Add 3rd request, making it complete (it has a lower priority compared to req2)
	_, err = lpImpl.insert(req3)
	assert.NoError(t, err)
	_ = lpImpl.isWinningLeaseRequest(req2)
	assert.Equal(t, *req2.Status, STATUS_AQUIRED)
}

func Test_leaseProviderImpl_isWinningLeaseRequest_errorNoLeaseAssigned(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 3})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}

	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	_, err := lpImpl.insert(req1)
	assert.NoError(t, err)
	_, err = lpImpl.insert(req2)
	assert.NoError(t, err)

	// Set req2 to be the aquired lesae
	req2.Status = pointer.String(STATUS_AQUIRED)
	lpImpl.aquired = req2

	// Make sure there's no status modification when checking if a lease is the winner
	req1copy := lpImpl.isWinningLeaseRequest(&LeaseRequest{
		HeadSHA:  req1.HeadSHA,
		Priority: req1.Priority,
		Status:   pointer.String(STATUS_PENDING),
	})
	// It should just mirror the request when the state is aquired
	assert.Equal(t, req1copy, req1copy)

}

func Test_leaseProviderImpl__FullLoop_ReleaseSuccess(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 3})
	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 2,
	}
	req3 := &LeaseRequest{
		HeadSHA:  "sha3",
		Priority: 3,
	}
	req3success := &LeaseRequest{
		HeadSHA:  "sha3",
		Priority: 3,
		Status:   pointer.String(STATUS_SUCCESS),
	}

	reqNext := &LeaseRequest{
		HeadSHA:  "next",
		Priority: 1,
	}

	req1, err := lp.Aquire(req1)
	assert.NoError(t, err)

	req3, err = lp.Aquire(req3)
	assert.NoError(t, err)

	// Add last remaining request. The system has full knowledge now but req2 is not the winner -> should have the status pending
	req2, err = lp.Aquire(req2)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req2.Status)

	// Check for req3, the winner
	req3, err = lp.Aquire(req3)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_AQUIRED, *req3.Status)

	// The reqNext will now be rejected, since the lease aquiring is locked and we're awaiting all other leases to return
	_, err = lp.Aquire(reqNext)
	assert.Error(t, err)

	// Report success status for req3
	req3, err = lp.Release(req3success)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_COMPLETED, *req3.Status)

	// Now, all other LeaseRequests will get the status COMPLETED assigned -> the process can die
	req1, err = lp.Aquire(req1)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_COMPLETED, *req1.Status)

	// The reqNext should still fail as confirmation or timeout of req2 is awaited
	_, err = lp.Aquire(reqNext)
	assert.Error(t, err)

	// Last remaining request, send COMPLETE and afterwards the next distributed lease can start
	req2, err = lp.Aquire(req2)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_COMPLETED, *req2.Status)

	// Now, all leases are marked as successful / released. the reqNew should now be accepted
	_, err = lp.Aquire(reqNext)
	assert.NoError(t, err)

}

func Test_leaseProviderImpl__FullLoop_ReleaseFailedNoNewRequest(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 3})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 2,
	}
	req3 := &LeaseRequest{
		HeadSHA:  "sha3",
		Priority: 3,
	}
	req3failure := &LeaseRequest{
		HeadSHA:  "sha3",
		Priority: 3,
		Status:   pointer.String(STATUS_FAILURE),
	}

	req1, err := lp.Aquire(req1)
	assert.NoError(t, err)

	req3, err = lp.Aquire(req3)
	assert.NoError(t, err)

	// Add last remaining request. The system has full knowledge now but req2 is not the winner -> should have the status pending
	req2, err = lp.Aquire(req2)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req2.Status)

	// Check for req3, the winner
	req3, err = lp.Aquire(req3)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_AQUIRED, *req3.Status)

	// Report failure status for req3
	req3, err = lp.Release(req3failure)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_FAILURE, *req3.Status)

	// The lease is released, but we did not have a successful outcome -> pass it to the next one waiting. It's not req1
	req1, err = lp.Aquire(req1)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req1.Status)

	// req2 has the highest priority -> it gets the lease (assuming sufficient time passed)
	// (note: backdate the stabilisation duration)
	lpImpl.lastUpdatedAt = time.Now().Add(-100 * time.Minute)
	req2, err = lp.Aquire(req2)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_AQUIRED, *req2.Status)
}

func Test_leaseProviderImpl__FullLoop_ReleaseFailedNewRequest(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 3})

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 2,
	}
	req3 := &LeaseRequest{
		HeadSHA:  "sha3",
		Priority: 3,
	}
	req3failure := &LeaseRequest{
		HeadSHA:  "sha3",
		Priority: 3,
		Status:   pointer.String(STATUS_FAILURE),
	}
	reqNext := &LeaseRequest{
		HeadSHA:  "next",
		Priority: 4,
	}

	req1, err := lp.Aquire(req1)
	assert.NoError(t, err)

	req3, err = lp.Aquire(req3)
	assert.NoError(t, err)

	// Add last remaining request. The system has full knowledge now but req2 is not the winner -> should have the status pending
	req2, err = lp.Aquire(req2)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req2.Status)

	// Check for req3, the winner
	req3, err = lp.Aquire(req3)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_AQUIRED, *req3.Status)

	// Report failure status for req3
	req3, err = lp.Release(req3failure)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_FAILURE, *req3.Status)

	// The lease is released, but we did not have a successful outcome -> pass it to the next one waiting. It's not req1
	req1, err = lp.Aquire(req1)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req1.Status)

	// A new request is coming in. Since there's no decision on who aquired the lease yet, it can still enter.
	// With it, we have full knowledge again and it gets the lease.
	reqNext, err = lp.Aquire(reqNext)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_AQUIRED, *reqNext.Status)
}

func Test_leaseProviderImpl__FullLoop_ReleaseWithNoAquiredLease(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 3})

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	req1, err := lp.Aquire(req1)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req1.Status)
	req2, err = lp.Aquire(req2)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req2.Status)

	// Try to release. There should be no lease, thus error
	_, err = lp.Release(&LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
		Status:   pointer.String(STATUS_SUCCESS),
	})
	assert.Error(t, err)
}

func Test_leaseProviderImpl__FullLoop_ReleaseFromInvalidHeadSHA(t *testing.T) {
	lp := NewLeaseProvider(&LeaseProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 2})

	req1 := &LeaseRequest{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &LeaseRequest{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	req1, err := lp.Aquire(req1)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_PENDING, *req1.Status)
	req2, err = lp.Aquire(req2)
	assert.NoError(t, err)
	assert.Equal(t, STATUS_AQUIRED, *req2.Status)

	// Try to release. There should be no lease, thus error
	_, err = lp.Release(&LeaseRequest{
		HeadSHA:  "thisdoesnotexist",
		Priority: 1,
		Status:   pointer.String(STATUS_SUCCESS),
	})
	assert.Error(t, err)
}

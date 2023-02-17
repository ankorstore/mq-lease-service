package lease

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
)

func TestLeaseRequest_UpdateLastSeenAt_Nil(t *testing.T) {
	now := time.Now()

	req := &Request{}
	req.UpdateLastSeenAt()
	assert.NotNil(t, req.lastSeenAt)
	assert.True(t, now.Before(*req.lastSeenAt))
}

func TestLeaseRequest_UpdateLastSeenAt_Update(t *testing.T) {
	now := time.Now()
	req := &Request{
		lastSeenAt: &now,
	}
	// Ensure there's a time shift
	time.Sleep(10 * time.Millisecond)
	req.UpdateLastSeenAt()
	assert.NotNil(t, req.lastSeenAt)
	assert.True(t, now.Before(*req.lastSeenAt))
}

func TestNewLeaseProvider(t *testing.T) {
	opts := ProviderOpts{
		StabilizeDuration:    time.Minute,
		TTL:                  time.Second,
		ExpectedRequestCount: 4,
	}

	leaseProvider := NewLeaseProvider(opts)
	leaseProviderImpl, ok := leaseProvider.(*leaseProviderImpl)
	assert.True(t, ok)
	assert.Equal(t, opts, leaseProviderImpl.opts)
	assert.NotNil(t, leaseProviderImpl.known)
	assert.Nil(t, leaseProviderImpl.acquired)
	assert.Nil(t, leaseProviderImpl.lastUpdatedAt)
}

func Test_leaseProviderImpl_insert_update_ok(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	// Empty at startup
	assert.Equal(t, 0, len(lpImpl.known))

	// Register first one
	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 10,
	}
	_, err := lpImpl.insert(context.Background(), req1)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(lpImpl.known))
	assert.Equal(t, lpImpl.known[req1.HeadSHA], req1)

	// Register 2nd
	req2 := &Request{
		HeadSHA: "sha2",
	}
	_, err = lpImpl.insert(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(lpImpl.known))
	assert.Equal(t, lpImpl.known[req2.HeadSHA], req2)

	// OOverride 1st request
	req1Update := &Request{
		HeadSHA:  "sha1",
		Priority: 1000,
	}
	_, err = lpImpl.insert(context.Background(), req1Update)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(lpImpl.known))
	assert.Equal(t, lpImpl.known[req1Update.HeadSHA], req1)
	assert.Equal(t, lpImpl.known[req1Update.HeadSHA].Priority, req1Update.Priority)
}

func Test_leaseProviderImpl_insert_invalid_state_new(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	for _, state := range []string{StatusAcquired, StatusCompleted, StatusFailure, StatusSuccess} {
		req := &Request{
			HeadSHA:  "sha1",
			Priority: 10,
			Status:   pointer.String(state),
		}
		_, err := lpImpl.insert(context.Background(), req)
		assert.Error(t, err)
	}
}

func Test_leaseProviderImpl_insert_valid_state_transition(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req := &Request{
		HeadSHA:  "sha1",
		Priority: 10,
	}
	_, err := lpImpl.insert(context.Background(), req)
	assert.NoError(t, err)

	for _, status := range []string{StatusFailure, StatusSuccess, StatusAcquired} {
		// Manually set acquired state. It's a pointer -> it's auto updated in the state
		req.Status = pointer.String(StatusAcquired)
		lpImpl.acquired = req

		updateReq := &Request{
			HeadSHA:  "sha1",
			Priority: 10,
			Status:   pointer.String(status),
		}
		_, err = lpImpl.insert(context.Background(), updateReq)
		assert.NoError(t, err)
	}
}

func Test_leaseProviderImpl_insert_invalid_state_transition(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req := &Request{
		HeadSHA:  "sha1",
		Priority: 10,
	}
	_, err := lpImpl.insert(context.Background(), req)
	assert.NoError(t, err)

	for _, previousStatus := range []string{StatusPending, StatusCompleted, StatusFailure, StatusSuccess} {
		for _, status := range []string{StatusFailure, StatusSuccess, StatusAcquired} {
			// Manually set previous state. It's a pointer -> it's auto updated in the state
			req.Status = pointer.String(previousStatus)

			updateReq := &Request{
				HeadSHA:  "sha1",
				Priority: 10,
				Status:   pointer.String(status),
			}
			_, err = lpImpl.insert(context.Background(), updateReq)
			if previousStatus == status {
				assert.NoErrorf(t, err, "previous: %s, new: %s", previousStatus, status)
			} else {
				assert.Errorf(t, err, "previous: %s, new: %s", previousStatus, status)
			}
		}
	}
}

func Test_leaseProviderImpl_evictTTL(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 10,
	}

	// Insert one request
	_, err := lpImpl.insert(context.Background(), req1)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(lpImpl.known))
	lpImpl.evictTTL(context.Background())

	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 10,
	}
	_, err = lpImpl.insert(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(lpImpl.known))

	aheadOfTime := time.Now().Add(-100 * time.Second)
	req1.lastSeenAt = &aheadOfTime
	lpImpl.evictTTL(context.Background()) // <-- eviction should evict older entries now
	assert.Equal(t, 1, len(lpImpl.known))
}

func Test_leaseProviderImpl_evictTTL_acquired(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 10 * time.Second})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req := &Request{
		HeadSHA:  "sha1",
		Priority: 10,
	}

	// Insert one request
	_, err := lpImpl.insert(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(lpImpl.known))

	aheadOfTime := time.Now().Add(-100 * time.Second)
	req.lastSeenAt = &aheadOfTime
	req.Status = pointer.String(StatusAcquired)
	lpImpl.acquired = req

	// Despite being outdated, this key should not be evicted!
	lpImpl.evictTTL(context.Background())
	assert.Equal(t, 1, len(lpImpl.known))
}

func Test_leaseProviderImpl_evaluateRequest_timePassed(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 4})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}

	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	_, err := lpImpl.insert(context.Background(), req1)
	assert.NoError(t, err)
	_, err = lpImpl.insert(context.Background(), req2)
	assert.NoError(t, err)

	// Immediately check if req2 gets the lease (it should not!)
	_ = lpImpl.evaluateRequest(context.Background(), req2)
	assert.Equal(t, *req2.Status, StatusPending)

	// Simulate a time passed by setting the last updated timestamp in the past
	updatedAt := time.Now().Add(-2 * time.Minute)
	lpImpl.lastUpdatedAt = &updatedAt
	_ = lpImpl.evaluateRequest(context.Background(), req2)
	assert.Equal(t, *req2.Status, StatusAcquired)
}

func Test_leaseProviderImpl_evaluateRequest_reachedExpectedRequestCount(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 3})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 1000,
	}
	req3 := &Request{
		HeadSHA:  "sha3",
		Priority: 100,
	}

	// Inject the two requests
	_, err := lpImpl.insert(context.Background(), req1)
	assert.NoError(t, err)
	_, err = lpImpl.insert(context.Background(), req2)
	assert.NoError(t, err)

	// Immediately check if req2 gets the lease (it should not!)
	_ = lpImpl.evaluateRequest(context.Background(), req2)
	assert.Equal(t, *req2.Status, StatusPending)

	// Add 3rd request, making it complete (it has a lower priority compared to req2)
	_, err = lpImpl.insert(context.Background(), req3)
	assert.NoError(t, err)
	_ = lpImpl.evaluateRequest(context.Background(), req2)
	assert.Equal(t, *req2.Status, StatusAcquired)
}

func Test_leaseProviderImpl_evaluateRequest_errorNoLeaseAssigned(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 3})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}

	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	_, err := lpImpl.insert(context.Background(), req1)
	assert.NoError(t, err)
	_, err = lpImpl.insert(context.Background(), req2)
	assert.NoError(t, err)

	// Set req2 to be the acquired lease
	req2.Status = pointer.String(StatusAcquired)
	lpImpl.acquired = req2

	// Make sure there's no status modification when checking if a lease is the winner
	req1copy := lpImpl.evaluateRequest(context.Background(), &Request{
		HeadSHA:  req1.HeadSHA,
		Priority: req1.Priority,
		Status:   pointer.String(StatusPending),
	})
	// It should just mirror the request when the state is acquired
	assert.Equal(t, req1copy, req1copy)
}

func Test_leaseProviderImpl__FullLoop_ReleaseSuccess(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: 1 * time.Minute, ExpectedRequestCount: 3})
	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 2,
	}
	req3 := &Request{
		HeadSHA:  "sha3",
		Priority: 3,
	}
	req3success := &Request{
		HeadSHA:  "sha3",
		Priority: 3,
		Status:   pointer.String(StatusSuccess),
	}

	reqNext := &Request{
		HeadSHA:  "next",
		Priority: 1,
	}

	req1, err := lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)

	req3, err = lp.Acquire(context.Background(), req3)
	assert.NoError(t, err)

	// Add last remaining request. The system has full knowledge now but req2 is not the winner -> should have the status pending
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req2.Status)

	// Check for req3, the winner
	req3, err = lp.Acquire(context.Background(), req3)
	assert.NoError(t, err)
	assert.Equal(t, StatusAcquired, *req3.Status)

	// The reqNext will now be rejected, since the lease acquiring is locked, and we're awaiting all other leases to return
	_, err = lp.Acquire(context.Background(), reqNext)
	assert.Error(t, err)

	// Report success status for req3
	req3, err = lp.Release(context.Background(), req3success)
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, *req3.Status)

	// Now, all other LeaseRequests will get the status COMPLETED assigned -> the process can die
	req1, err = lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, *req1.Status)

	// The reqNext should still fail as confirmation or timeout of req2 is awaited
	_, err = lp.Acquire(context.Background(), reqNext)
	assert.Error(t, err)

	// Last remaining request, send COMPLETE and afterwards the next distributed lease can start
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, *req2.Status)

	// Now, all leases are marked as successful / released. the reqNew should now be accepted
	_, err = lp.Acquire(context.Background(), reqNext)
	assert.NoError(t, err)
}

func Test_leaseProviderImpl__FullLoop_ReleaseFailedNoNewRequest(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 3})
	lpImpl, ok := lp.(*leaseProviderImpl)
	assert.True(t, ok)

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 2,
	}
	req3 := &Request{
		HeadSHA:  "sha3",
		Priority: 3,
	}
	req3failure := &Request{
		HeadSHA:  "sha3",
		Priority: 3,
		Status:   pointer.String(StatusFailure),
	}

	req1, err := lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)

	req3, err = lp.Acquire(context.Background(), req3)
	assert.NoError(t, err)

	// Add last remaining request. The system has full knowledge now but req2 is not the winner -> should have the status pending
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req2.Status)

	// Check for req3, the winner
	req3, err = lp.Acquire(context.Background(), req3)
	assert.NoError(t, err)
	assert.Equal(t, StatusAcquired, *req3.Status)

	// Report failure status for req3
	req3, err = lp.Release(context.Background(), req3failure)
	assert.NoError(t, err)
	assert.Equal(t, StatusFailure, *req3.Status)

	// The lease is released, but we did not have a successful outcome -> pass it to the next one waiting. It's not req1
	req1, err = lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req1.Status)

	// req2 has the highest priority -> it gets the lease (assuming sufficient time passed)
	// (note: backdate the stabilisation duration)
	updatedAt := time.Now().Add(-100 * time.Minute)
	lpImpl.lastUpdatedAt = &updatedAt
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusAcquired, *req2.Status)
}

func Test_leaseProviderImpl__FullLoop_ReleaseFailedNewRequest(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 3})

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 2,
	}
	req3 := &Request{
		HeadSHA:  "sha3",
		Priority: 3,
	}
	req3failure := &Request{
		HeadSHA:  "sha3",
		Priority: 3,
		Status:   pointer.String(StatusFailure),
	}
	reqNext := &Request{
		HeadSHA:  "next",
		Priority: 4,
	}

	req1, err := lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)

	req3, err = lp.Acquire(context.Background(), req3)
	assert.NoError(t, err)

	// Add last remaining request. The system has full knowledge now but req2 is not the winner -> should have the status pending
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req2.Status)

	// Check for req3, the winner
	req3, err = lp.Acquire(context.Background(), req3)
	assert.NoError(t, err)
	assert.Equal(t, StatusAcquired, *req3.Status)

	// Report failure status for req3
	req3, err = lp.Release(context.Background(), req3failure)
	assert.NoError(t, err)
	assert.Equal(t, StatusFailure, *req3.Status)

	// The lease is released, but we did not have a successful outcome -> pass it to the next one waiting. It's not req1
	req1, err = lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req1.Status)

	// A new request is coming in. Since there has been a previous failure, it should be rejected
	_, err = lp.Acquire(context.Background(), reqNext)
	assert.Error(t, err)

	// Request 2 is the highest one in the batch now
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusAcquired, *req2.Status)
}

func Test_leaseProviderImpl__FullLoop_ReleaseWithNoAcquiredLease(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 3})

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	req1, err := lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req1.Status)
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req2.Status)

	// Try to release. There should be no lease, thus error
	_, err = lp.Release(context.Background(), &Request{
		HeadSHA:  "sha1",
		Priority: 1,
		Status:   pointer.String(StatusSuccess),
	})
	assert.Error(t, err)
}

func Test_leaseProviderImpl__FullLoop_ReleaseFromInvalidHeadSHA(t *testing.T) {
	lp := NewLeaseProvider(ProviderOpts{TTL: 1 * time.Hour, StabilizeDuration: time.Minute, ExpectedRequestCount: 2})

	req1 := &Request{
		HeadSHA:  "sha1",
		Priority: 1,
	}
	req2 := &Request{
		HeadSHA:  "sha2",
		Priority: 2,
	}

	// Inject the two requests
	req1, err := lp.Acquire(context.Background(), req1)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, *req1.Status)
	req2, err = lp.Acquire(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, StatusAcquired, *req2.Status)

	// Try to release. There should be no lease, thus error
	_, err = lp.Release(context.Background(), &Request{
		HeadSHA:  "this_does_not_exist",
		Priority: 1,
		Status:   pointer.String(StatusSuccess),
	})
	assert.Error(t, err)
}

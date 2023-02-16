package lease

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/utils/pointer"
)

type ProviderOpts struct {
	StabilizeDuration    time.Duration
	TTL                  time.Duration
	ExpectedRequestCount int
}

type Status string

const (
	StatusPending   = "pending"
	StatusAcquired  = "acquired"
	StatusFailure   = "failure"
	StatusSuccess   = "success"
	StatusCompleted = "completed"
)

type Request struct {
	HeadSHA    string  `json:"head_sha"`
	Priority   int     `json:"priority"`
	Status     *string `json:"status,omitempty"`
	lastSeenAt *time.Time
}

// MarshalZerologObject allows the .Embed log context.
func (lr *Request) MarshalZerologObject(e *zerolog.Event) {
	status := ""
	if lr.Status != nil {
		status = *lr.Status
	}
	e.Str("lease_request_head_sha", lr.HeadSHA).
		Int("lease_request_priority", lr.Priority).
		Str("lease_request_status", status)
}

func (lr *Request) UpdateLastSeenAt() {
	now := time.Now()
	lr.lastSeenAt = &now
}

type Provider interface {
	Acquire(ctx context.Context, LeaseRequest *Request) (*Request, error)
	Release(ctx context.Context, LeaseRequest *Request) (*Request, error)
	GetKnown() map[string]*Request
}

// FIXME this should survive with crashes -> migrate to badger
type leaseProviderImpl struct {
	mutex sync.Mutex
	opts  ProviderOpts

	lastUpdatedAt time.Time

	acquired *Request
	known    map[string]*Request
}

func NewLeaseProvider(opts ProviderOpts) Provider {
	return &leaseProviderImpl{
		opts:          opts,
		lastUpdatedAt: time.Now(),
		known:         make(map[string]*Request),
	}
}

func (lp *leaseProviderImpl) GetKnown() map[string]*Request {
	return lp.known
}

// evictTTL performs housekeeping based on TTLs and when events have last been received
func (lp *leaseProviderImpl) evictTTL(ctx context.Context) {
	for k, v := range lp.known {
		// Do nothing when status is acquired / success.
		status := pointer.StringDeref(v.Status, StatusPending)
		if status == StatusAcquired || status == StatusSuccess {
			continue
		}
		if time.Since(*v.lastSeenAt) > lp.opts.TTL {
			delete(lp.known, k)
		}
	}
}

// evictSuccess cleanups a successful release event, so the next processing can start!
func (lp *leaseProviderImpl) cleanup(ctx context.Context) {
	// When all commits reported their status, cleanup acquire lock for the next one.
	if lp.acquired == nil {
		return
	}
	if pointer.StringDeref(lp.acquired.Status, StatusAcquired) != StatusCompleted {
		return
	}
	if len(lp.known) == 1 {
		delete(lp.known, lp.aquired.HeadSHA)
		delete(lp.known, lp.acquired.HeadSHA)
		lp.acquired = nil
	}
}

func (lp *leaseProviderImpl) insert(ctx context.Context, leaseRequest *Request) (*Request, error) {
	var updated bool = false

	// Cleanup a potential leftover lease
	lp.cleanup(ctx)

	// Update the last seen timestamp
	leaseRequest.UpdateLastSeenAt()

	// If we don't have a lease request for this commit, add it
	if existing, ok := lp.known[leaseRequest.HeadSHA]; !ok {

		if lp.acquired != nil {
			return nil, errors.New("lease already acquired")
		}

		if leaseRequest.Status != nil && pointer.StringDeref(leaseRequest.Status, StatusPending) != StatusPending {
			return nil, fmt.Errorf("invalid status %s for new LeaseRequest with HeadSHA %s", *leaseRequest.Status, leaseRequest.HeadSHA)
		}

		lp.known[leaseRequest.HeadSHA] = leaseRequest
		lp.known[leaseRequest.HeadSHA].Status = pointer.String(StatusPending)

	} else {
		// Priority changed, update it
		if existing.Priority != leaseRequest.Priority {
			existing.Priority = leaseRequest.Priority
			updated = true
		}

		// Update the state when it's a whitelisted transition (ACQUIRED -> SUCCESS/FAILURE)
		existingStatus := pointer.StringDeref(existing.Status, StatusPending)
		leaseRequestStatus := pointer.StringDeref(leaseRequest.Status, StatusPending)
		statusMismatch := existingStatus != leaseRequestStatus
		allowedTransition := existingStatus == StatusAcquired && (leaseRequestStatus == StatusSuccess || leaseRequestStatus == StatusFailure)
		// condition
		if statusMismatch && allowedTransition {
			existing.Status = &leaseRequestStatus
			updated = true
		} else if statusMismatch {
			// status mismatch, we should not get this call
			return nil, fmt.Errorf("status missmatch for commit %s; expected: `success|failure`, got: `%s`", leaseRequest.HeadSHA, leaseRequestStatus)
		}

		return existing, nil
	}

	if updated {
		lp.lastUpdatedAt = time.Now()
	}

	lp.evictTTL(ctx)
	return lp.known[leaseRequest.HeadSHA], nil
}

func (lp *leaseProviderImpl) evaluateRequest(ctx context.Context, req *Request) *Request {
	// Prereq: we can expect the arg to be already part of the map!

	if lp.aquired != nil && !(pointer.StringDeref(lp.aquired.Status, STATUS_ACQUIRED) == STATUS_FAILURE) {

	if lp.acquired != nil && !(pointer.StringDeref(lp.acquired.Status, StatusAcquired) == StatusFailure) {
		// Lock already acquired
		return req
	}
	// 1st: we reached the time limit -> lastUpdatedAt + StabilizeDuration > now
	passedStabilizeDuration := time.Since(lp.lastUpdatedAt) >= lp.opts.StabilizeDuration
	// 2nd: we received all requests and can take a decision
	// 3rd: there has been no previous failure
	reachedExpectedRequestCount := len(lp.known) >= lp.opts.ExpectedRequestCount
	if lp.aquired == nil && (!passedStabilizeDuration && !reachedExpectedRequestCount) {
	if lp.acquired == nil && (!passedStabilizeDuration && !reachedExpectedRequestCount) {
		return req
	}

	maxPriority := 0
	// get max priority
	for _, known := range lp.known {
		if known.Priority > maxPriority {
			maxPriority = known.Priority
		}
	}

	// Got the max priority, now check if we are the winner
	if req.Priority == maxPriority {
		req.Status = pointer.String(StatusAcquired)
		lp.acquired = req
	}

	return req
}

func (lp *leaseProviderImpl) Acquire(ctx context.Context, leaseRequest *Request) (*Request, error) {
	// Ensure we don't have any collisions
	lp.mutex.Lock()
	defer lp.mutex.Unlock()

	// Insert or get the correct one
	req, err := lp.insert(ctx, leaseRequest)
	if err != nil {
		return nil, err
	}

	// Check if the lease was released successful, let the client know it can die.
	if lp.acquired != nil && pointer.StringDeref(lp.acquired.Status, StatusPending) == StatusCompleted {
		req.Status = pointer.String(StatusCompleted)
		delete(lp.known, req.HeadSHA)
		return req, nil
	}

	// Return the request object with the correct status
	return lp.evaluateRequest(ctx, req), nil
}

func (lp *leaseProviderImpl) Release(ctx context.Context, leaseRequest *Request) (*Request, error) {
	lp.mutex.Lock()
	defer lp.mutex.Unlock()

	// There are several occurrences when a lease cannot be released
	// 1. No lease acquired
	if lp.acquired == nil {
		return nil, errors.New("no lease acquired")
	}
	// 2. Releasing from unknown HeadSHA that does not hold the lease
	if lp.acquired.HeadSHA != leaseRequest.HeadSHA {
		return nil, fmt.Errorf("commit %s does not hold the lease", leaseRequest.HeadSHA)
	}

	// At this point in time, we can ingest the lease
	req, err := lp.insert(ctx, leaseRequest)
	if err != nil {
		return nil, err
	}
	status := pointer.StringDeref(req.Status, StatusAcquired)

	if status == StatusSuccess {
		// On success, set status to completed so all remaining ones can be removed
		req.Status = pointer.String(StatusCompleted)
		return req, nil
	}

	if status == StatusFailure {
		// On failure, drop it. This way the next one can acquire the lease
		delete(lp.known, req.HeadSHA)
		return req, nil
	}

	return req, fmt.Errorf("unknown condition for commit %s", leaseRequest.HeadSHA)

}

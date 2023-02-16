package lease

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/utils/pointer"
)

type LeaseProviderOpts struct {
	StabilizeDuration    time.Duration
	TTL                  time.Duration
	ExpectedRequestCount int
}

type LeaseStatus string

const (
	STATUS_PENDING   = "pending"
	STATUS_AQUIRED   = "aquired"
	STATUS_FAILURE   = "failure"
	STATUS_SUCCESS   = "success"
	STATUS_COMPLETED = "completed"
)

type LeaseRequest struct {
	HeadSHA    string  `json:"head_sha"`
	Priority   int     `json:"priority"`
	Status     *string `json:"status,omitempty"`
	lastSeenAt *time.Time
}

func (lr *LeaseRequest) UpdateLastSeenAt() {
	now := time.Now()
	lr.lastSeenAt = &now
}

type LeaseProvider interface {
	Aquire(LeaseRequest *LeaseRequest) (*LeaseRequest, error)
	Release(LeaseRequest *LeaseRequest) (*LeaseRequest, error)
}

// FIXME this should survive with crashes -> migrate to badger
type leaseProviderImpl struct {
	mutex sync.Mutex
	opts  *LeaseProviderOpts

	lastUpdatedAt time.Time

	aquired *LeaseRequest
	known   map[string]*LeaseRequest
}

func NewLeaseProvider(opts *LeaseProviderOpts) LeaseProvider {
	return &leaseProviderImpl{
		opts:          opts,
		lastUpdatedAt: time.Now(),
		known:         make(map[string]*LeaseRequest),
	}
}

// evictTTL performs housekeeping based on TTLs and when events have last been received
func (lp *leaseProviderImpl) evictTTL() {
	for k, v := range lp.known {
		// Do nothing when status is aquired / success.
		status := pointer.StringDeref(v.Status, STATUS_PENDING)
		if status == STATUS_AQUIRED || status == STATUS_SUCCESS {
			continue
		}
		if time.Since(*v.lastSeenAt) > lp.opts.TTL {
			delete(lp.known, k)
		}
	}
}

// evictSuccess cleanups a successful release event, so the next processing can start!
func (lp *leaseProviderImpl) cleanup() {
	// When all commits reported their status, cleanup aquire lock for the next one.
	if lp.aquired == nil {
		return
	}
	if pointer.StringDeref(lp.aquired.Status, STATUS_AQUIRED) != STATUS_COMPLETED {
		return
	}
	if len(lp.known) == 1 {
		delete(lp.known, lp.aquired.HeadSHA)
		lp.aquired = nil
	}
}

func (lp *leaseProviderImpl) insert(leaseRequest *LeaseRequest) (*LeaseRequest, error) {
	var updated bool = false

	// Cleanup a potential leftover lease
	lp.cleanup()

	// Update the last seen timestamp
	leaseRequest.UpdateLastSeenAt()

	// If we don't have a lease request for this commit, add it
	if existing, ok := lp.known[leaseRequest.HeadSHA]; !ok {

		if lp.aquired != nil {
			return nil, errors.New("lease already aquired")
		}

		if leaseRequest.Status != nil && pointer.StringDeref(leaseRequest.Status, STATUS_PENDING) != STATUS_PENDING {
			return nil, fmt.Errorf("invalid status %s for new LeaseRequest with HeadSHA %s", *leaseRequest.Status, leaseRequest.HeadSHA)
		}

		lp.known[leaseRequest.HeadSHA] = leaseRequest
		lp.known[leaseRequest.HeadSHA].Status = pointer.String(STATUS_PENDING)

	} else {
		// Priority changed, update it
		if existing.Priority != leaseRequest.Priority {
			existing.Priority = leaseRequest.Priority
			updated = true
		}

		// Update the state when it's a whitelisted transition (AQUIRED -> SUCCESS/FAILURE)
		existingStatus := pointer.StringDeref(existing.Status, STATUS_PENDING)
		leaseRequestStatus := pointer.StringDeref(leaseRequest.Status, STATUS_PENDING)
		statusMismatch := existingStatus != leaseRequestStatus
		allowedTransition := (existingStatus == STATUS_AQUIRED && (leaseRequestStatus == STATUS_SUCCESS || leaseRequestStatus == STATUS_FAILURE))
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

	lp.evictTTL()
	return lp.known[leaseRequest.HeadSHA], nil
}

func (lp *leaseProviderImpl) isWinningLeaseRequest(req *LeaseRequest) *LeaseRequest {
	// Prereq: we can expect the arg to be already part of the map!

	if lp.aquired != nil {
		// Lock already aquired
		return req
	}
	// 1st: we reached the time limit -> lastUpdatedAt + StabilizeDuration > now
	passedStabilizeDuration := time.Since(lp.lastUpdatedAt) >= lp.opts.StabilizeDuration
	// 2nd: we received all requests and can take a decision
	reachedExpectedRequestCount := len(lp.known) >= lp.opts.ExpectedRequestCount
	if !passedStabilizeDuration && !reachedExpectedRequestCount {
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
		req.Status = pointer.String(STATUS_AQUIRED)
		lp.aquired = req
	}

	return req
}

func (lp *leaseProviderImpl) Aquire(leaseRequest *LeaseRequest) (*LeaseRequest, error) {
	// Ensure we don't have any collisions
	lp.mutex.Lock()
	defer lp.mutex.Unlock()

	// Insert or get the correct one
	req, err := lp.insert(leaseRequest)
	if err != nil {
		return nil, err
	}

	// Check if the lease was released successful, let the client know it can die.
	if lp.aquired != nil && pointer.StringDeref(lp.aquired.Status, STATUS_PENDING) == STATUS_COMPLETED {
		req.Status = pointer.String(STATUS_COMPLETED)
		delete(lp.known, req.HeadSHA)
		return req, nil
	}

	// Return the request object with the correct status
	return lp.isWinningLeaseRequest(req), nil
}

func (lp *leaseProviderImpl) Release(leaseRequest *LeaseRequest) (*LeaseRequest, error) {
	lp.mutex.Lock()
	defer lp.mutex.Unlock()

	// There are several occurences when a lease cannot be released
	// 1. No lease aquired
	if lp.aquired == nil {
		return nil, errors.New("no lease aquired")
	}
	// 2. Releasing from unknown HeadSHA that does not hold the lease
	if lp.aquired.HeadSHA != leaseRequest.HeadSHA {
		return nil, fmt.Errorf("commit %s does not hold the lease", leaseRequest.HeadSHA)
	}

	// At this point in time, we can ingest the lease
	req, err := lp.insert(leaseRequest)
	if err != nil {
		return nil, err
	}
	status := pointer.StringDeref(req.Status, STATUS_AQUIRED)

	if status == STATUS_SUCCESS {
		// On success, set status to completed so all remaining ones can be removed
		req.Status = pointer.String(STATUS_COMPLETED)
		return req, nil
	}

	if status == STATUS_FAILURE {
		// On failure, drop it. This way the next one can aquire the lease
		delete(lp.known, req.HeadSHA)
		lp.aquired = nil
		return req, nil
	}

	return req, fmt.Errorf("unknown condition for commit %s", leaseRequest.HeadSHA)

}

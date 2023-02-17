package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	Acquire(ctx context.Context, leaseRequest *Request) (*Request, error)
	Release(ctx context.Context, leaseRequest *Request) (*Request, error)
}

// FIXME this should survive with crashes -> migrate to badger
type leaseProviderImpl struct {
	mutex sync.Mutex
	opts  ProviderOpts

	lastUpdatedAt *time.Time

	acquired *Request
	known    map[string]*Request
}

func NewLeaseProvider(opts ProviderOpts) Provider {
	return &leaseProviderImpl{
		opts:          opts,
		lastUpdatedAt: nil,
		known:         make(map[string]*Request),
	}
}

func (lp *leaseProviderImpl) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		LastUpdatedAt *time.Time          `json:"last_updated_at"`
		Acquired      *Request            `json:"acquired"`
		Known         map[string]*Request `json:"known"`
	}{
		LastUpdatedAt: lp.lastUpdatedAt,
		Acquired:      lp.acquired,
		Known:         lp.known,
	})
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
			log.Ctx(ctx).Debug().EmbedObject(v).Msg("Request evicted (TTL)")
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
		log.Ctx(ctx).Debug().EmbedObject(lp.acquired).Msg("Cleanup completed request")
		delete(lp.known, lp.acquired.HeadSHA)
		lp.acquired = nil
	}
}

func (lp *leaseProviderImpl) insert(ctx context.Context, leaseRequest *Request) (*Request, error) {
	log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msg("Inserting new lease request")

	updated := false

	// Cleanup a potential leftover lease
	lp.cleanup(ctx)

	// Update the last seen timestamp
	leaseRequest.UpdateLastSeenAt()

	// If we don't have a lease request for this commit, add it
	if existing, ok := lp.known[leaseRequest.HeadSHA]; !ok {
		log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msg("Lease request is new")
		if lp.acquired != nil {
			return nil, errors.New("lease already acquired")
		}

		if leaseRequest.Status != nil && pointer.StringDeref(leaseRequest.Status, StatusPending) != StatusPending {
			return nil, fmt.Errorf("invalid status %s for new LeaseRequest with HeadSHA %s", *leaseRequest.Status, leaseRequest.HeadSHA)
		}

		lp.known[leaseRequest.HeadSHA] = leaseRequest
		lp.known[leaseRequest.HeadSHA].Status = pointer.String(StatusPending)
		updated = true
	} else {
		log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msg("Lease request is already existing")
		// Priority changed, update it
		if existing.Priority != leaseRequest.Priority {
			log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msgf("Lease request priority has changed (previous: %d, new: %d)", existing.Priority, leaseRequest.Priority)
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
			log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msgf("Lease request status has changed (previous: %s, new: %s)", existingStatus, leaseRequestStatus)
			existing.Status = &leaseRequestStatus
			updated = true
		} else if statusMismatch {
			// status mismatch, we should not get this call
			return nil, fmt.Errorf("status missmatch for commit %s; expected: `success|failure`, got: `%s`", leaseRequest.HeadSHA, leaseRequestStatus)
		}

		log.Ctx(ctx).Debug().EmbedObject(existing).Msg("Lease request updated")
	}

	if updated {
		now := time.Now()
		lp.lastUpdatedAt = &now
		log.Ctx(ctx).Debug().Msgf("Provider last updated time bumped (new time: %s, StabilizeDuration now ends at %s)", lp.lastUpdatedAt.Format(time.RFC3339), lp.lastUpdatedAt.Add(lp.opts.StabilizeDuration).Format(time.RFC3339))
	}

	lp.evictTTL(ctx)
	return lp.known[leaseRequest.HeadSHA], nil
}

func (lp *leaseProviderImpl) evaluateRequest(ctx context.Context, req *Request) *Request {
	// Prereq: we can expect the arg to be already part of the map!

	log.Ctx(ctx).Debug().EmbedObject(req).Msg("Evaluating lease request")

	if lp.acquired != nil && !(pointer.StringDeref(lp.acquired.Status, StatusAcquired) == StatusFailure) {
		// Lock already acquired
		log.Ctx(ctx).Debug().EmbedObject(req).Msgf("Lock already acquired (by sha %s, priority %d)", lp.acquired.HeadSHA, lp.acquired.Priority)
		return req
	}
	// 1st: we reached the time limit -> lastUpdatedAt + StabilizeDuration > now
	passedStabilizeDuration := time.Since(*lp.lastUpdatedAt) >= lp.opts.StabilizeDuration
	log.Ctx(ctx).Debug().Msg("Now: " + time.Now().Format(time.RFC3339))
	log.Ctx(ctx).Debug().EmbedObject(req).Msgf("Stabilize duration check: Duration config: %.0fs, Last updated at: %s, Stabilize duration end: %s, Stabilize duration passed: %t", lp.opts.StabilizeDuration.Seconds(), lp.lastUpdatedAt.Format(time.RFC3339), lp.lastUpdatedAt.Add(lp.opts.StabilizeDuration).Format(time.RFC3339), passedStabilizeDuration)
	// 2nd: we received all requests and can take a decision
	// 3rd: there has been no previous failure
	reachedExpectedRequestCount := len(lp.known) >= lp.opts.ExpectedRequestCount
	log.Ctx(ctx).Debug().EmbedObject(req).Msgf("Expected request count check: config: %d, actual: %d, expected request count reached: %t", lp.opts.ExpectedRequestCount, len(lp.known), reachedExpectedRequestCount)

	if lp.acquired == nil && (!passedStabilizeDuration && !reachedExpectedRequestCount) {
		log.Ctx(ctx).Debug().EmbedObject(req).Msg("Stabilize duration has not been met yet, or we're still waiting for more request to register")
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
		log.Ctx(ctx).Debug().EmbedObject(req).Msg("Current lease request has the higher priority. It then acquires the lock")
		log.Ctx(ctx).Info().EmbedObject(req).Msg("Lock acquired")
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
	log.Ctx(ctx).Debug().EmbedObject(req).Msg("Lease request has been inserted")

	// Check if the lease was released successful, let the client know it can die.
	if lp.acquired != nil && pointer.StringDeref(lp.acquired.Status, StatusPending) == StatusCompleted {
		req.Status = pointer.String(StatusCompleted)
		delete(lp.known, req.HeadSHA)
		log.Ctx(ctx).Info().EmbedObject(req).Msg("Lock holder succeeded. Current lease request completed")
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

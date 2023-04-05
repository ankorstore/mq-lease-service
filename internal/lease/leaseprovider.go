package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/storage"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/utils/clock"
	"k8s.io/utils/pointer"
)

var refRegex *regexp.Regexp

func init() {
	// ex: gh-readonly-queue/develop/pr-31132-d107b89c095dd85ba6c62b8a4503100ee33a04bb
	refRegex = regexp.MustCompile(`^gh-readonly-queue/([^/]+)/pr-(\d+)-([0-9a-fA-F]+)$`)
}

type ProviderOpts struct {
	StabilizeDuration    time.Duration
	TTL                  time.Duration
	ExpectedRequestCount int
	ID                   string
	Clock                clock.PassiveClock
	Storage              storage.Storage[*ProviderState]
	Metrics              *providerMetrics
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
	HeadRef    string  `json:"head_ref"`
	Priority   int     `json:"priority"`
	Status     *string `json:"status,omitempty"`
	lastSeenAt *time.Time
}

type StackedPullRequest struct {
	Number int `json:"number"`
}

type RequestContext struct {
	Request             *Request              `json:"request"`
	StackedPullRequests []*StackedPullRequest `json:"stacked_pull_requests,omitempty"`
}

func (lr *Request) UpdateLastSeenAt(t time.Time) {
	lr.lastSeenAt = &t
}

// MarshalZerologObject allows the .Embed log context.
func (lr *Request) MarshalZerologObject(e *zerolog.Event) {
	status := ""
	if lr.Status != nil {
		status = *lr.Status
	}
	e.Str("lease_request_head_sha", lr.HeadSHA).
		Str("lease_request_head_ref", lr.HeadRef).
		Int("lease_request_priority", lr.Priority).
		Str("lease_request_status", status)
}

// ProviderState is the in-memory representation of the current merge queue.
// This struct is persisted in the storage.
type ProviderState struct {
	id            string
	lastUpdatedAt time.Time
	acquired      *Request
	known         map[string]*Request
}

type NewProviderStateOpts struct {
	ID            string
	LastUpdatedAt time.Time
	Acquired      *Request
	Known         map[string]*Request
}

func NewProviderState(opts NewProviderStateOpts) *ProviderState {
	if opts.Known == nil {
		opts.Known = make(map[string]*Request)
	}
	return &ProviderState{
		id:            opts.ID,
		lastUpdatedAt: opts.LastUpdatedAt,
		acquired:      opts.Acquired,
		known:         opts.Known,
	}
}

func (ps *ProviderState) GetIdentifier() string {
	return ps.id
}

type providerStateRequestStorePayload struct {
	HeadSHA    string     `json:"head_sha"`
	HeadRef    string     `json:"head_ref"`
	Priority   int        `json:"priority"`
	Status     *string    `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}
type providerStateStorePayload struct {
	ID            string                                       `json:"id"`
	LastUpdatedAt time.Time                                    `json:"last_updated_at"`
	AcquiredSHA   *string                                      `json:"acquired_sha"`
	Known         map[string]*providerStateRequestStorePayload `json:"known"`
}

// Marshal used to marshal the state before being stored
func (ps *ProviderState) Marshal() ([]byte, error) {
	var acquiredSHA *string
	if ps.acquired != nil {
		acquiredSHA = &ps.acquired.HeadSHA
	}
	known := map[string]*providerStateRequestStorePayload{}
	for k, v := range ps.known {
		known[k] = &providerStateRequestStorePayload{
			HeadSHA:    v.HeadSHA,
			HeadRef:    v.HeadRef,
			Priority:   v.Priority,
			Status:     v.Status,
			LastSeenAt: v.lastSeenAt,
		}
	}
	res, err := json.Marshal(&providerStateStorePayload{
		ID:            ps.id,
		LastUpdatedAt: ps.lastUpdatedAt,
		AcquiredSHA:   acquiredSHA,
		Known:         known,
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Unmarshal used to unmarshal the state from the store to its native type
func (ps *ProviderState) Unmarshal(b []byte) error {
	p := &providerStateStorePayload{}
	err := json.Unmarshal(b, p)
	if err != nil {
		return err
	}
	ps.id = p.ID
	ps.lastUpdatedAt = p.LastUpdatedAt
	known := map[string]*Request{}
	for k, v := range p.Known {
		known[k] = &Request{
			HeadSHA:    v.HeadSHA,
			HeadRef:    v.HeadRef,
			Priority:   v.Priority,
			Status:     v.Status,
			lastSeenAt: v.LastSeenAt,
		}
	}
	ps.known = known
	if p.AcquiredSHA != nil {
		ps.acquired = ps.known[*p.AcquiredSHA]
	}
	return nil
}

type Provider interface {
	Acquire(ctx context.Context, leaseRequest *Request) (*Request, error)
	Release(ctx context.Context, leaseRequest *Request) (*Request, error)
	BuildRequestContext(ctx context.Context, leaseRequest *Request) (*RequestContext, error)
	HydrateFromState(ctx context.Context) error
	Clear(ctx context.Context)
}

type leaseProviderImpl struct {
	mutex   sync.Mutex
	opts    ProviderOpts
	clock   clock.PassiveClock
	storage storage.Storage[*ProviderState]
	metrics *providerMetrics

	state *ProviderState
}

func NewLeaseProvider(opts ProviderOpts) Provider {
	cl := opts.Clock
	// if no Clock service is provided, fallback to a Real clock
	if cl == nil {
		cl = clock.RealClock{}
	}
	st := opts.Storage
	// if no Storage service is provided, fallback to a Null storage
	if st == nil {
		st = storage.NullStorage[*ProviderState]{}
	}

	return &leaseProviderImpl{
		opts:    opts,
		clock:   cl,
		storage: st,
		metrics: opts.Metrics,
		state: NewProviderState(NewProviderStateOpts{
			ID:            opts.ID,
			LastUpdatedAt: cl.Now(),
		}),
	}
}

func (lp *leaseProviderImpl) HydrateFromState(ctx context.Context) error {
	if err := lp.storage.Hydrate(ctx, lp.state); err != nil {
		return err
	}
	lp.updateMetrics()
	return nil
}

// MarshalJSON used to marshall the provider to its JSON form (used in API responses)
func (lp *leaseProviderImpl) MarshalJSON() ([]byte, error) {
	requestContexts := make([]*RequestContext, 0, len(lp.state.known))
	// build lease request context (= request data + stacked Pulls data)
	for _, r := range lp.state.known {
		reqContext, err := lp.BuildRequestContext(context.Background(), r)
		if err != nil {
			return []byte{}, err
		}
		requestContexts = append(requestContexts, reqContext)
	}

	// sort the known requests (low priority = higher in the list)
	sort.SliceStable(requestContexts, func(i, j int) bool {
		return requestContexts[i].Request.Priority < requestContexts[j].Request.Priority
	})

	// build the request context for the acquired request
	acquiredReqContext, err := lp.BuildRequestContext(context.Background(), lp.state.acquired)
	if err != nil {
		return []byte{}, err
	}

	return json.Marshal(&struct {
		LastUpdatedAt time.Time         `json:"last_updated_at"`
		Acquired      *RequestContext   `json:"acquired"`
		Known         []*RequestContext `json:"known"`
	}{
		LastUpdatedAt: lp.state.lastUpdatedAt,
		Acquired:      acquiredReqContext,
		Known:         requestContexts,
	})
}

func (lp *leaseProviderImpl) saveState(ctx context.Context) {
	// Ignore upstream context, as this has to run no matter if the context is cancelled or not
	err := lp.storage.Save(context.Background(), lp.state)
	if err != nil {
		log.Ctx(ctx).
			Error().
			Str("lease_provider_id", lp.state.id).
			Err(err).
			Msg("Failed to save provider")
	}
}

// updateRequestLastSeenAt bump the last seen date on the request
func (lp *leaseProviderImpl) updateRequestLastSeenAt(request *Request) {
	now := lp.clock.Now()
	request.UpdateLastSeenAt(now)
}

// evictTTL performs housekeeping based on TTLs and when events have last been received
func (lp *leaseProviderImpl) evictTTL(ctx context.Context) {
	for k, v := range lp.state.known {
		// Do nothing when status is acquired / success.
		status := pointer.StringDeref(v.Status, StatusPending)
		if status == StatusAcquired || status == StatusSuccess {
			continue
		}
		if lp.clock.Since(*v.lastSeenAt) > lp.opts.TTL {
			log.Ctx(ctx).Debug().EmbedObject(v).Msg("Request evicted (TTL)")
			delete(lp.state.known, k)
		}
	}
}

// cleanup cleanups a successful release event, so the next processing can start!
func (lp *leaseProviderImpl) cleanup(ctx context.Context) {
	// When all commits reported their status, cleanup acquire lock for the next one.
	if lp.state.acquired == nil {
		return
	}
	if pointer.StringDeref(lp.state.acquired.Status, StatusAcquired) != StatusCompleted {
		return
	}
	if len(lp.state.known) == 1 {
		log.Ctx(ctx).Debug().EmbedObject(lp.state.acquired).Msg("Cleanup completed request")
		delete(lp.state.known, lp.state.acquired.HeadSHA)
		lp.state.acquired = nil
	}
}

// insert is trying to insert (or update) the request into the in-memory known requests list
func (lp *leaseProviderImpl) insert(ctx context.Context, leaseRequest *Request) (*Request, error) {
	log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msg("Inserting new lease request")

	updated := false

	// Cleanup a potential leftover lease
	lp.cleanup(ctx)

	// Update the last seen timestamp
	lp.updateRequestLastSeenAt(leaseRequest)

	// If we don't have a lease request for this commit, add it
	if existing, ok := lp.state.known[leaseRequest.HeadSHA]; !ok {
		log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msg("Lease request is new")
		if lp.state.acquired != nil {
			return nil, errors.New("lease already acquired")
		}

		if leaseRequest.Status != nil && pointer.StringDeref(leaseRequest.Status, StatusPending) != StatusPending {
			return nil, fmt.Errorf("invalid status %s for new LeaseRequest with HeadSHA %s", *leaseRequest.Status, leaseRequest.HeadSHA)
		}

		lp.state.known[leaseRequest.HeadSHA] = leaseRequest
		lp.state.known[leaseRequest.HeadSHA].Status = pointer.String(StatusPending)
		updated = true
	} else {
		log.Ctx(ctx).Debug().EmbedObject(leaseRequest).Msg("Lease request is already existing")
		// Priority changed, update it
		if existing.Priority != leaseRequest.Priority {
			log.Ctx(ctx).
				Debug().
				EmbedObject(leaseRequest).
				Int("previous_priority", existing.Priority).
				Int("new_priority", leaseRequest.Priority).
				Msg("Lease request priority has changed")
			existing.Priority = leaseRequest.Priority
			updated = true
		}

		// Head ref changed, update it
		if existing.HeadRef != leaseRequest.HeadRef {
			log.Ctx(ctx).
				Debug().
				EmbedObject(leaseRequest).
				Str("previous_head_ref", existing.HeadRef).
				Str("new_head_ref", leaseRequest.HeadRef).
				Msg("Lease request head ref has changed")
			existing.HeadRef = leaseRequest.HeadRef
			updated = true
		}

		// Update the state when it's a whitelisted transition (ACQUIRED -> SUCCESS/FAILURE)
		existingStatus := pointer.StringDeref(existing.Status, StatusPending)
		// Check if it's a whitelisted transition
		leaseRequestStatus := pointer.StringDeref(leaseRequest.Status, StatusPending)
		statusMismatch := existingStatus != leaseRequestStatus
		allowedTransition := existingStatus == StatusAcquired && (leaseRequestStatus == StatusSuccess || leaseRequestStatus == StatusFailure)
		// condition
		if statusMismatch && allowedTransition {
			log.Ctx(ctx).
				Debug().
				EmbedObject(leaseRequest).
				Str("previous_status", existingStatus).
				Str("new_status", leaseRequestStatus).
				Msg("Lease request status has changed")
			existing.Status = &leaseRequestStatus
			updated = true
		} else if statusMismatch {
			// status mismatch, we should not get this call
			return nil, fmt.Errorf("status missmatch for commit %s; expected: `success|failure`, got: `%s`", leaseRequest.HeadSHA, leaseRequestStatus)
		}

		// Update existing request no matter if it changed or not (it's used for TTL eviction)
		lp.updateRequestLastSeenAt(existing)
		log.Ctx(ctx).Debug().EmbedObject(existing).Msg("Lease request updated")
	}

	if updated {
		lp.state.lastUpdatedAt = lp.clock.Now()
		log.Ctx(ctx).
			Debug().
			Time("new_last_updated_at", lp.state.lastUpdatedAt).
			Time("new_stabilize_ends_at", lp.state.lastUpdatedAt.Add(lp.opts.StabilizeDuration)).
			Msg("Provider last updated time bumped")
	}

	lp.evictTTL(ctx)
	return lp.state.known[leaseRequest.HeadSHA], nil
}

// evaluateRequest evaluate the given request status
func (lp *leaseProviderImpl) evaluateRequest(ctx context.Context, req *Request) *Request {
	// Prereq: we can expect the arg to be already part of the map!

	log.Ctx(ctx).Debug().EmbedObject(req).Msg("Evaluating lease request")

	if lp.state.acquired != nil && !(pointer.StringDeref(lp.state.acquired.Status, StatusAcquired) == StatusFailure) {
		// Lock already acquired
		log.Ctx(ctx).
			Debug().
			EmbedObject(req).
			Msgf("Lock already acquired (by sha %s, priority %d)", lp.state.acquired.HeadSHA, lp.state.acquired.Priority)
		return req
	}
	// 1st: we reached the time limit -> lastUpdatedAt + StabilizeDuration > now
	passedStabilizeDuration := lp.clock.Since(lp.state.lastUpdatedAt) >= lp.opts.StabilizeDuration
	log.Ctx(ctx).
		Debug().
		EmbedObject(req).
		Float64("config_stabilize_duration_sec", lp.opts.StabilizeDuration.Seconds()).
		Time("last_updated_at", lp.state.lastUpdatedAt).
		Time("stabilize_ends_at", lp.state.lastUpdatedAt.Add(lp.opts.StabilizeDuration)).
		Time("current_time", lp.clock.Now()).
		Bool("stabilize_duration_passed", passedStabilizeDuration).
		Msg("Stabilize duration check")

	// 2nd: we received all requests and can take a decision
	reachedExpectedRequestCount := len(lp.state.known) >= lp.opts.ExpectedRequestCount
	log.Ctx(ctx).
		Debug().
		EmbedObject(req).
		Int("config_expected_request_count", lp.opts.ExpectedRequestCount).
		Int("actual_request_count", len(lp.state.known)).
		Bool("expected_request_count_reached", reachedExpectedRequestCount).
		Msg("Expected request count check")

	// 3rd: there has been no previous failure
	if lp.state.acquired == nil && (!passedStabilizeDuration && !reachedExpectedRequestCount) {
		log.Ctx(ctx).
			Debug().
			EmbedObject(req).
			Msg("Stabilize duration has not been met yet, or we're still waiting for more request to register")
		return req
	}

	maxPriority := 0
	// get max priority
	for _, known := range lp.state.known {
		if known.Priority > maxPriority {
			maxPriority = known.Priority
		}
	}

	// Got the max priority, now check if we are the winner
	if req.Priority == maxPriority {
		req.Status = pointer.String(StatusAcquired)
		lp.state.acquired = req
		log.Ctx(ctx).
			Debug().
			EmbedObject(req).
			Msg("Current lease request has the higher priority. It then acquires the lock")
		log.Ctx(ctx).
			Info().
			EmbedObject(req).
			Msg("Lock acquired")
	}

	return req
}

func (lp *leaseProviderImpl) computeStackedPullRequests(leaseRequest *Request) ([]*StackedPullRequest, error) {
	if nil == leaseRequest {
		return make([]*StackedPullRequest, 0), nil
	}

	// consider only the other requests which have lower priority (+ current one)
	filteredRequestKeys := make([]string, 0, len(lp.state.known))
	for k, r := range lp.state.known {
		if r.Priority > leaseRequest.Priority {
			continue
		}
		filteredRequestKeys = append(filteredRequestKeys, k)
	}
	// sort the filtered requests by priority
	sort.SliceStable(filteredRequestKeys, func(i, j int) bool {
		return lp.state.known[filteredRequestKeys[i]].Priority < lp.state.known[filteredRequestKeys[j]].Priority
	})

	stackedPullRequests := make([]*StackedPullRequest, 0, len(filteredRequestKeys))
	// compute the stacked pr list (by looping over the filtered/sorted requests)
	for _, k := range filteredRequestKeys {
		prNumber, err := getPRNumberFromRef(lp.state.known[k].HeadRef)
		if err != nil {
			return stackedPullRequests, err
		}

		stackedPullRequests = append(stackedPullRequests, &StackedPullRequest{
			Number: prNumber,
		})
	}
	return stackedPullRequests, nil
}

func (lp *leaseProviderImpl) updateMetrics() {
	if lp.metrics != nil {
		queueSize := 0
		for _, r := range lp.state.known {
			if pointer.StringDeref(r.Status, StatusCompleted) != StatusCompleted {
				queueSize++
			}
		}

		lp.metrics.queueSize.WithLabelValues(lp.opts.ID).Set(float64(queueSize))
	}
}

func (lp *leaseProviderImpl) Acquire(ctx context.Context, leaseRequest *Request) (*Request, error) {
	// Ensure we don't have any collisions
	lp.mutex.Lock()
	defer lp.mutex.Unlock()
	defer lp.updateMetrics()

	// Save the state to storage
	defer lp.saveState(ctx)

	// Insert or get the correct one
	req, err := lp.insert(ctx, leaseRequest)
	if err != nil {
		return nil, err
	}
	log.Ctx(ctx).Debug().EmbedObject(req).Msg("Lease request has been inserted")

	// Check if the lease was released successful, let the client know it can die.
	if lp.state.acquired != nil && pointer.StringDeref(lp.state.acquired.Status, StatusPending) == StatusCompleted {
		req.Status = pointer.String(StatusCompleted)
		delete(lp.state.known, req.HeadSHA)
		log.Ctx(ctx).Info().EmbedObject(req).Msg("Lock holder succeeded. Current lease request completed")
		return req, nil
	}

	// Return the request object with the correct status
	return lp.evaluateRequest(ctx, req), nil
}

func (lp *leaseProviderImpl) Release(ctx context.Context, leaseRequest *Request) (*Request, error) {
	lp.mutex.Lock()
	defer lp.mutex.Unlock()
	defer lp.updateMetrics()

	// Save the state to storage
	defer lp.saveState(ctx)

	// There are several occurrences when a lease cannot be released
	// 1. No lease acquired
	if lp.state.acquired == nil {
		return nil, errors.New("no lease acquired")
	}
	// 2. Releasing from unknown HeadSHA that does not hold the lease
	if lp.state.acquired.HeadSHA != leaseRequest.HeadSHA {
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

		if lp.metrics != nil {
			// compute merged batch size to report in the metrics
			mergedBatchSize := 1
			for _, known := range lp.state.known {
				if known.Priority < req.Priority {
					mergedBatchSize++
				}
			}
			lp.metrics.mergedBatchSize.WithLabelValues(lp.opts.ID).Observe(float64(mergedBatchSize))
		}

		return req, nil
	}

	if status == StatusFailure {
		// On failure, drop it. This way the next one can acquire the lease
		delete(lp.state.known, req.HeadSHA)
		// when it is the last one, we can reset the state
		if len(lp.state.known) == 0 {
			lp.state.acquired = nil
		}
		return req, nil
	}

	return req, fmt.Errorf("unknown condition for commit %s", leaseRequest.HeadSHA)
}

func (lp *leaseProviderImpl) BuildRequestContext(ctx context.Context, leaseRequest *Request) (*RequestContext, error) {
	// a request context is a combination of a request object and its stacked pull requests info
	if nil == leaseRequest {
		return nil, nil
	}

	requestContext := &RequestContext{
		Request: leaseRequest,
	}

	if pointer.StringDeref(leaseRequest.Status, StatusPending) != StatusAcquired {
		return requestContext, nil
	}

	stackedPulls, err := lp.computeStackedPullRequests(leaseRequest)
	if err != nil {
		log.Ctx(ctx).
			Error().
			EmbedObject(leaseRequest).
			Err(err).
			Msg("Failed to build request context")

		return requestContext, err
	}
	requestContext.StackedPullRequests = stackedPulls
	return requestContext, nil
}

func (lp *leaseProviderImpl) Clear(ctx context.Context) {
	lp.mutex.Lock()
	defer lp.mutex.Unlock()
	defer lp.updateMetrics()

	lp.state = NewProviderState(NewProviderStateOpts{
		ID:            lp.state.id,
		LastUpdatedAt: lp.clock.Now(),
	})

	lp.saveState(ctx)
}

// getPRNumberFromRef extract pull request number from a GH read-only branch ref name
func getPRNumberFromRef(ref string) (int, error) {
	matches := refRegex.FindStringSubmatch(ref)

	if len(matches) == 0 {
		return 0, fmt.Errorf("could not extract PR number from ref: invalid ref format (given: `%s`)", ref)
	}

	prNumber, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, fmt.Errorf("could not extract PR number from ref: invalid PR integer (given: `%s`, ref: `%s`)", matches[2], ref)
	}
	return prNumber, nil
}

func ValidateGHTempRef(ref string) bool {
	return refRegex.MatchString(ref)
}

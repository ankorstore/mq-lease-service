package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	configHelper "github.com/ankorstore/gh-action-mq-lease-service/e2e/helpers/config"
	serverHelper "github.com/ankorstore/gh-action-mq-lease-service/e2e/helpers/server"
	storageHelper "github.com/ankorstore/gh-action-mq-lease-service/e2e/helpers/storage"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/pointer"
)

var _ = Describe("API", Ordered, func() {
	var config *configHelper.Helper
	var storage *storageHelper.Helper
	var clk *testing.FakePassiveClock
	var now time.Time
	var srv server.Server
	var storageDir string

	var owner string
	var repo string
	var baseRef string

	BeforeAll(func() {
		config = configHelper.NewHelper()
		storage = storageHelper.NewHelper()
		now, _ = time.Parse(time.RFC3339, "2023-01-01T10:00:00+01:00")
		clk = testing.NewFakePassiveClock(now)

		DeferCleanup(func() {
			// will clean up the temporary folder/files created for the config and the storage
			config.Cleanup()
			storage.Cleanup()
		})
	})

	BeforeEach(func() {
		// created a temporary storage dir (so that each test is working in isolation)
		storageDir = storage.NewStorageDir()
	})

	JustBeforeEach(func() {
		// use the default configuration used in the config helper
		_, configPath := config.LoadDefaultConfig()
		owner = configHelper.DefaultConfigRepoOwner
		repo = configHelper.DefaultConfigRepoName
		baseRef = configHelper.DefaultConfigRepoBaseRef

		// bootstrap a new server (will run the usual bootstrapping sequence, like starting the storage etc...)
		srv = serverHelper.CreateAndInit(configPath, storageDir, clk)
		DeferCleanup(func() {
			// Will gracefully shut down the server (useful to test if shutdown methods work correctly)
			serverHelper.Cleanup(srv)
			// Will clean up any env vars configured to test the placeholder replacement feature of the config
			config.CleanupEnv()
		})
	})

	Describe("Provider listing endpoint", func() {
		var providerListingResp *http.Response
		var providerListingRespBody string

		JustBeforeEach(func() {
			// make the API call for each test, before asserting on the response
			providerListingResp, providerListingRespBody = apiCall(srv, providerListReq())
		})

		It("should always return a 200 response", func() {
			Expect(providerListingResp.StatusCode).To(Equal(http.StatusOK))
		})

		Context("when the provider has no known lease requests", func() {
			It("should return an empty list of requests", func() {
				expectedPayload := fmt.Sprintf(`{
					"%s:%s:%s": {
						"last_updated_at": "%s",
						"acquired": null,
						"known": {}
					}
				}`, owner, repo, baseRef, now.Format(time.RFC3339))

				Expect(providerListingRespBody).To(MatchJSON(expectedPayload))
			})
		})

		Context("when the provider has some known lease requests", func() {
			var providerStateOpts *lease.NewProviderStateOpts
			BeforeEach(func() {
				var providerState *lease.ProviderState
				providerState, providerStateOpts = generateProviderState(now, owner, repo, baseRef, map[int]lease.Status{
					1: lease.StatusPending,
					2: lease.StatusPending,
					3: lease.StatusPending,
					4: lease.StatusAcquired,
				}, pointer.Int(4))

				storage.PrefillStorage(storageDir, providerState)
			})
			It("should return an non-empty list of requests", func() {
				leaseRequestsPayloadsJSON, _ := json.Marshal(providerStateOpts.Known)
				acquiredLeaseRequestPayloadJSON, _ := json.Marshal(providerStateOpts.Acquired)
				expectedPayload := fmt.Sprintf(`{
					"%s:%s:%s": {
						"last_updated_at": "%s",
						"acquired": %s,
						"known": %s
					}
				}`, owner, repo, baseRef, providerStateOpts.LastUpdatedAt.Format(time.RFC3339), string(acquiredLeaseRequestPayloadJSON), string(leaseRequestsPayloadsJSON))

				Expect(providerListingRespBody).To(MatchJSON(expectedPayload))
			})
		})
	})

	Describe("Provider details endpoint", func() {
		var providerDetailsResp *http.Response
		var providerDetailsRespBody string

		Context("when the provider is unknown", func() {
			JustBeforeEach(func() {
				providerDetailsResp, providerDetailsRespBody = apiCall(srv, providerDetailsReq("unknown", "unknown", "unknown"))
			})

			It("should return a 404 response", func() {
				Expect(providerDetailsResp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the provider is known", func() {
			JustBeforeEach(func() {
				providerDetailsResp, providerDetailsRespBody = apiCall(srv, providerDetailsReq(owner, repo, baseRef))
			})

			It("should return a 200 response", func() {
				Expect(providerDetailsResp.StatusCode).To(Equal(http.StatusOK))
			})

			Context("when the provider has no known lease requests", func() {
				It("should return an empty list of requests", func() {
					expectedPayload := fmt.Sprintf(`{
						"last_updated_at": "%s",
						"acquired": null,
						"known": {}
					}`, now.Format(time.RFC3339))

					Expect(providerDetailsRespBody).To(MatchJSON(expectedPayload))
				})
			})

			Context("when the provider has some known lease requests", func() {
				var providerStateOpts *lease.NewProviderStateOpts
				BeforeEach(func() {
					var providerState *lease.ProviderState
					providerState, providerStateOpts = generateProviderState(now, owner, repo, baseRef, map[int]lease.Status{
						1: lease.StatusPending,
						2: lease.StatusPending,
						3: lease.StatusPending,
						4: lease.StatusAcquired,
					}, pointer.Int(4))

					storage.PrefillStorage(storageDir, providerState)
				})
				It("should return an non-empty list of requests", func() {
					leaseRequestsPayloadsJSON, _ := json.Marshal(providerStateOpts.Known)
					acquiredLeaseRequestPayloadJSON, _ := json.Marshal(providerStateOpts.Acquired)
					expectedPayload := fmt.Sprintf(`{
						"last_updated_at": "%s",
						"acquired": %s,
						"known": %s
					}`, providerStateOpts.LastUpdatedAt.Format(time.RFC3339), string(acquiredLeaseRequestPayloadJSON), string(leaseRequestsPayloadsJSON))

					Expect(providerDetailsRespBody).To(MatchJSON(expectedPayload))
				})
			})
		})
	})

	Describe("Acquire endpoint", func() {
		BeforeEach(func() {
			clk.SetTime(now)
		})

		Context("when the provider is unknown", func() {
			It("should return a 404 response", func() {
				resp, _ := apiCall(srv, acquireReq("unknown", "unknown", "unknown", "xxx", 1))
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the provider is known", func() {
			var headSha string
			var priority int
			var acquireResp *http.Response
			var acquireRespBody string

			JustBeforeEach(func() {
				acquireResp, acquireRespBody = apiCall(srv, acquireReq(owner, repo, baseRef, headSha, priority))
			})

			Context("when the lease has not already been acquired", func() {
				Context("if the number of expected request is not reached", func() {
					BeforeEach(func() {
						statuses := map[int]lease.Status{}
						toGenerate := configHelper.DefaultConfigRepoExpectedRequestCount - 2
						for i := 1; i <= toGenerate; i++ {
							statuses[i] = lease.StatusPending
						}
						providerState, opts := generateProviderState(now, owner, repo, baseRef, statuses, nil)
						storage.PrefillStorage(storageDir, providerState)
						clk.SetTime(opts.LastUpdatedAt)

						headSha = fmt.Sprintf("xxx-%d", toGenerate+1)
						priority = toGenerate + 1
					})
					It("the request status should be pending", func() {
						Expect(acquireResp.StatusCode).To(Equal(http.StatusOK))
						expectedPayload := fmt.Sprintf(`{
									"head_sha": "%s",
									"priority": %d,
									"status": "pending"
								}`, headSha, priority)
						Expect(acquireRespBody).To(MatchJSON(expectedPayload))
					})
				})

				Context("if the number of expected request is reached", func() {
					BeforeEach(func() {
						statuses := map[int]lease.Status{}
						toGenerate := configHelper.DefaultConfigRepoExpectedRequestCount - 1
						for i := 1; i <= toGenerate; i++ {
							statuses[i] = lease.StatusPending
						}
						providerState, opts := generateProviderState(now, owner, repo, baseRef, statuses, nil)
						storage.PrefillStorage(storageDir, providerState)
						clk.SetTime(opts.LastUpdatedAt)

						headSha = fmt.Sprintf("xxx-%d", toGenerate+1)
						priority = toGenerate + 1
					})
					It("the request status should be acquired", func() {
						Expect(acquireResp.StatusCode).To(Equal(http.StatusOK))
						expectedPayload := fmt.Sprintf(`{
										"head_sha": "%s",
										"priority": %d,
										"status": "acquired"
									}`, headSha, priority)
						Expect(acquireRespBody).To(MatchJSON(expectedPayload))
					})
				})

				Context("if the stabilize duration has been consumed", func() {
					BeforeEach(func() {
						providerState, opts := generateProviderState(now, owner, repo, baseRef, map[int]lease.Status{
							1: lease.StatusPending,
						}, nil)
						storage.PrefillStorage(storageDir, providerState)
						currentTime := opts.LastUpdatedAt
						currentTime = currentTime.Add(time.Second * (configHelper.DefaultConfigRepoStabilizeDurationSeconds + 1))
						clk.SetTime(currentTime)

						headSha = "xxx-1" //nolint:goconst
						priority = 1
					})
					It("the request status should be acquired", func() {
						Expect(acquireResp.StatusCode).To(Equal(http.StatusOK))
						expectedPayload := fmt.Sprintf(`{
									"head_sha": "%s",
									"priority": %d,
									"status": "acquired"
								}`, headSha, priority)
						Expect(acquireRespBody).To(MatchJSON(expectedPayload))
					})
				})
			})

			Context("when the lease has already been acquired", func() {
				BeforeEach(func() {
					providerState, opts := generateProviderState(now, owner, repo, baseRef, map[int]lease.Status{
						1: lease.StatusPending,
						2: lease.StatusAcquired,
					}, pointer.Int(2))
					storage.PrefillStorage(storageDir, providerState)
					currentTime := opts.LastUpdatedAt
					currentTime = currentTime.Add(time.Second)
					clk.SetTime(currentTime)
				})
				Context("when the incoming lease request is new", func() {
					BeforeEach(func() {
						headSha = "xxx-3"
						priority = 3
					})
					It("the request should be rejected", func() {
						Expect(acquireResp.StatusCode).To(Equal(http.StatusConflict))
					})
				})

				Context("when the incoming lease request is already known", func() {
					BeforeEach(func() {
						headSha = "xxx-1"
						priority = 1
					})

					Context("if the lease owner has completed", func() {
						BeforeEach(func() {
							providerState, opts := generateProviderState(now, owner, repo, baseRef, map[int]lease.Status{
								1: lease.StatusPending,
								2: lease.StatusCompleted,
							}, pointer.Int(2))
							storage.PrefillStorage(storageDir, providerState)
							currentTime := opts.LastUpdatedAt
							currentTime = currentTime.Add(time.Second)
							clk.SetTime(currentTime)
						})
						It("the request status should be completed", func() {
							Expect(acquireResp.StatusCode).To(Equal(http.StatusOK))
							expectedPayload := fmt.Sprintf(`{
												"head_sha": "%s",
												"priority": %d,
												"status": "completed"
											}`, headSha, priority)
							Expect(acquireRespBody).To(MatchJSON(expectedPayload))
						})
					})

					Context("if the lease owner has not failed nor succeed yet", func() {
						It("the request status should continue to be pending", func() {
							Expect(acquireResp.StatusCode).To(Equal(http.StatusOK))
							expectedPayload := fmt.Sprintf(`{
													"head_sha": "%s",
													"priority": %d,
													"status": "pending"
												}`, headSha, priority)
							Expect(acquireRespBody).To(MatchJSON(expectedPayload))
						})
					})
				})
			})
		})
	})

	Describe("Release endpoint", func() {
		BeforeEach(func() {
			clk.SetTime(now)
		})

		Context("when the provider is unknown", func() {
			It("should return a 404 response", func() {
				resp, _ := apiCall(srv, releaseReq("unknown", "unknown", "unknown", "xxx", 1, lease.StatusSuccess))
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the provider is known", func() {
			var headSha string
			var priority int
			var status string
			var releaseResp *http.Response
			var releaseRespBody string

			JustBeforeEach(func() {
				releaseResp, releaseRespBody = apiCall(srv, releaseReq(owner, repo, baseRef, headSha, priority, status))
			})

			Context("when the lease has not already been acquired", func() {
				BeforeEach(func() {
					headSha = "xxx-1"
					priority = 1
					status = lease.StatusSuccess
				})
				It("should reject the release request", func() {
					Expect(releaseResp.StatusCode).To(Equal(http.StatusBadRequest))
				})
			})

			Context("when the lease has been previously acquired", func() {
				BeforeEach(func() {
					providerState, opts := generateProviderState(now, owner, repo, baseRef, map[int]lease.Status{
						1: lease.StatusPending,
						2: lease.StatusAcquired,
					}, pointer.Int(2))
					storage.PrefillStorage(storageDir, providerState)
					currentTime := opts.LastUpdatedAt
					currentTime = currentTime.Add(time.Second)
					clk.SetTime(currentTime)
				})

				Context("when the current request is not the lease owner", func() {
					BeforeEach(func() {
						headSha = "xxx-1"
						priority = 1
						status = lease.StatusSuccess
					})
					It("should reject the release request", func() {
						Expect(releaseResp.StatusCode).To(Equal(http.StatusBadRequest))
					})
				})

				Context("when the current request is the lease owner", func() {
					BeforeEach(func() {
						headSha = "xxx-2"
						priority = 2
					})

					Context("and the reported status is a success", func() {
						BeforeEach(func() {
							status = lease.StatusSuccess
						})
						It("should transition the release request to completed", func() {
							Expect(releaseResp.StatusCode).To(Equal(http.StatusOK))
							expectedPayload := fmt.Sprintf(`{
									"head_sha": "%s",
									"priority": %d,
									"status": "completed"
								}`, headSha, priority)
							Expect(releaseRespBody).To(MatchJSON(expectedPayload))
						})
					})

					Context("and the reported status is a failure", func() {
						BeforeEach(func() {
							status = lease.StatusFailure
						})
						It("should not transition the release request to failed", func() {
							Expect(releaseResp.StatusCode).To(Equal(http.StatusOK))
							expectedPayload := fmt.Sprintf(`{
									"head_sha": "%s",
									"priority": %d,
									"status": "failure"
								}`, headSha, priority)
							Expect(releaseRespBody).To(MatchJSON(expectedPayload))
						})
					})
				})
			})
		})
	})

	Describe("Complete flow", func() {
		Context("stabilize reached, Success build", func() {
			BeforeEach(func() {
				clk.SetTime(now)
			})

			It("should complete the flow successfully", func() {
				By("test acquire, request 1 => should be pending", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-1", 1))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-1",
									"priority": 1,
									"status": "pending"
								}`))
				})
				By("test acquire, request 2 => should be pending", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-2", 2))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-2",
									"priority": 2,
									"status": "pending"
								}`))
				})
				By("sleeping for stabilize duration", func() {
					currentTime := now
					currentTime = currentTime.Add(time.Second * (configHelper.DefaultConfigRepoStabilizeDurationSeconds + 1))
					clk.SetTime(currentTime)
				})
				By("test acquire, request 1 => should be pending", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-1", 1))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-1",
									"priority": 1,
									"status": "pending"
								}`))
				})
				By("test acquire, request 1 => should be acquired", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-2", 2))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-2",
									"priority": 2,
									"status": "acquired"
								}`))
				})
				By("test release (success), request 2 => should be completed", func() {
					resp, body := apiCall(srv, releaseReq(owner, repo, baseRef, "xxx-2", 2, lease.StatusSuccess))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-2",
									"priority": 2,
									"status": "completed"
								}`))
				})
				By("test acquire, request 1 => should be completed", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-1", 1))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-1",
									"priority": 1,
									"status": "completed"
								}`))
				})
			})
		})

		Context("stabilize reached, Failed build", func() {
			BeforeEach(func() {
				clk.SetTime(now)
			})

			It("should complete the flow successfully", func() {
				By("test acquire, request 1 => should be pending", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-1", 1))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-1",
									"priority": 1,
									"status": "pending"
								}`))
				})
				By("test acquire, request 2 => should be pending", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-2", 2))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-2",
									"priority": 2,
									"status": "pending"
								}`))
				})
				By("sleeping for stabilize duration", func() {
					currentTime := now
					currentTime = currentTime.Add(time.Second * (configHelper.DefaultConfigRepoStabilizeDurationSeconds + 1))
					clk.SetTime(currentTime)
				})
				By("test acquire, request 1 => should be pending", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-1", 1))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-1",
									"priority": 1,
									"status": "pending"
								}`))
				})
				By("test acquire, request 2 => should be acquired", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-2", 2))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-2",
									"priority": 2,
									"status": "acquired"
								}`))
				})
				By("test release (failure), request 2 => should be failure", func() {
					resp, body := apiCall(srv, releaseReq(owner, repo, baseRef, "xxx-2", 2, lease.StatusFailure))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-2",
									"priority": 2,
									"status": "failure"
								}`))
				})
				By("test acquire, request 1 => should be acquired", func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-1", 1))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(`{
									"head_sha": "xxx-1",
									"priority": 1,
									"status": "acquired"
								}`))
				})
			})
		})
	})

	Context("maximum request reached, Success build", func() {
		BeforeEach(func() {
			clk.SetTime(now)
		})

		It("should complete the flow successfully", func() {
			max := configHelper.DefaultConfigRepoExpectedRequestCount
			for i := 1; i <= max-1; i++ {
				By(fmt.Sprintf("test acquire, request %d => should be pending", i), func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(i), i))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "pending"
								}`, i, i)))
				})
			}
			By(fmt.Sprintf("test acquire, request %d => should be acquired", max), func() {
				resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(max), max))
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "acquired"
								}`, max, max)))
			})
			By(fmt.Sprintf("test release (success), request %d => should be completed", max), func() {
				resp, body := apiCall(srv, releaseReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(max), max, lease.StatusSuccess))
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "completed"
								}`, max, max)))
			})
			for i := 1; i <= max-1; i++ {
				By(fmt.Sprintf("test acquire, request %d => should be completed", i), func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(i), i))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "completed"
								}`, i, i)))
				})
			}
		})
	})

	Context("maximum request reached, Failed build", func() {
		BeforeEach(func() {
			clk.SetTime(now)
		})

		It("should complete the flow successfully", func() {
			max := configHelper.DefaultConfigRepoExpectedRequestCount
			for i := 1; i <= max-1; i++ {
				By(fmt.Sprintf("test acquire, request %d => should be pending", i), func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(i), i))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "pending"
								}`, i, i)))
				})
			}
			By(fmt.Sprintf("test acquire, request %d => should be acquired", max), func() {
				resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(max), max))
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "acquired"
								}`, max, max)))
			})
			By(fmt.Sprintf("test release (failure), request %d => should be failure", max), func() {
				resp, body := apiCall(srv, releaseReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(max), max, lease.StatusFailure))
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "failure"
								}`, max, max)))
			})
			for i := 1; i <= max-2; i++ {
				By(fmt.Sprintf("test acquire, request %d => should be pending", i), func() {
					resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(i), i))
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "pending"
								}`, i, i)))
				})
			}
			By(fmt.Sprintf("test acquire, request %d => should be acquired", max-1), func() {
				resp, body := apiCall(srv, acquireReq(owner, repo, baseRef, "xxx-"+strconv.Itoa(max-1), max-1))
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(body).To(MatchJSON(fmt.Sprintf(`{
									"head_sha": "xxx-%d",
									"priority": %d,
									"status": "acquired"
								}`, max-1, max-1)))
			})
		})
	})
})

// providerListReq returns pre-configured request for the "GET /" endpoint
func providerListReq() *http.Request {
	return httptest.NewRequest(
		"GET",
		"/",
		nil,
	)
}

// providerDetailsReq returns a pre-configured request for the "GET /:owner/:repo/:baseRef" endpoint
func providerDetailsReq(owner string, repo string, baseRef string) *http.Request {
	return httptest.NewRequest(
		"GET",
		fmt.Sprintf("/%s/%s/%s", owner, repo, baseRef),
		nil,
	)
}

// acquireReq returns a pre-configured request for the "POST /:owner/:repo/:baseRef/acquire" endpoint
func acquireReq(owner string, repo string, baseRef string, headSha string, priority int) *http.Request {
	req := httptest.NewRequest(
		"POST",
		fmt.Sprintf("/%s/%s/%s/acquire", owner, repo, baseRef),
		strings.NewReader(fmt.Sprintf(`{"head_sha": "%s", "priority": %d}`, headSha, priority)),
	)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// releaseReq returns a pre-configured request for the "POST /:owner/:repo/:baseRef/release" endpoint
func releaseReq(owner string, repo string, baseRef string, headSha string, priority int, status string) *http.Request {
	req := httptest.NewRequest(
		"POST",
		fmt.Sprintf("/%s/%s/%s/release", owner, repo, baseRef),
		strings.NewReader(fmt.Sprintf(`{"head_sha": "%s", "priority": %d, "status": "%s"}`, headSha, priority, status)),
	)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// apiCall is simulating an API call to the server (using the provided http request).
// note that it is not calling a standalone server, but hooking into the fiber app directly, using their app.Test() method.
func apiCall(srv server.Server, req *http.Request) (resp *http.Response, body string) {
	var err error
	resp, err = srv.Test(req)
	Expect(err).To(BeNil())

	data, err := io.ReadAll(resp.Body)
	Expect(err).To(BeNil())
	body = string(data)

	GinkgoWriter.Printf("[%s %s] %d %s\n", req.Method, req.URL.Path, resp.StatusCode, body)

	return resp, body
}

// generateProviderState ease the generation of a lease.ProviderState object, which can be then feed into the storage helper
// to inject a know state in the storage before running the test case.
func generateProviderState(now time.Time, owner string, repo string, baseRef string, releaseStatus map[int]lease.Status, acquired *int) (*lease.ProviderState, *lease.NewProviderStateOpts) {
	currentTime := now
	known := map[string]*lease.Request{}
	var acquiredLeaseRequest *lease.Request
	for i, status := range releaseStatus {
		currentTime = currentTime.Add(time.Second * 2)
		sha := "xxx-" + strconv.Itoa(i)
		known[sha] = &lease.Request{
			HeadSHA:  sha,
			Priority: i,
			Status:   pointer.String(string(status)),
		}
		known[sha].UpdateLastSeenAt(currentTime)
		if acquired != nil && i == *acquired {
			acquiredLeaseRequest = known[sha]
		}
	}

	opts := lease.NewProviderStateOpts{
		ID:            fmt.Sprintf("%s:%s:%s", owner, repo, baseRef),
		LastUpdatedAt: currentTime,
		Acquired:      acquiredLeaseRequest,
		Known:         known,
	}
	return lease.NewProviderState(opts), &opts
}

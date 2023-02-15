# gh-action-mq-lease-service

THis service is an extension to the GH merge queue. It's resposible for having a distributed mutex with priority across a set of active merge groups.
One use-case is to coordinate a deployment to a single environemtn being a "funnel".


## Components

### HTTP API
- Implementing lease logic
- Assiging leases to requests for one repository

### CLI 
- Providing a CLI to interact with the HTTP API
- Blocking until a lease is aquired
- reports the priority (number of commits ahead of the head of the merge queue) to the HTTP API
- Fails or exits gracefully based on the HTTP API response (aka if the Github Action Run acquired the lease or lost it and another one succeeded)

### GH Merge Queue
- Providing stacked PR candidates for more efficient and grouped deployments


### GH Action run
1. Aquire the lease (blocking) `<owner>/<repo>/<baseRef>/aquire`
2. Deploy to Preprod
3. Trigger the E2E tests
4. Wait for results
5. Release the lease (success/failure)
   1. Success: exit all other runs
   2. Failure: pass the lease to the next one waiting

```
<5>
<4>  <--- Gets the lease .... Fails
<3>
<2>
<1>


<5>
<4>  <--- discarded by GH merge queue
<3>  <--- Gets the lease (passed to the next one)
<2>
<1>


<5 -> 4> <--- starts waiting for the lease, can't register as a lease-holder is already active
<3>      <--- Got lease
<2>
<1>


<5 -> 4> <--- gets the lease
<3>      <--- completed
<2>
<1>
```

## HTTP API
Config options (default):
- `--port` (8080)
- `--stabilisation-window` (5m) - time to wait before giving out a lease without all expected PRs being in the merge queue
- `--ttl` (15s) - time to wait before considering an aquire interest being stale
- `--expected-pr-count` (4) - number of PRs expected to be in the merge queue for a given baseRef


- GET `/healthz` (health check)
- GET `/metrics` (Prometheus metrics per repo)
- GET `/:owner/:repo/<baseRef>/aquire`
- GET `/:owner/:repo/<baseRef>/release`
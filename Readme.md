# gh-action-mq-lease-service
> A priority mutex with stabilisation window and TTLs, designed to work with the Github MergeQueue accessing a shared resource

## Components

### LeaseProvider
The LeaseProvider is a server that provides the ability to manage distributed leases among multiple github action runs, letting the highest priority run _win_ the lease. This process is helpful when there are multiple runs running that need access to a shared resource. It allows them to agree on the _winner_ of a race for the resource, and subsequently provide the _winner_ with a lease until it is released.
Depending on the release status (success/failure), the lease is completed and confirmation is awaited or the request from the failing lease is discarded and the process restarts.

It exposes two endpoints:
- POST `/:owner/:repo/:baseRef/aquire` for aquiring a lease (poll until status is aquired or completed)
- POST `/:owner/:repo/:baseRef/release` for releasing a lease (the winnder informs the LeaseProvider with the end result)

The payload and response (_LeaseRequest_) is encoded as JSON and follows this scheme:
```jsonnet
{
  "head_sha": "...",
  "priority" 0,
  "status": "(optional) pending|aquired|failure|success|completed"
}
```

#### STM of status transformations
```mermaid
stateDiagram-v2
    [*] --> PENDING: register the LeaseRequest
    PENDING --> AQUIRED: LeaseRequest is the winner
    PENDING --> COMPLETED: LeaseProvider completed with status success
    COMPLETED --> [*]: Discard the LeaseRequest
    AQUIRED --> SUCCESS: the LeaseRequest is released (success)
    AQUIRED --> FAILURE: the leaseRequest is relesed (failure)
    SUCCESS --> COMPLETED: Update LeaseRequest state
    FAILURE --> [*]: the LeaseRequest is discarded
```

#### Sequence diagram of a successful run
> Note: assuming a number of 3 parallel builds
```mermaid
sequenceDiagram
    participant LeaseProvider
    participant GHA1
    participant GHA2
    participant GHA3

    GHA3->>+LeaseProvider: Aquire: priority: 3 
    note right of LeaseProvider: No full state awareness (yet)
    LeaseProvider-->>-GHA3: priority: 2, status: PENDING
    
    par
    loop until aquired lease is released or aquired
    GHA1->>+LeaseProvider: Aquire: priority: 1
    LeaseProvider-->>-GHA1: priority: 1, status: PENDING
    end
    
    loop until aquired lease is released or aquired
    GHA2->>+LeaseProvider: Aquire: priority: 2
    LeaseProvider-->>-GHA2: priority: 2, status: PENDING
    end
    

    rect rgb(191, 223, 255)
    GHA3->>+LeaseProvider: Aquire: priority:3 
    note right of LeaseProvider: Full state awareness 
    LeaseProvider-->>GHA3: priority: 3, status: AQUIRED
    note left of GHA3: holds lease to access shared resource

    GHA3->>LeaseProvider: Release: priority: 3, status: SUCCESS
    note right of LeaseProvider: the lease is marked as completed -> status is available on the next requests
    LeaseProvider-->>-GHA3: priority: 3, status: COMPLETED
    end
end
    
    GHA1->>+LeaseProvider: Aquire: priority: 1
    LeaseProvider-->>-GHA1: priority: 1, status: COMPLETED

    GHA2->>+LeaseProvider: Aquire: priority: 2
    LeaseProvider-->>-GHA2: priority: 2, status: COMPLETED

```

<details><summary>Additional Sequence Diagrams of different scnearios</summary>

#### Sequence diagram of a failure with a new build coming in right away
> :warning: I see a potential conflict here. It could be that GHA1 or GHA2 causes the failure of GHA3, we might not want to accept new LeaseRequests but handle priority across remaining ones

> Note: Expecting full status of 3 parallel builds and a new build immediately starting after the last one failed (GHA3). Also, this sequence diagram does not cover any parallel calls from github actions.

```mermaid
sequenceDiagram
    participant LeaseProvider
    participant GHA1
    participant GHA2
    participant GHA3
    participant GHA_NEXT

    
    GHA1->>+LeaseProvider: Aquire: priority: 1
    LeaseProvider-->>-GHA1: priority: 1, status: PENDING
    GHA2->>+LeaseProvider: Aquire: priority: 2
    LeaseProvider-->>-GHA2: priority: 2, status: PENDING

    rect rgb(255, 200, 200)
    GHA3->>+LeaseProvider: Aquire: priority:3 
    note right of LeaseProvider: Full state awareness 
    LeaseProvider-->>GHA3: priority: 3, status: AQUIRED
    note left of GHA3: holds lease to access shared resource

    GHA3->>LeaseProvider: Release: priority: 3, status: FAILURE
    note right of LeaseProvider: the lease is removed since it failed
    LeaseProvider-->>-GHA3: priority: 3, status: FAILURE
    end

    note right of GHA1: Assuming not sufficient time has passed for stabilize window
    GHA1->>+LeaseProvider: Aquire: priority: 1
    LeaseProvider-->>-GHA1: priority: 1, status: PENDING
    GHA2->>+LeaseProvider: Aquire: priority: 2
    LeaseProvider-->>-GHA2: priority: 2, status: PENDING

    rect rgb(200, 255,200)
    note over GHA_NEXT: New GHA run started by GH merge queue after GHA3 failed
    GHA_NEXT->>+LeaseProvider: Aquire: priority:3 
    note right of LeaseProvider: Full state awareness 
    LeaseProvider-->>GHA_NEXT: priority: 3, status: AQUIRED
    note left of GHA_NEXT: holds lease to access shared resource

    GHA_NEXT->>LeaseProvider: Release: priority: 3, status: SUCCESS
    note right of LeaseProvider: the lease is marked as completed -> status is available on the next requests
    LeaseProvider-->>-GHA_NEXT: priority: 3, status: COMPLETED
    end

    GHA1->>+LeaseProvider: Aquire: priority: 1
    LeaseProvider-->>-GHA1: priority: 1, status: COMPLETED
    GHA2->>+LeaseProvider: Aquire: priority: 2
    LeaseProvider-->>-GHA2: priority: 2, status: COMPLETED
```

#### Sequence Diagram of a failure and passing the lease to the next LeaseReqeust without a new contendor

```mermaid
sequenceDiagram
    participant LeaseProvider
    participant GHA1
    participant GHA2
    participant GHA3

    
    GHA1->>+LeaseProvider: Aquire: priority: 1
    LeaseProvider-->>-GHA1: priority: 1, status: PENDING
    GHA2->>+LeaseProvider: Aquire: priority: 2
    LeaseProvider-->>-GHA2: priority: 2, status: PENDING

    rect rgb(255, 200, 200)
    GHA3->>+LeaseProvider: Aquire: priority:3 
    note right of LeaseProvider: Full state awareness 
    LeaseProvider-->>GHA3: priority: 3, status: AQUIRED
    note left of GHA3: holds lease to access shared resource

    GHA3->>LeaseProvider: Release: priority: 3, status: FAILURE
    note right of LeaseProvider: the lease is removed since it failed
    LeaseProvider-->>-GHA3: priority: 3, status: FAILURE
    end

    note right of GHA1: Assuming sufficient time has passed for stabilize window

    rect rgb(200, 255,200)

    GHA2->>+LeaseProvider: Aquire: priority: 2
    note right of LeaseProvider: Full state awareness 
    LeaseProvider-->>GHA2: priority: 2, status: AQUIRED
    note left of GHA2: holds lease to access shared resource

    GHA2->>LeaseProvider: Release: priority: 2, status: SUCCESS
    note right of LeaseProvider: the lease is marked as completed -> status is available on the next requests
    LeaseProvider-->>-GHA2: priority: 2, status: COMPLETED
    end

    GHA1->>+LeaseProvider: Aquire: priority: 1
    LeaseProvider-->>-GHA1: priority: 1, status: COMPLETED
```
</detail>

### GithubAction
> :warning: WIP
The GithubAction component of this repo interacts with the LeaseProvider and determines the priority of each run based on the commits ahead of the baseRef.
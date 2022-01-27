# Design and Purpose

A cache is needed for any paginated APIs and returning a consistent set of responses
to HTTP clients is required.  The pagination method allows a caller to set the page
size (e.g., 500) and the page being requested, with 1 being the first.  The response
includes the items, and the total size of the dataset.

Due to this pagintion method, we need to know the total size of the dataset for the
first response.  We could query all the clouddrivers each time, and return the
appropriate data.  However, this would mean we need to sort the list, or query each
clouddriver sequentially, adding latency to the response time.  This would also
impose additional unknown load on the clouddrivers as repeated queries for the same
data will be made for every page. [ turns out we need to sort if we use a shared
cache as well... ]

This design implements a simple, non-blocking, per-user per-query in-memory cache.
It efficiently handles multuple queries from the same user for the same data, and
uses no complex locking strategy, instead relying on Go's channels.

## Assumptions:

1. Cache must be per-user, per-query.

2. The increased memory footprint is OK for now.

3. We do not need to run more than one Stormdriver.  If we did, we would have to
impose some ordering of the data returned and probsably use an external shared
cache such as Redis.  Strictly speaking external caches are not needed, if
we are ok with "eventual consistency" in the case where each Stormdriver has
different page contents.

# Implementation

## Structures

cacheRequest:
```go
{
    username     string
    queryURL     string
    page         int
    pageSize     int
    replyChannel chan cacheResponse
}
```

cacheResponse:
```go
{
    totalItems int
    items      []interface{}
}
```

updateResponse:
```go
{
    username string
    queryURL string
    items    []interface{}
}
```

cacheEntry:
```go
{
    username       string
    items          []interface{}
    expiry         int64
    waitingClients []request
}
```

## Cache Expiry

The cache items expire quickly, ideally a few
seconds after the last page is retrieved by
a client.

For cache items that the last page is not retrieved, they are expired in 30 seconds.


# Cache Client

In this context, a "client" is a HTTP request made to Stormdriver, which Stormdriver will aggregate when responding to.

1. Sends a request to the cache.

2. Reads its reply channel.

3. Responds to the HTTP caller.

# Cache Fetcher

1. Queries every clouddriver URL known for the query, using pagination.  Ideally, this would be
done in parallel.  It must include the security context information in outgoing queries.

2. Sends the list of all items found to the update channel of the maintainer.

# Cache Maintainer:

1. Listens on a `request` channel and a `update` channel.

2. Keeps a list of pending updates it has requested.

3. When a new request comes in that has no usable cache entry:
   * Make a cache entry for this query
   * Start a goroutine to perform
the fetch.
   * Add the client's reply channel to the cache item's `waitingClients` list.

4. If a request comes in that has a usable cached entry, use it.

5.  When data is available on the "update" channel, record it, and notify any pending listeners.

6. Periodically clear the cache.

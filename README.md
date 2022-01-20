# Stormdriver
An aggregator front-end for Spinnaker's Clouddriver API.

Stormdriver takes requests from Spinnaker components (orca, igor,
the ui) and forwards them to a Clouddriver most likely to handle that
request.  This allows Clouddrivers to be sharded by account, or
in any other useful way.

When combined with the OpsMX Agent, Stormdriver can securely cross
security boundaries, allowing users to run their own Clouddriver for
their accounts, while a centralize Spinnaker install can use any
of them.

# Status

Currently, only a paritial implementation of the very disjoint
Clouddriver API is implemented.

## Cloud Providers

* AWS should be fully supported.

* Kubernetes should be fully supported.

All others are not.  Adding support is mostly handling the POST
endpoint which mutates infrastructure, and any associated but
not yet implemented GET requests.

## Artifact Accounts

All artifacts should be supported.

## Handling Unknown Requests

For all GET requests, a random (well, the first known) Clouddriver
will get the request, and whatever it replies with will be sent as a
response.  This is probably not useful.

For all PUT, POST, and other modification requests which are not
understood, HTTP status 503 will be returned.  This is to ensure
accidental modifications are not made when we are not sure where
the request should be routed.

# Security

Stormdriver itself does not implement any security.  All the security
comes from what the request contains for user identification, and what
the underlying Clouddrivers are willing to allow.

When Stormdriver receives a request, it will use the X-* headers, the
exact URI, and the exact contents of the body when sending upstream
Clouddriver requests.  This ensures that requests are properly scoped.

No caching is performed.  However, an account must be configued when
using Spinnaker's RBAC that has access to all accounts of all types,
for internal location purposes.  This is the only case where special
permissions are needed.  This allows Stormdriver to track locations of
the various accounts and artifact accounts.

# Clouddriver Accounts

Clouddriver has cloud provider accounts, and artifact accounts.
Internally, these are referred to as "credentials", and are available
on the `/credentials` and `/artifact/credentials` endpoints.
When Stormdriver is asked for these, it will broadcast a request to
all Clouddriver instances, and merge the results into one single
combined result.

# Routing

Stormdriver polls frequently for new accounts and artifact accounts,
and maintains a map of account-name to Clouddriver URL.

When a request includes an account scope, the request is forwarded
to a single Clouddriver instance which we know handles that
account, and the entire response is sent back to the client.  If
no route exists, we will return HTTP status 503 Service Unavailble.

Some requests are not scoped to a specific account, in which case all
Clouddrivers are queried, and the result is a merge of all the
results.  Some are "any response is OK" queries, like the result for
a task's status.  In this case, whoever responds with something other
than 404 is used in the reply.  If no one does, 404 will be returned.

# Performance

Performance should be quite good.  When we need to ask multiple
Clouddrivers and combine the results, we will send the request
to all Clouddrivers in parallel, and merge the results as they arrive.

For Clouddrivers which may be down, return junk, or otherwise are
not healthy, we will ignore the responses when merging.  A timeout
(default 15 seconds) is allowed for the TCP session to be established,
the headers to be provided, and the body to be provided.  This
sets a hard-limit on how long a client must wait for some response.

Memory usage should be very low, as is CPU usage.  During testing
an active Spinnaker and four independent Clouddrivers,
the CPU never exeeded 0.01% of a single core, and memory usage
was around 20 MB.  However, memory usage is directly related to
the size of returned items, such as artifacts.  Currently,
the entire artifact is read into memory when retrieved, rather
than streamed.

# Configuration

See `sample-config.yaml` in the project for a simple sample to
start with.

A Clouddriver stanza has three components:

`name` is the internal named used for printing debugging messages.
It can be anything, but should be unique.

`url` is the endpoint URL to use to contact that clouddriver.
It should generally not end with a slash.

`healthcheck` defaults to `${url}/health` but can be overridden.
A status code of 200 to 399 is considered "healthy", while anything
else, or a timeout, will indicate unhealthy.

# Additional URLs

In addition to all the currently supported Clouddriver URL paths,
three additional endpoints are included for Stormdriver monitoring
and debugging.

* `/_internal/accounts` returns the list of currently known accounts,
both for cloud providers and artifacts.

* `/_internal/accountRoutes` shows the currently known accounts,
and which Clouddriver they will be forwarded to.

* `/health` indicates the health of Stormdriver.  This also 
includes the status of each Clouddriver connection.
While included, if any specific Clouddriver is down or unreachable,
it will be reported here, but will not affect the health indicator
reported by Stormdriver.  The `/health` endpoint is suitable for use
in a Kubernetes or other liveness probe.  It will return 200 if
all the required health checks pass, or 418 if Stormdriver is
unhealthy.

# To Do

* Handle large resposnes without exploding memory usage,
expecially around artifacts.

* Support more endpoints, including `/search` (used by the UI)
and more cloud providers.

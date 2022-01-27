/*
 * Copyright 2022 OpsMx, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License")
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

// PaginatedCache holds the state and data for a cache that uses a specific format
// of pagination.  Specifically, one that follows a model of a list of
// paginated objects, where each member of this list has a page number and
// items.
type PaginatedCache struct {
	// The cache.  Top level key is user+query.  Second level is platform.
	cache       map[string]map[string][]cacheEntry
	updateChan  chan cacheUpdateResponse
	requestChan chan CacheRequest
	stopChan    chan bool
}

// CacheResponse is a reply to a CacheRequest.
type CacheResponse struct {
	TotalMatches int           `json:"totalMatches,omitempty"`
	PageNumber   int           `json:"pageNumber,omitempty"`
	PageSize     int           `json:"pageSize,omitempty"`
	Platform     string        `json:"platform,omitempty"`
	Query        string        `json:"query,omitempty"`
	Results      []interface{} `json:"results,omitempty"`
}

// CacheRequest is a request to receive info from the cache.  After this is
// sent to the cache, the sender should listen on its replyChannel.
// Exactly one reply will be sent per CacheRequest.  The channel will be
// closed by the cache to ensure this.
//
// Typically, this is in response to a HTTP request, and the format of the query
// generally maps into this structure's shape.
type CacheRequest struct {
	Username     string
	Query        string
	PageNumber   int
	PageSize     int
	replyChannel chan CacheResponse
}

type cacheUpdateResponse struct {
	username string
	query    string
	items    []interface{}
}

// cacheEntry holds the data for a single query, scoped to the user by design.
// States:
//   If waitingClients is not empty, we have a fetch running.
//   If waitingClients is empty, items is valid (even if empty), and we have no fetches running.
type cacheEntry struct {
	username       string
	items          []interface{}
	expiry         int64
	waitingClients []CacheRequest
}

func (c *PaginatedCache) runCache() {

}

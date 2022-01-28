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

import "fmt"

// PaginatedCache holds the state and data for a cache that uses a specific format
// of pagination.  Specifically, one that follows a model of a list of
// paginated objects, where each member of this list has a page number and
// items.
type PaginatedCache struct {
	cache       map[string]*cacheEntry
	updateChan  chan cacheUpdateResponse
	requestChan chan CacheRequest
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
	QueryURL     string
	PageNumber   int
	PageSize     int
	ReplyChannel chan CacheResponse
}

// cacheUpdateResponse contains the new data to store in the cache.
// This is a complete update, with full data.
type cacheUpdateResponse struct {
	username string        // copied from the request
	queryURL string        // copied from the request
	query    string        // from clouddriver
	platform string        // from clouddriver
	results  []interface{} // from clouddriver
}

// cacheEntry holds the data for a single query, scoped to the user by design.
// States:
// *  If waitingClients is not empty, we have a fetch running.
// *  If waitingClients is empty, results is valid (even if empty), and
//    we have no fetches running.
type cacheEntry struct {
	results        []interface{} // set from update
	platform       string        // set from update
	query          string        // set from update
	expiry         int64
	waitingClients []*CacheRequest
}

// MakePaginatedCache returns a new cache.
func MakePaginatedCache() *PaginatedCache {
	return &PaginatedCache{
		cache:       map[string]*cacheEntry{},
		updateChan:  make(chan cacheUpdateResponse),
		requestChan: make(chan CacheRequest),
	}
}

// RunCache runs the cache, forever.  Use a goroutine.
func (c *PaginatedCache) RunCache() {
	for {
		select {
		case request := <-c.requestChan:
			key := fmt.Sprintf("%s::%s", request.Username, request.QueryURL)
			entry, found := c.cache[key]
			if !found {
				go c.update(request.Username, request.QueryURL)
				c.cache[key] = &cacheEntry{
					expiry:         0,
					waitingClients: []*CacheRequest{&request},
				}
				continue
			}
			if len(entry.waitingClients) == 0 {
				c.reply(entry, &request)
			} else {
				entry.waitingClients = append(entry.waitingClients, &request)
			}
		case update := <-c.updateChan:
			key := fmt.Sprintf("%s::%s", update.username, update.queryURL)
			c.cache[key].results = update.results
			c.cache[key].platform = update.platform
			c.cache[key].query = update.query
			for _, request := range c.cache[key].waitingClients {
				c.reply(c.cache[key], request)
			}
			c.cache[key].waitingClients = []*CacheRequest{}
		}
	}
}

func (c *PaginatedCache) update(username string, queryURL string) {
	// fire off parallel queries, combine them, and send a reply to the
	// cache runner.

	c.updateChan <- cacheUpdateResponse{
		username: username,
		queryURL: queryURL,
		query:    "TODO",
		platform: "TODO",
		results:  []interface{}{},
	}
}

func (c *PaginatedCache) reply(entry *cacheEntry, request *CacheRequest) {
	startOffset := request.PageNumber * request.PageSize
	endOffset := startOffset + request.PageSize

	totalMatches := len(entry.results)

	reply := CacheResponse{
		TotalMatches: totalMatches,
		PageNumber:   request.PageNumber,
		PageSize:     request.PageSize,
		Platform:     entry.platform,
		Query:        entry.query,
	}

	if startOffset >= totalMatches {
		reply.Results = []interface{}{}
	} else {
		if endOffset > totalMatches {
			endOffset = totalMatches
		}
		reply.Results = entry.results[startOffset:endOffset]
	}
	request.ReplyChannel <- reply
}

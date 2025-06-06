// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package users

import (
	"os/user"
	"sync"
	"time"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/lib/lang"
	"oss.indeed.com/go/libtime"
)

const (
	cacheTTL   = 1 * time.Hour
	failureTTL = 1 * time.Minute
)

type entry[T any] lang.Pair[T, time.Time]

func (e *entry[T]) expired(now time.Time, ttl time.Duration) bool {
	return now.After(e.Second.Add(ttl))
}

type (
	userCache        map[string]*entry[*user.User]
	userFailureCache map[string]*entry[error]
)

type lookupUserFunc func(string) (*user.User, error)

type cache struct {
	clock      libtime.Clock
	lookupUser lookupUserFunc

	lock         sync.Mutex
	users        userCache
	userFailures userFailureCache
	logger       hclog.Logger // <-- Add this line
}

func newCache() *cache {
	return &cache{
		clock:        libtime.SystemClock(),
		lookupUser:   internalLookupUser,
		users:        make(userCache),
		userFailures: make(userFailureCache),
		logger:       hclog.L().Named("user-cache"),
	}
}

func (c *cache) GetUser(username string) (*user.User, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// record this moment as "now" for further cache operations
	now := c.clock.Now()
	c.logger.Info("GetUser:",username)
	// first check if the user is in the cache and the entry we have
	// is not yet expired
	usr, exists := c.users[username]
	if exists && !usr.expired(now, cacheTTL) {
		return usr.First, nil
	}
	c.logger.Info("User not found in cache, performing lookup:", "username", username)
	// next check if there was a recent failure already, so we
	// avoid spamming the OS with dead user lookups
	failure, exists2 := c.userFailures[username]
	c.logger.Info("Checking user failures:", "username", username, "exists", exists2)
	if exists2 {
		if !failure.expired(now, failureTTL) {
			return nil, failure.First
		}
		// may as well cleanup expired case
		delete(c.userFailures, username)
	}

	// need to perform an OS lookup
	u, err := c.lookupUser(username)

	// lookup was a failure, populate the failure cache
	if err != nil {
		c.userFailures[username] = &entry[error]{err, now}
		return nil, err
	}

	// lookup was a success, populate the user cache
	c.users[username] = &entry[*user.User]{u, now}
	c.logger.Info("Cached users:")
    for username, entry := range c.users {
        c.logger.Info("user", "username", username, "uid", entry.First.Uid, "gid", entry.First.Gid, "home", entry.First.HomeDir)
    }
	c.logger.Info("--------")
	return u, nil
}

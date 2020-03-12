package auth

import (
	auth "k8s.io/api/authentication/v1beta1"
	"sync"
	"time"
)

//
// Singleton (global) token cache.
var tokenCache = TokenCache{
	content: map[string]*CachedToken{},
	ttl:     time.Second * 10,
}

//
// Cache of tokens.
type CachedToken struct {
	token   string
	created time.Time
	review  *auth.TokenReview
}

type TokenCache struct {
	// Mutex.
	mutex sync.RWMutex
	// Cache content.
	content map[string]*CachedToken
	// Lifespan (time-to-live).
	ttl time.Duration
}

//
// Add token to the cache.
func (r *TokenCache) Add(token string, tr *auth.TokenReview) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	r.content[token] = &CachedToken{
		token:   token,
		created: time.Now(),
		review:  tr,
	}
}

//
// Get a cached token (review).
func (r *TokenCache) Get(token string) (tr *auth.TokenReview, found bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	r.Evict()
	entry, found := r.content[token]
	if found {
		tr = entry.review
	}

	return tr, found
}

//
// Evict expired cache entries.
func (r *TokenCache) Evict() {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	evicted := []string{}
	for token, entry := range r.content {
		if time.Since(entry.created) > r.ttl {
			evicted = append(evicted, token)
		}
	}
	for _, token := range evicted {
		delete(r.content, token)
	}
}

// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package store

import (
	"sync"

	"github.com/ovh/go-ovh/ovh"
)

// ovhClientPool hands out OVH API clients, one per concurrent request.
//
// A single *ovh.Client cannot be shared between goroutines: go-ovh assigns the client
// wide timeout to the embedded http.Client while building every request, which races
// with the requests already in flight on that same http.Client. Both the search and
// the kubeconfig retrieval call the OVH API in parallel, so each of them borrows a
// client for the duration of a single request and returns it afterwards.
//
// The pool grows to the peak number of concurrent requests and then reuses its
// clients, so the connection pool and the API time delta of a client are not
// negotiated again for every call.
type ovhClientPool struct {
	newClient func() (*ovh.Client, error)

	mutex sync.Mutex
	idle  []*ovh.Client
}

func newOVHClientPool(newClient func() (*ovh.Client, error)) *ovhClientPool {
	return &ovhClientPool{newClient: newClient}
}

// acquire borrows an idle client, creating one when the pool is empty.
func (p *ovhClientPool) acquire() (*ovh.Client, error) {
	p.mutex.Lock()
	if last := len(p.idle) - 1; last >= 0 {
		client := p.idle[last]
		p.idle = p.idle[:last]
		p.mutex.Unlock()
		return client, nil
	}
	p.mutex.Unlock()

	return p.newClient()
}

// release returns a client to the pool.
func (p *ovhClientPool) release(client *ovh.Client) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.idle = append(p.idle, client)
}

// get performs an authenticated GET with a borrowed client.
func (p *ovhClientPool) get(url string, result any) error {
	client, err := p.acquire()
	if err != nil {
		return err
	}
	defer p.release(client)

	return client.Get(url, result)
}

// post performs an authenticated POST with a borrowed client.
func (p *ovhClientPool) post(url string, body, result any) error {
	client, err := p.acquire()
	if err != nil {
		return err
	}
	defer p.release(client)

	return client.Post(url, body, result)
}

/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package servers

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
)

var _ = Describe("PrivateEventsServer processEvent", func() {
	It("Does not block when one subscriber is slow", func() {
		server := &PrivateEventsServer{
			logger:   logger,
			subs:     map[string]privateEventsServerSubInfo{},
			subsLock: &sync.RWMutex{},
		}

		slowChan := make(chan *privatev1.Event, eventsChanBufferSize)
		fastChan := make(chan *privatev1.Event, eventsChanBufferSize)

		server.subs["slow"] = privateEventsServerSubInfo{
			eventsChan: slowChan,
		}
		server.subs["fast"] = privateEventsServerSubInfo{
			eventsChan: fastChan,
		}

		event := &privatev1.Event{}
		event.SetId("test-event")
		event.SetType(privatev1.EventType_EVENT_TYPE_OBJECT_UPDATED)

		// Call processEvent — it should return quickly even though nobody is reading from
		// either channel, because channels are buffered and sends are non-blocking.
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			_ = server.processEvent(context.Background(), event)
			close(done)
		}()

		Eventually(done, 2*time.Second).Should(BeClosed())

		// Both subscribers should have the event in their channel.
		Eventually(fastChan).Should(Receive())
		Eventually(slowChan).Should(Receive())
	})

	It("Drops events for a subscriber whose buffer is full", func() {
		server := &PrivateEventsServer{
			logger:   logger,
			subs:     map[string]privateEventsServerSubInfo{},
			subsLock: &sync.RWMutex{},
		}

		fullChan := make(chan *privatev1.Event, eventsChanBufferSize)
		healthyChan := make(chan *privatev1.Event, eventsChanBufferSize)

		// Fill the slow subscriber's buffer completely.
		for range eventsChanBufferSize {
			fullChan <- &privatev1.Event{}
		}

		server.subs["full"] = privateEventsServerSubInfo{
			eventsChan: fullChan,
		}
		server.subs["healthy"] = privateEventsServerSubInfo{
			eventsChan: healthyChan,
		}

		event := &privatev1.Event{}
		event.SetId("overflow-event")
		event.SetType(privatev1.EventType_EVENT_TYPE_OBJECT_UPDATED)

		// processEvent should still return quickly — the full subscriber's event is dropped.
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			_ = server.processEvent(context.Background(), event)
			close(done)
		}()

		Eventually(done, 2*time.Second).Should(BeClosed())

		// The healthy subscriber should have the event.
		Eventually(healthyChan).Should(Receive())

		// The full subscriber's channel should still be at capacity (overflow event was dropped).
		Expect(fullChan).To(HaveLen(eventsChanBufferSize))
	})
})

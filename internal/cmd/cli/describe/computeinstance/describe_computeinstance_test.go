/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package computeinstance

import (
	"bytes"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/timestamppb"

	publicv1 "github.com/osac-project/fulfillment-service/internal/api/osac/public/v1"
)

// formatComputeInstance formats a compute instance for display using RenderComputeInstance.
func formatComputeInstance(ci *publicv1.ComputeInstance) string {
	var buf bytes.Buffer
	RenderComputeInstance(&buf, ci)
	return buf.String()
}

var _ = Describe("Describe Compute Instance", func() {
	It("should display last_restarted_at when set", func() {
		restartTime := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
		ci := &publicv1.ComputeInstance{
			Id: "ci-test-001",
			Spec: &publicv1.ComputeInstanceSpec{
				Template: "tpl-small-001",
			},
			Status: &publicv1.ComputeInstanceStatus{
				State:           publicv1.ComputeInstanceState_COMPUTE_INSTANCE_STATE_RUNNING,
				LastRestartedAt: timestamppb.New(restartTime),
			},
		}

		output := formatComputeInstance(ci)
		Expect(output).To(ContainSubstring("Last Restarted At:"))
		Expect(output).To(ContainSubstring("2026-03-15T10:30:00Z"))
	})

	It("should omit last_restarted_at when not set", func() {
		ci := &publicv1.ComputeInstance{
			Id: "ci-test-002",
			Spec: &publicv1.ComputeInstanceSpec{
				Template: "tpl-small-001",
			},
			Status: &publicv1.ComputeInstanceStatus{
				State: publicv1.ComputeInstanceState_COMPUTE_INSTANCE_STATE_RUNNING,
			},
		}

		output := formatComputeInstance(ci)
		Expect(output).NotTo(ContainSubstring("Last Restarted At:"))
	})

	It("should omit last_restarted_at when status is nil", func() {
		ci := &publicv1.ComputeInstance{
			Id: "ci-test-003",
			Spec: &publicv1.ComputeInstanceSpec{
				Template: "tpl-small-001",
			},
		}

		output := formatComputeInstance(ci)
		Expect(output).To(MatchRegexp(`State:\s+-`))
		Expect(output).NotTo(ContainSubstring("Last Restarted At:"))
	})
})

var _ = Describe("CEL filter construction", func() {
	It("should produce valid CEL with == operator and quoted value for a plain name", func() {
		filter := buildFilter("my-instance")
		Expect(filter).To(Equal(`this.id == "my-instance" || this.metadata.name == "my-instance"`))
	})

	It("should escape double quotes in the reference value", func() {
		filter := buildFilter(`my"instance`)
		Expect(filter).To(ContainSubstring(`"my\"instance"`))
	})

	It("should escape backslashes in the reference value", func() {
		filter := buildFilter(`my\instance`)
		Expect(filter).To(ContainSubstring(`"my\\instance"`))
	})

	It("should produce valid CEL for a UUID-style ID", func() {
		filter := buildFilter("550e8400-e29b-41d4-a716-446655440000")
		Expect(filter).To(Equal(`this.id == "550e8400-e29b-41d4-a716-446655440000" || this.metadata.name == "550e8400-e29b-41d4-a716-446655440000"`))
	})
})

var _ = Describe("Multi-result guard", func() {
	It("should produce the expected error message", func() {
		ref := "ambiguous-name"
		err := fmt.Errorf("multiple compute instances match '%s', use the ID instead", ref)
		Expect(err.Error()).To(ContainSubstring("multiple compute instances match"))
		Expect(err.Error()).To(ContainSubstring("ambiguous-name"))
		Expect(err.Error()).To(ContainSubstring("use the ID instead"))
	})
})

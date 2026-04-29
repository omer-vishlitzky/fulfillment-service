/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package securitygroup

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/controllers/finalizers"
)

var _ = Describe("buildSpec", func() {
	It("Includes virtualNetwork and rules", func() {
		portFrom := int32(80)
		portTo := int32(443)
		ipv4 := "10.0.0.0/8"
		ipv6 := "2001:db8::/32"

		t := &task{
			securityGroup: privatev1.SecurityGroup_builder{
				Id: "sg-test-123",
				Spec: privatev1.SecurityGroupSpec_builder{
					VirtualNetwork: "vnet-123",
					Ingress: []*privatev1.SecurityRule{
						privatev1.SecurityRule_builder{
							Protocol: privatev1.Protocol_PROTOCOL_TCP,
							PortFrom: &portFrom,
							PortTo:   &portTo,
							Ipv4Cidr: &ipv4,
						}.Build(),
					},
					Egress: []*privatev1.SecurityRule{
						privatev1.SecurityRule_builder{
							Protocol: privatev1.Protocol_PROTOCOL_ALL,
							Ipv6Cidr: &ipv6,
						}.Build(),
					},
				}.Build(),
			}.Build(),
		}

		spec := t.buildSpec()

		Expect(spec.VirtualNetwork).To(Equal("vnet-123"))

		Expect(spec.IngressRules).To(HaveLen(1))
		Expect(string(spec.IngressRules[0].Protocol)).To(Equal("tcp"))
		Expect(*spec.IngressRules[0].PortFrom).To(Equal(int32(80)))
		Expect(*spec.IngressRules[0].PortTo).To(Equal(int32(443)))
		Expect(spec.IngressRules[0].SourceCIDR).To(Equal("10.0.0.0/8"))

		Expect(spec.EgressRules).To(HaveLen(1))
		Expect(string(spec.EgressRules[0].Protocol)).To(Equal("all"))
		Expect(spec.EgressRules[0].DestinationCIDR).To(Equal("2001:db8::/32"))
		Expect(spec.EgressRules[0].PortFrom).To(BeNil())
		Expect(spec.EgressRules[0].PortTo).To(BeNil())
	})

	It("Omits empty rule lists", func() {
		t := &task{
			securityGroup: privatev1.SecurityGroup_builder{
				Id: "sg-test-456",
				Spec: privatev1.SecurityGroupSpec_builder{
					VirtualNetwork: "vnet-456",
				}.Build(),
			}.Build(),
		}

		spec := t.buildSpec()

		Expect(spec.VirtualNetwork).To(Equal("vnet-456"))
		Expect(spec.IngressRules).To(BeEmpty())
		Expect(spec.EgressRules).To(BeEmpty())
	})
})

var _ = Describe("protocolToString", func() {
	It("Converts all protocol values correctly", func() {
		Expect(protocolToString(privatev1.Protocol_PROTOCOL_TCP)).To(Equal("tcp"))
		Expect(protocolToString(privatev1.Protocol_PROTOCOL_UDP)).To(Equal("udp"))
		Expect(protocolToString(privatev1.Protocol_PROTOCOL_ICMP)).To(Equal("icmp"))
		Expect(protocolToString(privatev1.Protocol_PROTOCOL_ALL)).To(Equal("all"))
	})
})

// hasFinalizer checks if the fulfillment-controller finalizer is present on the security group.
func hasFinalizer(sg *privatev1.SecurityGroup) bool {
	return slices.Contains(sg.GetMetadata().GetFinalizers(), finalizers.Controller)
}

var _ = Describe("validateTenant", func() {
	It("should succeed when exactly one tenant is assigned", func() {
		sg := privatev1.SecurityGroup_builder{
			Metadata: privatev1.Metadata_builder{
				Tenants: []string{"tenant-1"},
			}.Build(),
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		err := t.validateTenant()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should fail when no tenants are assigned", func() {
		sg := privatev1.SecurityGroup_builder{
			Metadata: privatev1.Metadata_builder{
				Tenants: []string{},
			}.Build(),
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		err := t.validateTenant()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one tenant"))
	})

	It("should fail when metadata is missing", func() {
		sg := privatev1.SecurityGroup_builder{}.Build()

		t := &task{
			securityGroup: sg,
		}

		err := t.validateTenant()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one tenant"))
	})
})

var _ = Describe("setDefaults", func() {
	It("should set PENDING state when status is unspecified", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-defaults",
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		t.setDefaults()

		Expect(t.securityGroup.GetStatus().GetState()).To(Equal(privatev1.SecurityGroupState_SECURITY_GROUP_STATE_PENDING))
	})

	It("should not overwrite existing state", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-existing-state",
			Status: privatev1.SecurityGroupStatus_builder{
				State: privatev1.SecurityGroupState_SECURITY_GROUP_STATE_READY,
			}.Build(),
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		t.setDefaults()

		Expect(t.securityGroup.GetStatus().GetState()).To(Equal(privatev1.SecurityGroupState_SECURITY_GROUP_STATE_READY))
	})

	It("should create status if it doesn't exist", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-no-status",
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		Expect(t.securityGroup.HasStatus()).To(BeFalse())

		t.setDefaults()

		Expect(t.securityGroup.HasStatus()).To(BeTrue())
		Expect(t.securityGroup.GetStatus().GetState()).To(Equal(privatev1.SecurityGroupState_SECURITY_GROUP_STATE_PENDING))
	})
})

var _ = Describe("addFinalizer", func() {
	It("should add finalizer when not present", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-no-finalizer",
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{},
			}.Build(),
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		added := t.addFinalizer()

		Expect(added).To(BeTrue())
		Expect(hasFinalizer(t.securityGroup)).To(BeTrue())
	})

	It("should not add finalizer when already present", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-has-finalizer",
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
			}.Build(),
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		added := t.addFinalizer()

		Expect(added).To(BeFalse())
		Expect(hasFinalizer(t.securityGroup)).To(BeTrue())
		// Should not duplicate
		Expect(len(t.securityGroup.GetMetadata().GetFinalizers())).To(Equal(1))
	})

	It("should create metadata if it doesn't exist", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-no-metadata",
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		Expect(t.securityGroup.HasMetadata()).To(BeFalse())

		added := t.addFinalizer()

		Expect(added).To(BeTrue())
		Expect(t.securityGroup.HasMetadata()).To(BeTrue())
		Expect(hasFinalizer(t.securityGroup)).To(BeTrue())
	})
})

var _ = Describe("removeFinalizer", func() {
	It("should remove finalizer when present", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-has-finalizer",
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller, "other-finalizer"},
			}.Build(),
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		Expect(hasFinalizer(t.securityGroup)).To(BeTrue())

		t.removeFinalizer()

		Expect(hasFinalizer(t.securityGroup)).To(BeFalse())
		// Other finalizers should remain
		Expect(t.securityGroup.GetMetadata().GetFinalizers()).To(ContainElement("other-finalizer"))
	})

	It("should do nothing when finalizer not present", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-no-finalizer",
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{"other-finalizer"},
			}.Build(),
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		Expect(hasFinalizer(t.securityGroup)).To(BeFalse())

		t.removeFinalizer()

		Expect(hasFinalizer(t.securityGroup)).To(BeFalse())
		Expect(t.securityGroup.GetMetadata().GetFinalizers()).To(ContainElement("other-finalizer"))
	})

	It("should do nothing when metadata doesn't exist", func() {
		sg := privatev1.SecurityGroup_builder{
			Id: "sg-no-metadata",
		}.Build()

		t := &task{
			securityGroup: sg,
		}

		// Should not panic
		t.removeFinalizer()

		Expect(t.securityGroup.HasMetadata()).To(BeFalse())
	})
})

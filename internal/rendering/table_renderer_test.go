/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package rendering

import (
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	publicv1 "github.com/osac-project/fulfillment-service/internal/api/osac/public/v1"
	"github.com/osac-project/fulfillment-service/internal/packages"
	"github.com/osac-project/fulfillment-service/internal/reflection"
	internaltesting "github.com/osac-project/fulfillment-service/internal/testing"
)

var _ = Describe("Table renderer", func() {
	var (
		ctx        context.Context
		server     *internaltesting.Server
		connection *grpc.ClientConn
		helper     *reflection.Helper
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		server = internaltesting.NewServer()
		DeferCleanup(server.Stop)

		connection, err = grpc.NewClient(
			server.Address(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(connection.Close)

		helper, err = reflection.NewHelper().
			SetLogger(logger).
			SetConnection(connection).
			AddPackage(packages.PublicV1, 1).
			Build()
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Lookup columns", func() {
		It("Falls back to the key when the looked-up object has no name", func() {
			// Register a ComputeInstanceTemplates server that returns a template with no name set.
			// This is the regression case for MGMT-23970: the TEMPLATE column was blank when
			// metadata.name was empty.
			publicv1.RegisterComputeInstanceTemplatesServer(
				server.Registrar(),
				&internaltesting.ComputeInstanceTemplatesServerFuncs{
					ListFunc: func(
						ctx context.Context,
						req *publicv1.ComputeInstanceTemplatesListRequest,
					) (*publicv1.ComputeInstanceTemplatesListResponse, error) {
						return publicv1.ComputeInstanceTemplatesListResponse_builder{
							Size:  1,
							Total: 1,
							Items: []*publicv1.ComputeInstanceTemplate{
								publicv1.ComputeInstanceTemplate_builder{
									Id: "osac.templates.ocp_virt_vm",
									// Metadata.Name deliberately left empty.
								}.Build(),
							},
						}.Build(), nil
					},
				},
			)
			server.Start()

			var buf bytes.Buffer
			renderer, err := NewTableRenderer().
				SetLogger(logger).
				SetHelper(helper).
				SetWriter(&buf).
				Build()
			Expect(err).ToNot(HaveOccurred())

			instance := publicv1.ComputeInstance_builder{
				Id: "019d53bd-42b4-7e23-b98e-6368490d3d83",
				Metadata: publicv1.Metadata_builder{
					Name: "test",
				}.Build(),
				Spec: publicv1.ComputeInstanceSpec_builder{
					Template: "osac.templates.ocp_virt_vm",
				}.Build(),
			}.Build()

			err = renderer.Render(ctx, []*publicv1.ComputeInstance{instance})
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("osac.templates.ocp_virt_vm"))
		})

		It("Shows the name when the looked-up object has a name", func() {
			publicv1.RegisterComputeInstanceTemplatesServer(
				server.Registrar(),
				&internaltesting.ComputeInstanceTemplatesServerFuncs{
					ListFunc: func(
						ctx context.Context,
						req *publicv1.ComputeInstanceTemplatesListRequest,
					) (*publicv1.ComputeInstanceTemplatesListResponse, error) {
						return publicv1.ComputeInstanceTemplatesListResponse_builder{
							Size:  1,
							Total: 1,
							Items: []*publicv1.ComputeInstanceTemplate{
								publicv1.ComputeInstanceTemplate_builder{
									Id: "osac.templates.ocp_virt_vm",
									Metadata: publicv1.Metadata_builder{
										Name: "OpenShift Virt VM",
									}.Build(),
								}.Build(),
							},
						}.Build(), nil
					},
				},
			)
			server.Start()

			var buf bytes.Buffer
			renderer, err := NewTableRenderer().
				SetLogger(logger).
				SetHelper(helper).
				SetWriter(&buf).
				Build()
			Expect(err).ToNot(HaveOccurred())

			instance := publicv1.ComputeInstance_builder{
				Id: "019d53bd-42b4-7e23-b98e-6368490d3d83",
				Metadata: publicv1.Metadata_builder{
					Name: "test",
				}.Build(),
				Spec: publicv1.ComputeInstanceSpec_builder{
					Template: "osac.templates.ocp_virt_vm",
				}.Build(),
			}.Build()

			err = renderer.Render(ctx, []*publicv1.ComputeInstance{instance})
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("OpenShift Virt VM"))
		})
	})
})

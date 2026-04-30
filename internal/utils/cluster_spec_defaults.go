/*
Copyright (c) 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package utils

import (
	"net"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
)

// ApplyClusterSpecDefaults applies default values from a template's spec_defaults to a cluster spec.
//
// User-provided values have precedence over defaults, and should never be overridden by defaults.
func ApplyClusterSpecDefaults(spec *privatev1.ClusterSpec, defaults *privatev1.ClusterTemplateSpecDefaults) {
	if spec == nil || defaults == nil {
		return
	}
	if !spec.HasPullSecret() && defaults.HasPullSecret() {
		spec.SetPullSecret(defaults.GetPullSecret())
	}
	if !spec.HasSshPublicKey() && defaults.HasSshPublicKey() {
		spec.SetSshPublicKey(defaults.GetSshPublicKey())
	}
	if !spec.HasReleaseImage() && defaults.HasReleaseImage() {
		spec.SetReleaseImage(defaults.GetReleaseImage())
	}
	mergeClusterNetworkDefaults(spec, defaults)
}

func mergeClusterNetworkDefaults(spec *privatev1.ClusterSpec, defaults *privatev1.ClusterTemplateSpecDefaults) {
	if !defaults.HasNetwork() {
		return
	}
	if !spec.HasNetwork() {
		spec.SetNetwork(proto.Clone(defaults.GetNetwork()).(*privatev1.ClusterNetwork))
		return
	}
	specNet := spec.GetNetwork()
	defNet := defaults.GetNetwork()
	if !specNet.HasPodCidr() && defNet.HasPodCidr() {
		specNet.SetPodCidr(defNet.GetPodCidr())
	}
	if !specNet.HasServiceCidr() && defNet.HasServiceCidr() {
		specNet.SetServiceCidr(defNet.GetServiceCidr())
	}
}

// ValidateClusterSpecFields validates the format of cluster spec fields that are present.
// Unlike ComputeInstance, cluster credentials (pull_secret, ssh_public_key) are not required
// at API time — the Ansible role can fall back to a provider default Secret.
func ValidateClusterSpecFields(spec *privatev1.ClusterSpec) error {
	if spec == nil {
		return nil
	}

	// Validate CIDR format if provided:
	if spec.HasNetwork() {
		if err := validateClusterNetwork(spec.GetNetwork()); err != nil {
			return err
		}
	}

	return nil
}

func validateClusterNetwork(network *privatev1.ClusterNetwork) error {
	if network == nil {
		return nil
	}
	if network.HasPodCidr() {
		if _, _, err := net.ParseCIDR(network.GetPodCidr()); err != nil {
			return grpcstatus.Errorf(
				grpccodes.InvalidArgument,
				"invalid pod_cidr %q: %v",
				network.GetPodCidr(), err,
			)
		}
	}
	if network.HasServiceCidr() {
		if _, _, err := net.ParseCIDR(network.GetServiceCidr()); err != nil {
			return grpcstatus.Errorf(
				grpccodes.InvalidArgument,
				"invalid service_cidr %q: %v",
				network.GetServiceCidr(), err,
			)
		}
	}
	return nil
}

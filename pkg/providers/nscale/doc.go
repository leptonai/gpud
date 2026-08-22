// Package nscale implements nscale provider detection and metadata helpers.
//
// Nscale is an OpenStack-based cloud provider. GPUd detects nscale by
// reading the OpenStack metadata JSON at:
//
//	http://169.254.169.254/openstack/latest/meta_data.json
//
// The metadata response includes uuid, availability_zone, and a meta
// object with provider-specific identifiers (organizationID, projectID,
// regionID). Provider detection succeeds when uuid, organizationID,
// and projectID are all non-empty.
//
// Instance metadata (instance-id, local-ipv4, public-ipv4) is read from
// the EC2-compatible endpoint at:
//
//	http://169.254.169.254/latest/meta-data/<path>
//
// The region is read from meta.regionID or availability_zone only when
// the value is human-readable; opaque region UUIDs and generic "nova"
// availability zones are rejected.
//
// ref. https://docs.openstack.org/nova/latest/user/metadata.html
// ref. https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html
package nscale

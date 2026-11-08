// Package fabric evaluates NVML fabric registration independently of NVLink port
// and peer connectivity. The NVLink and fabric-manager components share this
// policy so incomplete registration cannot be mistaken for broken hardware.
package fabric

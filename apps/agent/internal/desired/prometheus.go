package desired

import (
	"fmt"
	"net"
	"strconv"

	"google.golang.org/protobuf/proto"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/promexport"
	"ngfw/agent/internal/scheduler"
)

// PrometheusDescriptorName identifies the singleton external listener.
const PrometheusDescriptorName = "prometheus.listener"

// PrometheusKey is the listener singleton key.
var PrometheusKey = scheduler.Join(PrometheusDescriptorName, "vrx")

// PrometheusAddress validates defaults and direct gRPC input.
func PrometheusAddress(v *vrxv1.ManagementPrometheus) (string, error) {
	if v == nil {
		return "", fmt.Errorf("ManagementPrometheus required")
	}
	host := v.GetListen()
	if v.Listen == nil {
		host = "0.0.0.0"
	}
	if net.ParseIP(host) == nil {
		return "", fmt.Errorf("management.prometheus.listen must be an IP address")
	}
	port := v.GetPort()
	if v.Port == nil {
		port = 9101
	}
	if port == 0 || port > 65535 {
		return "", fmt.Errorf("management.prometheus.port must be in 1–65535")
	}
	if _, err := promexport.ParseAllow(v.GetAllow()); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10)), nil
}

// Prometheus projects the enabled management listener.
func Prometheus(s Sink, ds *vrxv1.DesiredState) {
	v := ds.GetManagement().GetPrometheus()
	if !v.GetEnabled() {
		return
	}
	if _, err := PrometheusAddress(v); err != nil {
		s.Errorf(Ptr("management", "prometheus"), "management.prometheus.invalid", "%v", err)
		return
	}
	s.Add(PrometheusKey, proto.Clone(v), Ptr("management", "prometheus"))
}

// AssemblePrometheus restores the retrieved listener configuration.
func AssemblePrometheus(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key != PrometheusKey {
			continue
		}
		if v, ok := kv.Value.(*vrxv1.ManagementPrometheus); ok {
			if ds.Management == nil {
				ds.Management = &vrxv1.ManagementConfig{}
			}
			ds.Management.Prometheus = proto.Clone(v).(*vrxv1.ManagementPrometheus)
		}
	}
}

func init() { ManagementImplemented["prometheus"] = true }

package subsystems

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	binlcp "ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/basepolicy"
	"ngfw/agent/internal/scheduler"
)

// EnvBasePolicy explicitly activates the appliance dynamic admission adapter.
const EnvBasePolicy = "VRX_BASE_POLICY"

type basePolicyRuntime struct {
	config    basepolicy.Config
	namespace func(context.Context) (string, error)
}

var basePolicyRuntimes sync.Map

func init() { Domains[Interfaces] = append(Domains[Interfaces], basepolicy.DescriptorName) }
func registerBasePolicy(reg scheduler.Registry, w *Wiring) error {
	basePolicyRuntimes.Delete(w.env.Owner)
	if os.Getenv(EnvBasePolicy) != "1" {
		return nil
	}
	if w.env.Owner != "vrx" || !w.env.GlobalsOwner {
		return errors.New("basepolicy: explicit activation requires appliance globals owner")
	}
	config, err := basepolicy.LoadConfig(basepolicy.ProductConfig)
	if err != nil {
		return err
	}
	runner := renderers.NewSystemRunner(renderers.NewAllowlist(basepolicy.NftBin))
	runner.MaxOutput = basepolicy.MaxJSON
	renderer, err := basepolicy.New(runner, config.Management)
	if err != nil {
		return err
	}
	pairDescriptor := lcp.NewItfPair(w.env.Client, w.env.Owner, lcp.WithInterfaceKey(dfkit.DefaultInterfaceKey))
	descriptor, err := basepolicy.NewDescriptor(renderer, config, func(ctx context.Context) ([]lcp.ItfPair, error) {
		kvs, err := pairDescriptor.Retrieve(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]lcp.ItfPair, 0, len(kvs))
		for _, kv := range kvs {
			var pair lcp.ItfPair
			if err := dfkit.Decode(kv.Value, &pair); err != nil {
				return nil, err
			}
			out = append(out, pair)
		}
		return out, nil
	})
	if err != nil {
		return err
	}
	runtime := &basePolicyRuntime{config: config, namespace: func(ctx context.Context) (string, error) {
		reply, err := binlcp.NewServiceClient(w.env.Client).LcpDefaultNsGet(ctx, &binlcp.LcpDefaultNsGet{})
		if err != nil {
			return "", err
		}
		namespace := strings.TrimRight(reply.Netns, "\x00")
		// Unlike the generic descriptor's VPP bug workaround, invalid readback is
		// ambiguous for host admission and must never be inferred to mean root.
		if err := (lcp.DefaultNetns{Netns: namespace}).Validate(); err != nil {
			return "", err
		}
		return namespace, nil
	}}
	reg.Register(descriptor)
	basePolicyRuntimes.Store(w.env.Owner, runtime)
	return nil
}

// ProjectBasePolicy augments interfaces transactions only for explicitly enabled
// product agents. The supplied KVs are the canonical desired LCP projection.
func ProjectBasePolicy(ctx context.Context, owner string, sink desired.Sink, kvs []scheduler.KV) {
	value, enabled := basePolicyRuntimes.Load(owner)
	if !enabled {
		return
	}
	runtime := value.(*basePolicyRuntime)
	var pairs []lcp.ItfPair
	for _, kv := range kvs {
		if kv.Key.Descriptor() != lcp.NameItfPair {
			continue
		}
		var pair lcp.ItfPair
		if err := dfkit.Decode(kv.Value, &pair); err != nil {
			sink.Errorf("/interfaces", "basepolicy.punt", "%v", err)
			return
		}
		pairs = append(pairs, pair)
	}
	var namespace string
	var err error
	finalNamespace := false
	for _, kv := range kvs {
		if kv.Key == lcp.KeyDefaultNetns {
			var setting lcp.DefaultNetns
			err = dfkit.Decode(kv.Value, &setting)
			if err == nil {
				err = setting.Validate()
			}
			namespace = setting.Netns
			finalNamespace = true
			break
		}
	}
	if !finalNamespace {
		namespace, err = runtime.namespace(ctx)
	}
	if err != nil {
		sink.Errorf("/interfaces", "basepolicy.namespace", "cannot establish effective LCP namespace: %v", err)
		return
	}
	admissions, err := basepolicy.Project(runtime.config.Management, runtime.config.Permanent, pairs, namespace, true)
	if err != nil {
		sink.Errorf("/interfaces", "basepolicy.punt", "%v", err)
		return
	}
	for _, admission := range admissions {
		sink.Add(scheduler.Join(basepolicy.DescriptorName, admission.Host), dfkit.Encode(admission), "/interfaces")
	}
}

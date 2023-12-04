package translation

import (
	"encoding/json"
	"sort"

	mosniov1 "mosn.io/moe/controller/api/v1"
	"mosn.io/moe/controller/internal/model"
	"mosn.io/moe/pkg/filtermanager"
	"mosn.io/moe/pkg/plugins"
)

// mergedState does the following:
// 1. merge policy among the same level policies
// 2. merge policy among different hierarchies
// 3. transform a plugin to different plugins if needed
type mergedState struct {
	Hosts map[string]*mergedHostPolicy
}

type mergedHostPolicy struct {
	VirtualHost *model.VirtualHost
	Routes      map[string]*mergedRoutePolicy
	Policy      *filtermanager.FilterManagerConfig
}

type mergedRoutePolicy struct {
	Policy *filtermanager.FilterManagerConfig
}

func toMergedState(ctx *Ctx, state *dataPlaneState) (*FinalState, error) {
	s := &mergedState{
		Hosts: make(map[string]*mergedHostPolicy),
	}
	for name, host := range state.Hosts {
		mh := &mergedHostPolicy{
			VirtualHost: host.VirtualHost,
			Routes:      make(map[string]*mergedRoutePolicy),
		}
		// FIXME: implement merge policy
		// According to the https://gateway-api.sigs.k8s.io/geps/gep-713/,
		// 1. A Policy targeting a more specific scope wins over a policy targeting a lesser specific scope.
		// 2. If multiple polices configure the same plugin, the oldest one (based on creation timestamp) wins.
		// 3. If there are multiple oldest polices, the one appearing first in alphabetical order by {namespace}/{name} wins.
		mergedPolicy := host.Policies[0]
		fmc := translateHTTPFilterPolicyToFilterManagerConfig(mergedPolicy)
		mh.Policy = fmc
		s.Hosts[name] = mh
	}

	return toFinalState(ctx, s)
}

func translateHTTPFilterPolicyToFilterManagerConfig(policy *mosniov1.HTTPFilterPolicy) *filtermanager.FilterManagerConfig {
	fmc := &filtermanager.FilterManagerConfig{
		Plugins: []*filtermanager.FilterConfig{},
	}
	for name, filter := range policy.Spec.Filters {
		cfg := model.GoPluginConfig{}
		// we validated the filter at the beginning, so theorily err should not happen
		_ = json.Unmarshal(filter.Raw, &cfg)
		fmc.Plugins = append(fmc.Plugins, &filtermanager.FilterConfig{
			Name:   name,
			Config: cfg.Config,
		})
	}

	sort.Slice(fmc.Plugins, func(i, j int) bool {
		return plugins.ComparePluginOrder(fmc.Plugins[i].Name, fmc.Plugins[j].Name)
	})
	return fmc
}

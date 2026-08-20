/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import "strings"

// OutboundProxy is a named proxy that can be selected for global outbound work.
// Mirror proxies remain independently configured on each Mirror.
type OutboundProxy struct {
	Name     string `json:"name" yaml:"name"`
	URL      string `json:"url" yaml:"url"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

func (p OutboundProxy) DeepCopy() OutboundProxy {
	return OutboundProxy{
		Name:     strings.Clone(p.Name),
		URL:      strings.Clone(p.URL),
		Username: strings.Clone(p.Username),
		Password: strings.Clone(p.Password),
	}
}

type ProxyConfig struct {
	Selected string          `json:"selected" yaml:"selected"`
	Proxies  []OutboundProxy `json:"proxies" yaml:"proxies"`
}

func (p ProxyConfig) DeepCopy() ProxyConfig {
	cloned := ProxyConfig{Selected: strings.Clone(p.Selected)}
	if p.Proxies != nil {
		cloned.Proxies = make([]OutboundProxy, len(p.Proxies))
		for i := range p.Proxies {
			cloned.Proxies[i] = p.Proxies[i].DeepCopy()
		}
	}
	return cloned
}

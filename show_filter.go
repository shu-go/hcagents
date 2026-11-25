package main

import "github.com/shu-go/hcagents/internal/config"

// shownOf is snap with only the agents the show: setting lists. An agent goes by
// its Group (with its sub-agents, as the list's sections), so an idle parent of a
// working sub-agent stays with "active". Pinned agents always stay: pinning says
// "keep this in sight". Sub-agents of a listed agent are kept as they are.
func shownOf(snap Snapshot, mode string) Snapshot {
	if mode == config.ShowAll || len(snap.Agents) == 0 {
		return snap
	}
	list := make([]AgentView, 0, len(snap.Agents))
	for _, a := range snap.Agents {
		g := a.Group
		if g == "" {
			g = a.Status
		}
		hidden := g == "idle" || (mode == config.ShowAttention && g == "working")
		if a.Pinned || !hidden {
			list = append(list, a)
		}
	}
	snap.Agents = list
	return snap
}

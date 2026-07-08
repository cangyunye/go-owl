package node

func ContainsAny(slice, items []string) bool {
	if len(items) == 0 {
		return true
	}
	for _, item := range items {
		for _, s := range slice {
			if s == item {
				return true
			}
		}
	}
	return false
}

func ListNodesByGroups(resolver *NodeResolver, groups []string) ([]*ResolvedNode, error) {
	if len(groups) == 0 {
		return resolver.ListNodes(nil)
	}
	seen := make(map[string]bool)
	var result []*ResolvedNode
	for _, g := range groups {
		nodes, err := resolver.ListNodes(&ListOptions{Group: g})
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			if !seen[n.ID] {
				seen[n.ID] = true
				result = append(result, n)
			}
		}
	}
	return result, nil
}

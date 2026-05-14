package node

import (
	"fmt"
	"sync"
)

type NodeResolver struct {
	apiSource   *APINodeSource
	localSource *LocalSource
	sshConfig   *SSHConfigSource
	cache       map[string]*ResolvedNode
	cacheMu     sync.RWMutex
	preferAPI   bool
}

type ResolvedNode struct {
	ID          string
	Name        string
	Address     string
	Port        int
	User        string
	Groups      []string
	Labels      map[string]string
	SSHKey      string
	SSHPassword string
	ProxyJump   string
	Source      string
}

func NewNodeResolver() *NodeResolver {
	return &NodeResolver{
		apiSource:   GetAPINodeSource(),
		localSource: NewLocalSource(),
		sshConfig:   NewSSHConfigSource(),
		cache:       make(map[string]*ResolvedNode),
		preferAPI:   IsAPIEnabled(),
	}
}

func (r *NodeResolver) Resolve(idOrName string) (*ResolvedNode, error) {
	r.cacheMu.RLock()
	if node, ok := r.cache[idOrName]; ok {
		r.cacheMu.RUnlock()
		return node, nil
	}
	r.cacheMu.RUnlock()

	var lastErr error

	if r.preferAPI && r.apiSource != nil {
		node, err := r.resolveFromAPI(idOrName)
		if err == nil {
			return node, nil
		}
		lastErr = err
	}

	if node, err := r.resolveFromLocal(idOrName); err == nil {
		return node, nil
	} else if lastErr == nil {
		lastErr = err
	}

	if node, err := r.resolveFromSSHConfig(idOrName); err == nil {
		return node, nil
	} else if lastErr == nil {
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("节点未找到: %s", idOrName)
}

func (r *NodeResolver) resolveFromAPI(idOrName string) (*ResolvedNode, error) {
	apiNode, err := r.apiSource.GetNode(idOrName)
	if err != nil {
		return nil, err
	}

	node := &ResolvedNode{
		ID:          apiNode.ID,
		Name:        apiNode.Name,
		Address:     apiNode.Address,
		Port:        apiNode.Port,
		User:        apiNode.User,
		Groups:      apiNode.Groups,
		Labels:      apiNode.Labels,
		SSHKey:      apiNode.SSHKey,
		SSHPassword: apiNode.SSHPassword,
		ProxyJump:   apiNode.ProxyJump,
		Source:      "api",
	}

	r.cacheMu.Lock()
	r.cache[idOrName] = node
	r.cacheMu.Unlock()

	return node, nil
}

func (r *NodeResolver) resolveFromLocal(idOrName string) (*ResolvedNode, error) {
	localNode, err := r.localSource.GetNode(idOrName)
	if err != nil {
		return nil, err
	}

	node := &ResolvedNode{
		ID:          localNode.ID,
		Name:        localNode.Name,
		Address:     localNode.Address,
		Port:        localNode.Port,
		User:        localNode.User,
		Groups:      localNode.Groups,
		Labels:      localNode.Labels,
		SSHKey:      localNode.SSHKey,
		SSHPassword: localNode.SSHPassword,
		Source:      "local",
	}

	r.cacheMu.Lock()
	r.cache[idOrName] = node
	r.cacheMu.Unlock()

	return node, nil
}

func (r *NodeResolver) resolveFromSSHConfig(name string) (*ResolvedNode, error) {
	sshNode, err := r.sshConfig.GetNode(name)
	if err != nil {
		return nil, err
	}

	node := &ResolvedNode{
		ID:        sshNode.Name,
		Name:      sshNode.Name,
		Address:   sshNode.HostName,
		Port:      sshNode.Port,
		User:      sshNode.User,
		SSHKey:    sshNode.IdentityFile,
		ProxyJump: sshNode.ProxyJump,
		Source:    "ssh-config",
	}

	r.cacheMu.Lock()
	r.cache[name] = node
	r.cacheMu.Unlock()

	return node, nil
}

func (r *NodeResolver) ListNodes(opts *ListOptions) ([]*ResolvedNode, error) {
	var nodes []*ResolvedNode
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	if r.preferAPI && r.apiSource != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			apiNodes, err := r.apiSource.ListNodes(opts)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			mu.Lock()
			for _, n := range apiNodes {
				nodes = append(nodes, &ResolvedNode{
					ID:          n.ID,
					Name:        n.Name,
					Address:     n.Address,
					Port:        n.Port,
					User:        n.User,
					Groups:      n.Groups,
					Labels:      n.Labels,
					SSHKey:      n.SSHKey,
					SSHPassword: n.SSHPassword,
					ProxyJump:   n.ProxyJump,
					Source:      "api",
				})
			}
			mu.Unlock()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		localNodes, err := r.localSource.ListNodes(opts)
		if err == nil {
			mu.Lock()
			for _, n := range localNodes {
				nodes = append(nodes, &ResolvedNode{
					ID:          n.ID,
					Name:        n.Name,
					Address:     n.Address,
					Port:        n.Port,
					User:        n.User,
					Groups:      n.Groups,
					Labels:      n.Labels,
					SSHKey:      n.SSHKey,
					SSHPassword: n.SSHPassword,
					Source:      "local",
				})
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	if len(nodes) == 0 && firstErr != nil {
		return nil, firstErr
	}

	return nodes, nil
}

func (r *NodeResolver) ResolveMultiple(ids []string) ([]*ResolvedNode, error) {
	nodes := make([]*ResolvedNode, 0, len(ids))
	var errors []string

	for _, id := range ids {
		node, err := r.Resolve(id)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		nodes = append(nodes, node)
	}

	if len(errors) > 0 && len(nodes) == 0 {
		return nil, fmt.Errorf("解析节点失败: %v", errors)
	}

	return nodes, nil
}

func (r *NodeResolver) ClearCache() {
	r.cacheMu.Lock()
	r.cache = make(map[string]*ResolvedNode)
	r.cacheMu.Unlock()

	if r.apiSource != nil {
		r.apiSource.ClearCache()
	}
}

func (r *NodeResolver) Refresh() error {
	r.ClearCache()
	if r.apiSource != nil {
		return r.apiSource.RefreshCache()
	}
	return nil
}

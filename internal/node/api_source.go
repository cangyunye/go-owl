package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	apiEndpoint string
	apiToken    string
	apiTimeout  time.Duration
	apiOnce     sync.Once
	apiClient   *APINodeSource
)

type APINodeSource struct {
	endpoint string
	key      string
	timeout  time.Duration
	client   *http.Client
	cache    map[string]*APINode
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
}

type APINode struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Hostname    string            `json:"hostname"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	User        string            `json:"user"`
	Status      string            `json:"status"`
	Groups      []string          `json:"groups"`
	Labels      map[string]string `json:"labels"`
	SSHKey      string            `json:"ssh_key"`
	SSHPassword string            `json:"ssh_password"`
	ProxyJump   string            `json:"proxy_jump"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	cachedAt    time.Time
}

type APIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type NodeListResponse struct {
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Items    []APINode `json:"items"`
}

func initAPIConfig() {
	apiEndpoint = os.Getenv("OWL_API_ENDPOINT")
	apiToken = os.Getenv("OWL_API_TOKEN")

	timeoutStr := os.Getenv("OWL_API_TIMEOUT")
	if timeoutStr == "" {
		apiTimeout = 30 * time.Second
	} else {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			apiTimeout = time.Duration(t) * time.Second
		} else {
			apiTimeout = 30 * time.Second
		}
	}
}

func GetAPINodeSource() *APINodeSource {
	apiOnce.Do(func() {
		initAPIConfig()
		if apiEndpoint == "" || apiToken == "" {
			return
		}
		apiClient = &APINodeSource{
			endpoint: apiEndpoint,
			key:      apiToken,
			timeout:  apiTimeout,
			client: &http.Client{
				Timeout: apiTimeout,
			},
			cache:    make(map[string]*APINode),
			cacheTTL: 5 * time.Minute,
		}
	})
	return apiClient
}

func IsAPIEnabled() bool {
	return apiEndpoint != "" && apiToken != ""
}

func (s *APINodeSource) makeRequest(method, path string, body interface{}) (*APIResponse, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := s.endpoint + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &apiResp, nil
}

func (s *APINodeSource) ListNodes(opts *ListOptions) ([]*APINode, error) {
	if s == nil {
		return nil, fmt.Errorf("API 节点源未配置")
	}

	path := ""
	if opts != nil {
		params := make([]string, 0)
		if opts.Name != "" {
			params = append(params, "name="+opts.Name)
		}
		if opts.Group != "" {
			params = append(params, "group="+opts.Group)
		}
		if opts.Label != "" {
			params = append(params, "label="+opts.Label)
		}
		if len(params) > 0 {
			path = "?" + params[0]
			for i := 1; i < len(params); i++ {
				path += "&" + params[i]
			}
		}
	}

	resp, err := s.makeRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("API 错误: %s (code: %d)", resp.Message, resp.Code)
	}

	var listResp NodeListResponse
	if err := json.Unmarshal(resp.Data, &listResp); err != nil {
		return nil, fmt.Errorf("解析节点列表失败: %w", err)
	}

	now := time.Now()
	nodes := make([]*APINode, 0, len(listResp.Items))
	for i := range listResp.Items {
		listResp.Items[i].cachedAt = now
		nodes = append(nodes, &listResp.Items[i])

		s.cacheMu.Lock()
		s.cache[listResp.Items[i].ID] = &listResp.Items[i]
		if listResp.Items[i].Name != "" {
			s.cache[listResp.Items[i].Name] = &listResp.Items[i]
		}
		s.cacheMu.Unlock()
	}

	return nodes, nil
}

func (s *APINodeSource) GetNode(idOrName string) (*APINode, error) {
	if s == nil {
		return nil, fmt.Errorf("API 节点源未配置")
	}

	s.cacheMu.RLock()
	if node, ok := s.cache[idOrName]; ok {
		if time.Since(node.cachedAt) < s.cacheTTL {
			s.cacheMu.RUnlock()
			return node, nil
		}
	}
	s.cacheMu.RUnlock()

	path := "/" + idOrName
	resp, err := s.makeRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	if resp.Code != 0 {
		if resp.Code == 1003 {
			return nil, fmt.Errorf("节点不存在: %s", idOrName)
		}
		return nil, fmt.Errorf("API 错误: %s (code: %d)", resp.Message, resp.Code)
	}

	var node APINode
	if err := json.Unmarshal(resp.Data, &node); err != nil {
		return nil, fmt.Errorf("解析节点数据失败: %w", err)
	}

	node.cachedAt = time.Now()

	s.cacheMu.Lock()
	s.cache[node.ID] = &node
	if node.Name != "" {
		s.cache[node.Name] = &node
	}
	s.cacheMu.Unlock()

	return &node, nil
}

func (s *APINodeSource) GetNodesByGroup(group string) ([]*APINode, error) {
	return s.ListNodes(&ListOptions{Group: group})
}

func (s *APINodeSource) GetNodesByLabel(label string) ([]*APINode, error) {
	return s.ListNodes(&ListOptions{Label: label})
}

func (s *APINodeSource) RefreshCache() error {
	if s == nil {
		return fmt.Errorf("API 节点源未配置")
	}

	s.cacheMu.Lock()
	s.cache = make(map[string]*APINode)
	s.cacheMu.Unlock()

	_, err := s.ListNodes(nil)
	return err
}

func (s *APINodeSource) ClearCache() {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	s.cache = make(map[string]*APINode)
	s.cacheMu.Unlock()
}

type ListOptions struct {
	Name  string
	Group string
	Label string
}

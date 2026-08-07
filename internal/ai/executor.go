package ai

import "context"

type (
	QueryNodesParams struct {
		Group  string                 `json:"group"`
		Labels map[string]interface{} `json:"labels"`
		Status string                 `json:"status"`
		Search string                 `json:"search"`
		Format string                 `json:"format"`
	}

	ExecCommandParams struct {
		Nodes   []string `json:"nodes"`
		Command string   `json:"command"`
		Group   string   `json:"group"`
		Label   string   `json:"label"`
		Search  string   `json:"search"`
		Timeout int      `json:"timeout"`
		Format  string   `json:"format"`
		Mode    string   `json:"mode"`
	}

	ExecScriptParams struct {
		Script  string   `json:"script"`
		Nodes   []string `json:"nodes"`
		Group   string   `json:"group"`
		Label   string   `json:"label"`
		Search  string   `json:"search"`
		Dest    string   `json:"dest"`
		Args    string   `json:"args"`
		Timeout int      `json:"timeout"`
		Inline  bool     `json:"inline"`
		Keep    bool     `json:"keep"`
	}

	GeneratePlaybookParams struct {
		Requirement string                 `json:"requirement"`
		Vars        map[string]interface{} `json:"vars"`
	}

	TransferFileParams struct {
		SourceFile string   `json:"source_file"`
		Nodes      []string `json:"nodes"`
		DestDir    string   `json:"dest_dir"`
		Mode       string   `json:"mode"`
		Permission string   `json:"permission"`
		Search     string   `json:"search"`
	}

	RunPlaybookParams struct {
		Name   string                 `json:"name"`
		Nodes  []string               `json:"nodes"`
		Group  string                 `json:"group"`
		Label  string                 `json:"label"`
		Search string                 `json:"search"`
		Vars   map[string]interface{} `json:"vars"`
		Tags   string                 `json:"tags"`
		Check  bool                   `json:"check"`
	}

	QueryNodesResult       struct{ Text string }
	ExecResult             struct{ Text string }
	ExecScriptResult       struct{ Text string }
	GeneratePlaybookResult struct{ Text string }
	TransferResult         struct{ Text string }
	RunPlaybookResult      struct{ Text string }
	ListPlaybooksResult    struct{ Text string }
	PlaybookInfoResult     struct{ Text string }
	ValidateResult         struct{ Text string }
	NodeCheckResult        struct{ Text string }
	QueryDatabaseResult    struct{ Text string }
)

type Executor interface {
	QueryNodes(ctx context.Context, params QueryNodesParams) (*QueryNodesResult, error)
	ExecuteCommand(ctx context.Context, params ExecCommandParams) (*ExecResult, error)
	ExecuteScript(ctx context.Context, params ExecScriptParams) (*ExecScriptResult, error)
	GeneratePlaybook(ctx context.Context, params GeneratePlaybookParams) (*GeneratePlaybookResult, error)
	TransferFile(ctx context.Context, params TransferFileParams) (*TransferResult, error)
	RunPlaybook(ctx context.Context, params RunPlaybookParams) (*RunPlaybookResult, error)
	ListPlaybooks(ctx context.Context) (*ListPlaybooksResult, error)
	PlaybookInfo(ctx context.Context, params PlaybookInfoParams) (*PlaybookInfoResult, error)
	ValidatePlaybook(ctx context.Context, params ValidatePlaybookParams) (*ValidateResult, error)
	NodeCheck(ctx context.Context, params NodeCheckParams) (*NodeCheckResult, error)
	QueryDatabase(ctx context.Context, params QueryDatabaseParams) (*QueryDatabaseResult, error)
	AddNode(ctx context.Context, params NodeAddParams) (*NodeResult, error)
	RemoveNode(ctx context.Context, params NodeRemoveParams) (*NodeResult, error)
	UpdateNode(ctx context.Context, params NodeUpdateParams) (*NodeResult, error)
	NodeStatus(ctx context.Context, params NodeStatusParams) (*NodeResult, error)
	NodePing(ctx context.Context, params NodePingParams) (*NodeResult, error)
	NodeGroups(ctx context.Context, params NodeGroupsParams) (*NodeResult, error)
	NodeLabels(ctx context.Context, params NodeLabelsParams) (*NodeResult, error)
	NodeImport(ctx context.Context, params NodeImportParams) (*NodeResult, error)
	NodeExport(ctx context.Context, params NodeExportParams) (*NodeResult, error)
	FileDownload(ctx context.Context, params FileDownloadParams) (*FileDownloadResult, error)
	PlaybookTemplateList(ctx context.Context) (*PlaybookTemplateListResult, error)
	PlaybookTemplateInfo(ctx context.Context, params PlaybookTemplateInfoParams) (*PlaybookTemplateInfoResult, error)
	PlaybookTemplateExport(ctx context.Context, params PlaybookTemplateExportParams) (*PlaybookTemplateExportResult, error)
	PlaybookScaffold(ctx context.Context, params PlaybookScaffoldParams) (*PlaybookScaffoldResult, error)
	PlaybookStateList(ctx context.Context, params PlaybookStateListParams) (*PlaybookStateListResult, error)
	PlaybookStateShow(ctx context.Context, params PlaybookStateShowParams) (*PlaybookStateShowResult, error)
	AsyncList(ctx context.Context) (*AsyncListResult, error)
	AsyncStatus(ctx context.Context, params AsyncStatusParams) (*AsyncStatusResult, error)
	AsyncCancel(ctx context.Context, params AsyncStatusParams) (*AsyncCancelResult, error)
	SettingsShow(ctx context.Context) (*SettingsShowResult, error)
	SettingsSet(ctx context.Context, params SettingsSetParams) (*SettingsSetResult, error)
	HistoryList(ctx context.Context, params HistoryListParams) (*HistoryListResult, error)
	HistoryClean(ctx context.Context, params HistoryCleanParams) (*HistoryCleanResult, error)
}

type NodeResult struct{ Text string }

type (
	FileDownloadParams struct {
		RemoteFile string   `json:"remote_file"`
		Nodes      []string `json:"nodes"`
		Group      string   `json:"group"`
		Label      string   `json:"label"`
		Dest       string   `json:"dest"`
		Subdir     bool     `json:"subdir"`
		Resume     bool     `json:"resume"`
	}

	PlaybookTemplateInfoParams struct {
		Name string `json:"name"`
	}

	PlaybookTemplateExportParams struct {
		Name string `json:"name"`
		To   string `json:"to"`
	}

	PlaybookScaffoldParams struct {
		Type string `json:"type"`
	}

	PlaybookStateListParams struct {
		Playbook string `json:"playbook"`
		Status   string `json:"status"`
		Limit    int    `json:"limit"`
	}

	PlaybookStateShowParams struct {
		RunID string `json:"run_id"`
		Node  string `json:"node"`
	}

	FileDownloadResult              struct{ Text string }
	PlaybookTemplateListResult      struct{ Text string }
	PlaybookTemplateInfoResult      struct{ Text string }
	PlaybookTemplateExportResult    struct{ Text string }
	PlaybookScaffoldResult          struct{ Text string }
	PlaybookStateListResult         struct{ Text string }
	PlaybookStateShowResult         struct{ Text string }

	AsyncStatusParams struct {
		TaskID string `json:"task_id"`
	}

	SettingsSetParams struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	HistoryListParams struct {
		NodeID string `json:"node_id"`
		OpType string `json:"op_type"`
		Limit  int    `json:"limit"`
	}

	HistoryCleanParams struct {
		Days int `json:"days"`
	}

	AsyncListResult   struct{ Text string }
	AsyncStatusResult struct{ Text string }
	AsyncCancelResult struct{ Text string }
	SettingsShowResult struct{ Text string }
	SettingsSetResult  struct{ Text string }
	HistoryListResult  struct{ Text string }
	HistoryCleanResult struct{ Text string }
)

type PlaybookInfoParams struct {
	Name string `json:"name"`
}

type ValidatePlaybookParams struct {
	File string `json:"file"`
}

type NodeCheckParams struct {
	Nodes   []string `json:"nodes"`
	Group   string   `json:"group"`
	All     bool     `json:"all"`
	Timeout int      `json:"timeout"`
	Update  bool     `json:"update"`
}

type QueryDatabaseParams struct {
	Query  string                 `json:"query"`
	Group  string                 `json:"group"`
	Labels map[string]interface{} `json:"labels"`
	Status string                 `json:"status"`
	Search string                 `json:"search"`
	Format string                 `json:"format"`
}

type (
	NodeAddParams struct {
		Name      string                 `json:"name"`
		Address   string                 `json:"address"`
		Port      int                    `json:"port"`
		User      string                 `json:"user"`
		Password  string                 `json:"password"`
		SSHKey    string                 `json:"ssh_key"`
		ProxyJump string                 `json:"proxy_jump"`
		Groups    string                 `json:"groups"`
		Labels    map[string]interface{} `json:"labels"`
	}

	NodeRemoveParams struct {
		Nodes []string `json:"nodes"`
	}

	NodeUpdateParams struct {
		ID        string                 `json:"id"`
		Name      string                 `json:"name"`
		Address   string                 `json:"address"`
		Port      int                    `json:"port"`
		User      string                 `json:"user"`
		Password  string                 `json:"password"`
		SSHKey    string                 `json:"ssh_key"`
		ProxyJump string                 `json:"proxy_jump"`
		Groups    string                 `json:"groups"`
		Labels    map[string]interface{} `json:"labels"`
		Status    string                 `json:"status"`
	}

	NodeStatusParams struct {
		Nodes  []string `json:"nodes"`
		All    bool     `json:"all"`
		Format string   `json:"format"`
	}

	NodePingParams struct {
		Nodes   []string `json:"nodes"`
		All     bool     `json:"all"`
		Count   int      `json:"count"`
		Timeout int      `json:"timeout_sec"`
	}

	NodeGroupsParams struct {
		Action string `json:"action"`
		Node   string `json:"node"`
		Group  string `json:"group"`
	}

	NodeLabelsParams struct {
		Action string                 `json:"action"`
		Node   string                 `json:"node"`
		Key    string                 `json:"key"`
		Labels map[string]interface{} `json:"labels"`
	}

	NodeImportParams struct {
		File      string `json:"file"`
		Format    string `json:"format"`
		Overwrite bool   `json:"overwrite"`
		DryRun    bool   `json:"dry_run"`
	}

	NodeExportParams struct {
		File   string   `json:"file"`
		Format string   `json:"format"`
		Nodes  []string `json:"nodes"`
		Groups []string `json:"groups"`
	}
)

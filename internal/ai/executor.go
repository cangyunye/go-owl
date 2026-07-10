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
}

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

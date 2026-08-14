package nodes

import (
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type ConfirmModel struct {
	node   *common.NodeInfo
	cursor int // 0=Delete 1=Cancel
	error  string
}

func NewConfirmModel(n *common.NodeInfo) *ConfirmModel {
	return &ConfirmModel{node: n}
}

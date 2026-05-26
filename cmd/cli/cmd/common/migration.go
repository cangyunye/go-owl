package common

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/cangyunye/go-owl/internal/logger"
)

func MigrateNodesJSONToDB(db *sql.DB) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&count)
	if err != nil {
		return nil
	}
	if count > 0 {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	jsonPath := filepath.Join(homeDir, ".owl", "nodes.json")

	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		logger.Warn("failed to read nodes.json for migration", logger.WithError(err))
		return nil
	}

	var nodes []*NodeInfo
	if err := json.Unmarshal(data, &nodes); err != nil {
		logger.Warn("failed to parse nodes.json for migration", logger.WithError(err))
		return nil
	}

	migrated := 0
	for _, n := range nodes {
		groupsJSON, err := json.Marshal(n.Groups)
		if err != nil {
			logger.Warn("failed to marshal groups for node during migration", logger.WithField("node_id", n.ID), logger.WithError(err))
			continue
		}
		labelsJSON, err := json.Marshal(n.Labels)
		if err != nil {
			logger.Warn("failed to marshal labels for node during migration", logger.WithField("node_id", n.ID), logger.WithError(err))
			continue
		}

		_, err = db.Exec(
			`INSERT INTO nodes (id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at, last_check_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, n.Name, n.Address, n.Port, n.User, n.Password, n.SSHKey, n.Status,
			string(groupsJSON), string(labelsJSON), n.ProxyJump,
			n.CreatedAt, n.UpdatedAt, n.LastCheckAt,
		)
		if err != nil {
			logger.Warn("failed to insert node during migration", logger.WithField("node_id", n.ID), logger.WithError(err))
			continue
		}
		migrated++
	}

	if migrated > 0 {
		logger.Info("Migrated nodes from nodes.json to database", logger.WithField("count", migrated))
	}

	return nil
}

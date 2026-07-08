package handler

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"golang.org/x/crypto/ssh"
)

type sshExecutor struct {
	db *sql.DB
}

type nodeSSHInfo struct {
	Address  string
	Port     int
	User     string
	Password string
	SSHKey   string
}

func (e *sshExecutor) getNodeInfo(nodeID string) (*nodeSSHInfo, error) {
	var info nodeSSHInfo
	var pw, key sql.NullString
	err := e.db.QueryRow(
		`SELECT address, port, user, password, ssh_key FROM nodes WHERE id = ?`, nodeID,
	).Scan(&info.Address, &info.Port, &info.User, &pw, &key)
	if err != nil {
		return nil, err
	}
	if pw.Valid {
		info.Password = pw.String
	}
	if key.Valid {
		info.SSHKey = key.String
	}
	return &info, nil
}

func (e *sshExecutor) Execute(ctx context.Context, nodeID, command string) (string, int, error) {
	info, err := e.getNodeInfo(nodeID)
	if err != nil {
		return "", -1, fmt.Errorf("resolve node: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            info.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         0,
	}

	if info.Password != "" {
		config.Auth = append(config.Auth, ssh.Password(info.Password))
	}
	if info.SSHKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(info.SSHKey))
		if err != nil {
			return "", -1, fmt.Errorf("parse ssh key: %w", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}

	addr := info.Address + ":" + strconv.Itoa(info.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "", -1, fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return "", -1, fmt.Errorf("ssh exec: %w", err)
		}
	}

	return string(output), exitCode, nil
}

func (e *sshExecutor) dial(nodeID string) (*ssh.Client, error) {
	info, err := e.getNodeInfo(nodeID)
	if err != nil {
		return nil, fmt.Errorf("resolve node: %w", err)
	}
	config := &ssh.ClientConfig{
		User:            info.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         0,
	}
	if info.Password != "" {
		config.Auth = append(config.Auth, ssh.Password(info.Password))
	}
	if info.SSHKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(info.SSHKey))
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}
	addr := info.Address + ":" + strconv.Itoa(info.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	return client, nil
}

func (e *sshExecutor) ExecuteStream(ctx context.Context, nodeID, command string, outputCh chan<- OutputLine) (int, error) {
	client, err := e.dial(nodeID)
	if err != nil {
		return -1, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start(command); err != nil {
		return -1, fmt.Errorf("ssh start: %w", err)
	}

	done := make(chan struct{}, 2)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: scanner.Text(), Type: "stdout"}:
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
		}
		done <- struct{}{}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: scanner.Text(), Type: "stderr"}:
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
		}
		done <- struct{}{}
	}()

	err = session.Wait()
	<-done
	<-done

	exitCode := 0
	if exitErr, ok := err.(*ssh.ExitError); ok {
		exitCode = exitErr.ExitStatus()
		err = nil
	}
	return exitCode, err
}

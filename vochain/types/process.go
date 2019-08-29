package vochain

import (
	"fmt"
)

// ________________________ PROCESS ________________________

// Process represents a state per process
type Process struct {
	// ProcessID identifies unequivocally a process
	ProcessID string
	// EntityID identifies unequivocally a process
	EntityID string
	// Votes is a list containing all the processed and valid votes (here votes are final)
	Votes map[string]*Vote `json:"votes"`
	// MkRoot merkle root of all the census in the process
	MkRoot string `json:"mkroot"`
	// EndBlock represents the tendermint block where the process goes from active to finished
	NumberOfBlocks int64 `json:"endblock"`
	// InitBlock represents the tendermint block where the process goes from scheduled to active
	InitBlock int64 `json:"initblock"`
	// CurrentState is the current process state
	CurrentState CurrentProcessState `json:"currentstate"`
	// EncryptionKeys are the keys required to encrypt the votes
	EncryptionKeys []string `json:"encryptionkeys"`
}

func (p *Process) String() string {
	return fmt.Sprintf(`{
		"processId": %v, 
		"entityId": %v,
		"votes": %v,
		"mkRoot": %v,
		"initBlock": %v,
		"encryptionKeys": %v,
		"currentState": %v }`,
		p.ProcessID,
		p.EntityID,
		p.Votes,
		p.MkRoot,
		p.InitBlock,
		p.EncryptionKeys,
		p.CurrentState,
	)
}

// NewProcess returns a new Process instance
func NewProcess() *Process {
	return &Process{}
}

// CurrentProcessState represents the current phase of process state
type CurrentProcessState int8

const (
	// Scheduled process is scheduled to start at some point of time
	Scheduled CurrentProcessState = iota
	// Active process is in progress
	Active
	// Paused active process is paused
	Paused
	// Finished process is finished
	Finished
	// Canceled process is canceled and/or invalid
	Canceled
)

const (
	SCHEDULED string = "scheduled"
	ACTIVE    string = "active"
	PAUSED    string = "paused"
	FINISHED  string = "finished"
	CANCELED  string = "cancelled"
)

// String returns the CurrentProcessState as string
func (c *CurrentProcessState) String() string {
	switch *c {
	// scheduled
	case 0:
		return fmt.Sprintf("%s", SCHEDULED)
	// active
	case 1:
		return fmt.Sprintf("%s", ACTIVE)
	// paused
	case 2:
		return fmt.Sprintf("%s", PAUSED)
	// finished
	case 3:
		return fmt.Sprintf("%s", FINISHED)
	// canceled
	case 4:
		return fmt.Sprintf("%s", CANCELED)
	default:
		return ""
	}
}
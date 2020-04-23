package scrutinizer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"gitlab.com/vocdoni/go-dvote/log"
	"gitlab.com/vocdoni/go-dvote/types"
	"gitlab.com/vocdoni/go-dvote/util"
)

func unmarshalVote(votePackage string) (*types.VotePackage, error) {
	rawVote, err := base64.StdEncoding.DecodeString(votePackage)
	if err != nil {
		return nil, err
	}
	var vote types.VotePackage
	if err := json.Unmarshal(rawVote, &vote); err != nil {
		return nil, err
	}
	return &vote, nil
}

func (s *Scrutinizer) addLiveResultsVote(envelope *types.Vote) error {
	vote, err := unmarshalVote(envelope.VotePackage)
	if err != nil {
		return err
	}
	if len(vote.Votes) > MaxQuestions {
		return fmt.Errorf("too many questions on addVote")
	}

	process, err := s.Storage.Get([]byte(types.ScrutinizerLiveProcessPrefix + envelope.ProcessID))
	if err != nil {
		return fmt.Errorf("error adding vote to process %s, skipping addVote: (%s)", envelope.ProcessID, err)
	}

	var pv ProcessVotes
	if err := s.VochainState.Codec.UnmarshalBinaryBare(process, &pv); err != nil {
		return fmt.Errorf("cannot unmarshal vote (%s)", err.Error())
	}

	for question, opt := range vote.Votes {
		if opt > MaxOptions {
			log.Warn("option overflow on addVote")
			continue
		}
		pv[question][opt]++
	}

	process, err = s.VochainState.Codec.MarshalBinaryBare(pv)
	if err != nil {
		return err
	}

	if err := s.Storage.Put([]byte(types.ScrutinizerLiveProcessPrefix+envelope.ProcessID), process); err != nil {
		return err
	}

	log.Debugf("addVote on process %s", envelope.ProcessID)
	return nil
}

// ComputeResult process a finished voting, compute the results and saves it in the Storage
func (s *Scrutinizer) ComputeResult(processID string) error {
	log.Debugf("computing results for %s", processID)
	// Check if process exist
	_, err := s.VochainState.Process(processID)
	if err != nil {
		return err
	}

	// If result already exist, skipping
	_, err = s.Storage.Get([]byte(types.ScrutinizerResultsPrefix + processID))
	if err == nil {
		return fmt.Errorf("process %s already computed", processID)
	}
	if err != nil && err.Error() != NoKeyStorageError {
		return err
	}

	// Compute the results
	// If poll-vote, results have been computed during their arrival
	isLive, err := s.isLiveResultsProcess(processID)
	if err != nil {
		return err
	}
	var pv ProcessVotes
	if isLive {
		if pv, err = s.computeLiveResults(processID); err != nil {
			return err
		}
		// Delete liveResults temporary storage
		if err = s.Storage.Del([]byte(types.ScrutinizerLiveProcessPrefix + processID)); err != nil {
			return err
		}
	} else {
		if pv, err = s.computeNonLiveResults(processID); err != nil {
			return err
		}
	}

	result, err := s.VochainState.Codec.MarshalBinaryBare(pv)
	if err != nil {
		return err
	}

	if err := s.Storage.Put([]byte(types.ScrutinizerResultsPrefix+processID), result); err != nil {
		return err
	}

	return nil
}

// VoteResult returns the current result for a processId summarized in a two dimension int slice
func (s *Scrutinizer) VoteResult(processID string) (ProcessVotes, error) {
	processID = util.TrimHex(processID)
	// Check if process exist
	_, err := s.VochainState.Process(processID)
	if err != nil {
		return nil, err
	}

	log.Debugf("finding results for %s", processID)
	// If exist a summary of the voting process, just return it
	var pv ProcessVotes
	processBytes, err := s.Storage.Get([]byte(types.ScrutinizerResultsPrefix + processID))
	if err != nil && err.Error() != NoKeyStorageError {
		return nil, err
	}
	if err == nil {
		if err := s.VochainState.Codec.UnmarshalBinaryBare(processBytes, &pv); err != nil {
			return nil, err
		}
		return pv, nil
	}

	// If results are not available, check if the process is PollVote (live)
	isLive, err := s.isLiveResultsProcess(processID)
	if err != nil {
		return nil, err
	}
	if !isLive {
		return nil, fmt.Errorf("no results yet")
	}

	// Return live results
	return s.computeLiveResults(processID)
}

func (s *Scrutinizer) computeLiveResults(processID string) (pv ProcessVotes, err error) {
	var pb []byte
	pb, err = s.Storage.Get([]byte(types.ScrutinizerLiveProcessPrefix + processID))
	if err != nil {
		return
	}
	if err = s.VochainState.Codec.UnmarshalBinaryBare(pb, &pv); err != nil {
		return
	}
	pruneVoteResult(&pv)
	log.Debugf("computed live results for %s", processID)
	return
}

func (s *Scrutinizer) computeNonLiveResults(processID string) (pv ProcessVotes, err error) {
	pv = emptyProcess()
	var nvotes int
	for _, e := range s.VochainState.EnvelopeList(processID, 0, 32<<20) { // 32K seems enough for now
		v, err := s.VochainState.Envelope(fmt.Sprintf("%s_%s", processID, e))
		if err != nil {
			log.Warn(err)
			continue
		}
		vp, err := unmarshalVote(v.VotePackage)
		for question, opt := range vp.Votes {
			if opt > MaxOptions {
				log.Warn("option overflow on computeResult, skipping vote...")
				continue
			}
			pv[question][opt]++
		}
		nvotes++
	}
	pruneVoteResult(&pv)
	log.Infof("computed results for process %s with %d votes", processID, nvotes)
	return
}

// To-be-improved
func pruneVoteResult(pv *ProcessVotes) {
	pvv := *pv
	var pvc ProcessVotes
	min := MaxQuestions - 1
	for ; min >= 0; min-- { // find the real size of first dimension (questions with some answer)
		j := 0
		for ; j < MaxOptions; j++ {
			if pvv[min][j] != 0 {
				break
			}
		}
		if j < MaxOptions {
			break
		} // we found a non-empty question, this is the min. Stop iteration.
	}

	for i := 0; i <= min; i++ { // copy the options for each question but pruning options too
		pvc = make([][]uint32, i+1)
		for i2 := 0; i2 <= i; i2++ { // copy only the first non-zero values
			j2 := MaxOptions - 1
			for ; j2 >= 0; j2-- {
				if pvv[i2][j2] != 0 {
					break
				}
			}
			pvc[i2] = make([]uint32, j2+1)
			copy(pvc[i2], pvv[i2])
		}
	}
	*pv = pvc
	return
}